package QR2

// #cgo CFLAGS: -g -Wall
// #include <stdlib.h>
// #include "enctypex_decoder.h"
import "C"

import (
	"encoding/binary"
	"log"
	"net"
	"net/netip"
	"os-serverlist-sync/Engine"
	"time"
	"unsafe"
)

const (
	ENCTYPEX_DATA_LEN uint = 261 //update 261 refs in this time too!
)

const (
	KEYTYPE_STRING = 0
	KEYTYPE_BYTE   = 1
	KEYTYPE_SHORT  = 2
)

const (
	UNSOLICITED_UDP_FLAG          uint8 = 1
	PRIVATE_IP_FLAG                     = 2
	CONNECT_NEGOTIATE_FLAG              = 4
	ICMP_IP_FLAG                        = 8
	NONSTANDARD_PORT_FLAG               = 16
	NONSTANDARD_PRIVATE_PORT_FLAG       = 32
	HAS_KEYS_FLAG                       = 64
	HAS_FULL_RULES_FLAG                 = 128
)

type ServerListEngineParams struct {
	ServerAddress string `json:"address"`
	Gamename      string `json:"gamename"`
	Secretkey     string `json:"secretkey"`
	QueryGamename string `json:"query_gamename"`

	//we don't want fields really... but we need to query them since some MSes won't send a proper response without it
	Fields string `json:"fields"`
}

type ServerListEngine struct {
	connection    *net.TCPConn
	queryEngine   Engine.IQueryEngine
	params        *ServerListEngineParams
	monitor       Engine.SyncStatusMonitor
	challenge     []byte
	enctypeXData  unsafe.Pointer
	enctypeXReady bool
}

type FieldKeyInfo struct {
	Name string
	Type uint8
}

func (se *ServerListEngine) SetQueryEngine(engine Engine.IQueryEngine) {
	se.queryEngine = engine
}

func (se *ServerListEngine) SetParams(params interface{}) {
	se.params = params.(*ServerListEngineParams)
}

func (se *ServerListEngine) Invoke(monitor Engine.SyncStatusMonitor) {
	se.monitor = monitor
	se.enctypeXReady = false
	monitor.BeginServerListEngine(se)
	se.queryEngine.SetMonitor(monitor)

	log.Println("Invoke " + se.params.ServerAddress)

	conn, dialErr := net.DialTimeout("tcp", se.params.ServerAddress, 15*time.Second)

	if dialErr != nil {
		log.Println("Dial failed:", dialErr.Error())
		se.monitor.EndServerListEngine(se)
		return
	}
	se.connection = conn.(*net.TCPConn)

	//wait for TCP reply, etc
	se.think()
}

func (se *ServerListEngine) waitForDataOfLen(len int) []byte {
	dataBuffer := make([]byte, len)

	var totalRead int = 0
	for {
		var remaining int = len - totalRead
		if remaining <= 0 {
			break
		}
		len, err := se.connection.Read(dataBuffer[totalRead:remaining])
		if err != nil {
			log.Printf("SBV2 Read error %s\n", err.Error())
			se.monitor.EndServerListEngine(se)
			break
		}

		totalRead += len
	}

	if se.enctypeXReady { //decrypt data
		buff := C.CBytes(dataBuffer)
		C.enctypex_func6((*C.uchar)(se.enctypeXData), (*C.uchar)(buff), C.int(totalRead))
		defer C.free(unsafe.Pointer(buff))

		dataBuffer = C.GoBytes(buff, C.int(totalRead))

	}

	return dataBuffer
}

func (se *ServerListEngine) enctypex_init(randomBuff []byte, keyBuf []byte) {
	se.enctypeXData = C.malloc(C.sizeof_char * 261) //will not accept ENCTYPEX_DATA_LEN...

	secretkey := C.CBytes([]byte(se.params.Secretkey))
	cKeyLen := C.int(len(keyBuf))
	challenge := C.CBytes(se.challenge)
	cKeyBuff := C.CBytes(keyBuf)

	defer C.free(unsafe.Pointer(secretkey))
	defer C.free(unsafe.Pointer(challenge))
	defer C.free(unsafe.Pointer(cKeyBuff))

	C.enctypex_funcx((*C.uchar)(se.enctypeXData), (*C.uchar)(secretkey),
		(*C.uchar)(challenge), (*C.uchar)(cKeyBuff), cKeyLen)

	se.enctypeXReady = true

}

func (se *ServerListEngine) waitForCryptHeader() {
	var cryptRandomLenBuff = se.waitForDataOfLen(1)
	var cryptLen uint8 = cryptRandomLenBuff[0] ^ 0xEC
	var cryptRandom = se.waitForDataOfLen(int(cryptLen))

	var cryptKeyLenBuff = se.waitForDataOfLen(1)
	var cryptKeyLen uint8 = cryptKeyLenBuff[0] ^ 0xEA
	var cryptKey = se.waitForDataOfLen(int(cryptKeyLen))

	se.enctypex_init(cryptRandom, cryptKey)

}

func (se *ServerListEngine) ReadNTS() string {
	var value string
	for {
		var charBuff = se.waitForDataOfLen(1)
		if charBuff[0] == 0 {
			break
		}
		value += string(charBuff[0])
	}
	return value
}

func (se *ServerListEngine) readFields() []FieldKeyInfo {
	var result []FieldKeyInfo = nil
	var numFieldsBuff = se.waitForDataOfLen(1)
	var numFields int = int(numFieldsBuff[0])

	for i := 0; i < numFields; i++ {
		var typeBuff = se.waitForDataOfLen(1)
		var info FieldKeyInfo
		info.Type = typeBuff[0]

		info.Name = se.ReadNTS()
		result = append(result, info)
	}
	return result
}

func (se *ServerListEngine) readListResponse() {
	_ = se.waitForDataOfLen(4) //skip pub ipv4 info

	var portBuff = se.waitForDataOfLen(2)
	var defaultPort = binary.BigEndian.Uint16(portBuff)

	var fields = se.readFields()

	var numPopularBuff = se.waitForDataOfLen(1)
	if numPopularBuff[0] != 0 {
		log.Printf("SBV2 Got unsupported popular values of size %d", numPopularBuff[0])
		se.monitor.EndServerListEngine(se)
		return
	}

	for {
		var flagsBuff = se.waitForDataOfLen(1)
		ipBuff := se.waitForDataOfLen(4)
		if ipBuff[0] == 0xff && ipBuff[1] == 0xff && ipBuff[2] == 0xff && ipBuff[3] == 0xff {
			break
		}
		publicIp, _ := netip.AddrFromSlice(ipBuff)
		//log.Println("Got serv ip", ip)
		//log.Println("flags: ", flagsBuff[0])

		var flags uint8 = flagsBuff[0]

		var port uint16 = defaultPort

		if flags&NONSTANDARD_PORT_FLAG != 0 {
			var portBuff = se.waitForDataOfLen(2)
			port = binary.BigEndian.Uint16(portBuff)
		}
		if flags&PRIVATE_IP_FLAG != 0 {
			_ = se.waitForDataOfLen(4)
			//privateIp, _ := netip.AddrFromSlice(privateIPBuff)
			//log.Println("Private IP: ", privateIp)
		}
		if flags&NONSTANDARD_PRIVATE_PORT_FLAG != 0 {
			_ = se.waitForDataOfLen(2)
			//_ = binary.BigEndian.Uint16(privatePortBuff)
			//log.Println("Private port: ", priateport)
		}
		if flags&ICMP_IP_FLAG != 0 {
			_ = se.waitForDataOfLen(4)
		}

		//we just need to skip this data since we get it from QR2 probes
		if flags&HAS_KEYS_FLAG != 0 {
			for _, v := range fields {
				switch v.Type {
				case KEYTYPE_STRING:
					stringIndexBuff := se.waitForDataOfLen(1)
					if stringIndexBuff[0] != 0xff {
						log.Printf("SBV2 Unhandled string index %d", stringIndexBuff[0])
						return
					} else {
						se.ReadNTS() //skip string
					}
				case KEYTYPE_BYTE:
					_ = se.waitForDataOfLen(1)
				case KEYTYPE_SHORT:
					_ = se.waitForDataOfLen(2)
				}
			}
		}
		if flags&HAS_FULL_RULES_FLAG != 0 {
			for {
				var nts = se.ReadNTS() //key
				if len(nts) == 0 {
					break
				}
				se.ReadNTS() //value
			}
		}

		var serverAddr netip.AddrPort = netip.AddrPortFrom(publicIp, port)
		if se.monitor.BeginQuery(se, se.queryEngine, serverAddr) {
			se.queryEngine.Query(serverAddr)
		}
	}

}

func (se *ServerListEngine) writeListRequest() {
	sendBuffer := make([]byte, 256)
	var currentIndex int = 2 //skip length

	sendBuffer[currentIndex] = 0 //SERVER_LIST_REQUEST
	currentIndex += 1

	sendBuffer[currentIndex] = 1 //protocol version
	currentIndex += 1

	sendBuffer[currentIndex] = 3 //encoding version
	currentIndex += 1

	//game version
	binary.BigEndian.PutUint32(sendBuffer[currentIndex:], 0)
	currentIndex += 4 //sizeof uint32

	//query for
	var gamename = []byte(se.params.QueryGamename)
	copy(sendBuffer[currentIndex:], gamename)
	currentIndex += len(gamename) + 1

	//query from
	gamename = []byte(se.params.Gamename)
	copy(sendBuffer[currentIndex:], gamename)
	currentIndex += len(gamename) + 1

	//challenge (always 8 bytes)
	se.challenge = []byte("12345678")
	copy(sendBuffer[currentIndex:], se.challenge)
	currentIndex += 8

	//filter
	sendBuffer[currentIndex] = 0
	currentIndex += 1

	//key list
	var fields = []byte(se.params.Fields)
	copy(sendBuffer[currentIndex:], fields)
	currentIndex += len(fields) + 1

	//options
	binary.BigEndian.PutUint32(sendBuffer[currentIndex:], 1)
	currentIndex += 4 //sizeof uint32

	binary.BigEndian.PutUint16(sendBuffer[0:2], uint16(currentIndex))

	_, err := se.connection.Write(sendBuffer[0:currentIndex])
	if err != nil {
		log.Println("Failed to write SBV2 Auth Query:", err.Error())
		se.monitor.EndServerListEngine(se)
		return
	}

	se.waitForCryptHeader()

	se.readListResponse()

}

func (se *ServerListEngine) think() {

	defer se.connection.Close()

	se.writeListRequest()

}

func (se *ServerListEngine) Shutdown() {
	if se.connection != nil {
		se.connection.Close()
	}
	if se.enctypeXReady {
		C.free(se.enctypeXData)
	}
}
