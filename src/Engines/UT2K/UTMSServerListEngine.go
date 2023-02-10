package UT2K

import (
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"net/netip"
	"os-serverlist-sync/Engine"
)

type UTMSServerListEngineParams struct {
	ServerAddress string `json:"address"`
	CdKey         string `json:"cdkey"`
	ClientName    string `json:"client_name"`
	ClientVersion int    `json:"client_version"`
	RunningOs     int    `json:"running_os"`
	Language      string `json:"language"`
	GpuDeviceId   int    `json:"gpu_device_id"`
	GpuVendorId   int    `json:"gpu_vendor_id"`
	CpuCycles     int    `json:"cpu_cycles"`
	RunningCpus   int    `json:"running_cpus"`

	//XXX: filter list
}

type UTMSParserState struct {
	TotalLength   int
	CurrentOffset int
	Buffer        []byte
}

type UTMSServerListEngine struct {
	connection    *net.TCPConn
	queryEngine   Engine.IQueryEngine
	params        *UTMSServerListEngineParams
	parser        UTMSParserState
	gotFatalError bool

	challenge string
}

func (se *UTMSServerListEngine) SetQueryEngine(engine Engine.IQueryEngine) {
	se.queryEngine = engine
}

func (se *UTMSServerListEngine) SetParams(params interface{}) {
	se.params = params.(*UTMSServerListEngineParams)
}

func (se *UTMSServerListEngine) Invoke() {

	fmt.Println("Invoke " + se.params.ServerAddress)

	servAddr := se.params.ServerAddress
	tcpAddr, err := net.ResolveTCPAddr("tcp", servAddr)
	if err != nil {
		println("ResolveTCPAddr failed:", err.Error())
		return
	}

	conn, dialErr := net.DialTCP("tcp", nil, tcpAddr)
	if dialErr != nil {
		println("Dial failed:", dialErr.Error())
		return
	}
	se.connection = conn

	//wait for TCP reply, etc
	se.think()
}

func (se *UTMSServerListEngine) think() {
	se.readMessage()
}

func (se *UTMSServerListEngine) readMessage() {
	defer se.connection.Close()

	se.readChallenge()

	se.readValidation()

	if se.gotFatalError {
		fmt.Printf("Got fatal error from MS, aborting")
		return
	}

	if se.params.ClientVersion >= 3000 {
		se.readVerification()
		if se.gotFatalError {
			fmt.Printf("Got fatal error from MS, aborting")
			return
		}
	}

	se.sendListRequest()
	if se.gotFatalError {
		fmt.Printf("Got fatal error from MS, aborting")
		return
	}

	se.readListResponse()
}

func (se *UTMSServerListEngine) sendListRequest() {
	var currentIndex int = 0

	sendBuffer := make([]byte, 256)

	sendBuffer[0] = 0 //redundant, but this is the msgid
	currentIndex++

	sendBuffer[0] = 0 //num properties ()
	currentIndex++

	se.sendBuffer(sendBuffer[0:currentIndex])
}

func (se *UTMSServerListEngine) readListResponse() {
	se.waitForData()

	if se.gotFatalError {
		return
	}

	//got list... parse
	numServers := binary.LittleEndian.Uint32(se.parser.Buffer[se.parser.CurrentOffset:4])
	se.parser.CurrentOffset += 4

	se.parser.CurrentOffset++ //compressed (skip)

	for i := uint32(0); i < numServers; i++ {
		se.waitForData()

		if se.gotFatalError {
			break
		}

		//Why does epic games not understand that network comms is big endian?!
		var invertedBuffer = []byte{
			se.parser.Buffer[se.parser.CurrentOffset+0],
			se.parser.Buffer[se.parser.CurrentOffset+1],
			se.parser.Buffer[se.parser.CurrentOffset+2],
			se.parser.Buffer[se.parser.CurrentOffset+3],
		}

		serverIP, _ := netip.AddrFromSlice(invertedBuffer)
		se.parser.CurrentOffset += 4

		se.parser.CurrentOffset += 2 //skip game port (we want query port)

		serverPort := binary.LittleEndian.Uint16(se.parser.Buffer[se.parser.CurrentOffset:])

		var addr = netip.AddrPortFrom(serverIP, serverPort)
		se.queryEngine.Query(addr)

		break
	}
}

func (se *UTMSServerListEngine) waitForData() {
	lengthBuffer := make([]byte, 4)

	_, lenErr := se.connection.Read(lengthBuffer)
	if lenErr != nil {
		println("Failed to read UTMS recv length", lenErr.Error())
		se.gotFatalError = true
		return
	}
	length := binary.LittleEndian.Uint32(lengthBuffer)

	incomingBuffer := make([]byte, length)

	se.parser.TotalLength = 0
	var expectedLen int = int(length)

	//Read all expected data...
	for {
		readLen, incErr := se.connection.Read(incomingBuffer[se.parser.TotalLength:])
		if readLen > 0 {
			se.parser.TotalLength += readLen
		}

		if incErr != nil {
			println("Failed to read UTMS incoming buffer", incErr.Error())
			se.gotFatalError = true
			return
		}

		if se.parser.TotalLength >= expectedLen {
			break
		}
	}

	se.parser.CurrentOffset = 0

	se.parser.Buffer = incomingBuffer
}
func (se *UTMSServerListEngine) readChallenge() {
	se.waitForData()

	se.challenge = se.readCompactString()
	se.writeClientInfo()
}

func (se *UTMSServerListEngine) readVerification() {
	se.waitForData()

	var verified = se.readCompactString()

	if verified != "VERIFIED" {
		se.gotFatalError = true
	}
}

func (se *UTMSServerListEngine) readValidation() {
	se.waitForData()
	var status = se.readCompactString()

	if status != "APPROVED" {
		se.gotFatalError = true
		return
	}

	if se.params.ClientVersion >= 3000 {
		verificationData := make([]byte, 34)
		verificationData[1] = 0x14
		verificationData[2] = 0xe8

		se.sendBuffer(verificationData)
	}
}

func (se *UTMSServerListEngine) readCompactString() string {
	var length int = int(se.parser.Buffer[se.parser.CurrentOffset])
	se.parser.CurrentOffset++
	string_data := string(se.parser.Buffer[se.parser.CurrentOffset:length])
	return string_data
}

func (se *UTMSServerListEngine) getCompactStringBuffer(str string) []byte {
	var len = len(str)
	buffer := make([]byte, len+2) //+2 for compact len (byte only currently) and null terminator

	buffer[0] = byte(len + 1)
	copy(buffer[1:], []byte(str))

	buffer[len+1] = 0
	return buffer
}

func (se *UTMSServerListEngine) writeClientInfo() {
	var currentIndex int = 0

	sendBuffer := make([]byte, 256)

	//Write CD Key hash
	cdKeyHash := md5.Sum([]byte(se.params.CdKey))
	var cdKeyHashStr = se.getCompactStringBuffer(hex.EncodeToString(cdKeyHash[:]))
	copy(sendBuffer[currentIndex:], cdKeyHashStr)
	currentIndex += len(cdKeyHashStr)

	//Write CD Key response
	cdKeyResponseHash := md5.Sum([]byte(se.params.CdKey + se.challenge))
	var cdKeyResponse = se.getCompactStringBuffer(hex.EncodeToString(cdKeyResponseHash[:]))
	copy(sendBuffer[currentIndex:], cdKeyResponse)
	currentIndex += len(cdKeyResponse)

	//Write client name
	var clientNameBuf = se.getCompactStringBuffer(se.params.ClientName)
	copy(sendBuffer[currentIndex:], clientNameBuf)
	currentIndex += len(clientNameBuf)

	//Write client version
	binary.LittleEndian.PutUint32(sendBuffer[currentIndex:], uint32(se.params.ClientVersion))
	currentIndex += 4 //sizeof uint32

	//Write running OS
	sendBuffer[currentIndex] = byte(se.params.RunningOs)
	currentIndex++

	//Write language
	var languageBuf = se.getCompactStringBuffer(se.params.Language)
	copy(sendBuffer[currentIndex:], languageBuf)
	currentIndex += len(languageBuf)

	if se.params.ClientVersion >= 3000 {
		//Write Device ID
		binary.LittleEndian.PutUint32(sendBuffer[currentIndex:], uint32(se.params.GpuDeviceId))
		currentIndex += 4 //sizeof uint32

		//Write Vendor ID
		binary.LittleEndian.PutUint32(sendBuffer[currentIndex:], uint32(se.params.GpuVendorId))
		currentIndex += 4 //sizeof uint32

		//Write CPU Cycles
		binary.LittleEndian.PutUint32(sendBuffer[currentIndex:], uint32(se.params.CpuCycles))
		currentIndex += 4 //sizeof uint32

		//Write "Running CPUs"
		sendBuffer[currentIndex] = byte(se.params.RunningCpus)
		currentIndex++
	}

	se.sendBuffer(sendBuffer[0:currentIndex])
}

func (se *UTMSServerListEngine) sendBuffer(buffer []byte) {
	lengthBuffer := make([]byte, 4)
	binary.LittleEndian.PutUint32(lengthBuffer, uint32(len(buffer)))

	// _, sendLenErr := se.connection.Write(lengthBuffer)
	// if sendLenErr != nil {
	//     println("Failed to send length buffer:", sendLenErr.Error())
	//     return
	// }

	writeBuffer := append(lengthBuffer, buffer...)

	_, sendErr := se.connection.Write(writeBuffer)
	if sendErr != nil {
		println("Failed to send buffer:", sendErr.Error())
		return
	}
}

func (se *UTMSServerListEngine) Shutdown() {
	if se.connection != nil {
		se.connection.Close()
	}
}
