package UT2K

import (
	"encoding/binary"
	"log"
	"net"
	"net/netip"
	"os"
	"os-serverlist-sync/Engine"
	"strconv"
)

type QueryEngineParams struct {
	SourcePort uint16 `json:"source_port"`
	VersionID  int    `json:"versionid"`
}

// Since this is UDP, do not associate the state with the engine itself! only pass by args!
type QueryParserState struct {
	TotalLength   int
	CurrentOffset int
	Buffer        []byte
}

const (
	UT2004_VERSION int = 128
	UT2003_VERSION     = 121
)

type QueryEngine struct {
	params        *QueryEngineParams
	connection    *net.UDPConn
	outputHandler Engine.IQueryOutputHandler
	monitor       Engine.SyncStatusMonitor
}

func (qe *QueryEngine) SetParams(params interface{}) {
	qe.params = params.(*QueryEngineParams)

	addr := net.UDPAddr{
		Port: int(qe.params.SourcePort),
		IP:   net.ParseIP("0.0.0.0"),
	}
	ser, err := net.ListenUDP("udp", &addr)
	if err != nil {
		println("UT2K QueryEngine bind failed:", err.Error())
		os.Exit(1)
	}

	qe.connection = ser

	go func() {
		qe.listen()
	}()
}

func (qe *QueryEngine) SetOutputHandler(handler Engine.IQueryOutputHandler) {
	qe.outputHandler = handler
}

func (qe *QueryEngine) Query(destination netip.AddrPort) {
	var addr = net.UDPAddrFromAddrPort(destination)

	writeBuffer := make([]byte, 5)
	binary.BigEndian.PutUint32(writeBuffer, uint32(qe.params.VersionID))

	writeBuffer[4] = 0x00 //basic info query

	qe.connection.WriteToUDP(writeBuffer, addr)
}

func (qe *QueryEngine) listen() {
	defer qe.connection.Close()

	buf := make([]byte, 1492)
	for {
		len, addr, err := qe.connection.ReadFrom(buf)
		if err != nil {
			println("UT2K Query Recvfrom failed:", err.Error())
			return
		}

		buf[len] = 0

		var state = &QueryParserState{}
		state.TotalLength = len
		state.CurrentOffset = 0
		state.Buffer = buf

		if qe.outputHandler != nil {
			qe.handleResponse(addr, state)
		}
	}
}

func (qe *QueryEngine) handleResponse(sourceAddress net.Addr, state *QueryParserState) {
	version := binary.LittleEndian.Uint32(state.Buffer[state.CurrentOffset:4])
	state.CurrentOffset += 4

	propMap := make(map[string]string)

	if int(version) != qe.params.VersionID {
		log.Printf("Unexpected version %d!", version)
		return
	}

	var queryType uint8 = state.Buffer[state.CurrentOffset]
	state.CurrentOffset++

	if queryType != 0 {
		log.Printf("Unexpected query response type!")
		return
	}

	state.CurrentOffset += 4 //server id

	qe.readCompactString(state) //skip address

	//gamePort := binary.LittleEndian.Uint32(state.Buffer[state.CurrentOffset:])
	state.CurrentOffset += 4 //maybe we want to trust this port... but for now lets just use the source address - 1

	//queryPort := binary.LittleEndian.Uint32(state.Buffer[state.CurrentOffset:])
	state.CurrentOffset += 4

	var hostname = qe.readCompactString(state)
	propMap["hostname"] = hostname

	var level = qe.readCompactString(state)
	propMap["mapname"] = level

	var gamegroup = qe.readCompactString(state)
	propMap["gametype"] = gamegroup

	numPlayers := binary.LittleEndian.Uint32(state.Buffer[state.CurrentOffset:])
	state.CurrentOffset += 4
	propMap["numplayers"] = strconv.Itoa(int(numPlayers))

	maxPlayers := binary.LittleEndian.Uint32(state.Buffer[state.CurrentOffset:])
	state.CurrentOffset += 4
	propMap["maxplayers"] = strconv.Itoa(int(maxPlayers))

	state.CurrentOffset += 4

	if int(version) == UT2004_VERSION {
		state.CurrentOffset += 4

		var skill = qe.readCompactString(state)
		propMap["botlevel"] = skill
	}

	//inject "calculated" properties
	if numPlayers < maxPlayers {
		propMap["freespace"] = "1"
	} else {
		propMap["freespace"] = "0"
	}
	propMap["currentplayers"] = propMap["numplayers"]

	//ideally we read this but for now, good enough
	propMap["standard"] = "true"
	propMap["nomutators"] = "false"

	//fake the query port info (maybe we want to trust what is in this packet, but for now just -1 it)
	var gamePortAddress = sourceAddress.(*net.UDPAddr)
	gamePortAddress.Port = gamePortAddress.Port - 1

	qe.outputHandler.OnServerInfoResponse(gamePortAddress, propMap)
	qe.monitor.CompleteQuery(qe, sourceAddress.(*net.UDPAddr).AddrPort())
}

func (qe *QueryEngine) readCompactInt(state *QueryParserState) int {
	var length int = 0
	var B [5]uint8
	B[0] = state.Buffer[state.CurrentOffset]
	state.CurrentOffset++
	if (B[0] & 0x40) != 0 {
		B[1] = state.Buffer[state.CurrentOffset]
		state.CurrentOffset++
		if (B[1] & 0x80) != 0 {
			B[2] = state.Buffer[state.CurrentOffset]
			state.CurrentOffset++
			if (B[2] & 0x80) != 0 {
				B[3] = state.Buffer[state.CurrentOffset]
				state.CurrentOffset++
				if (B[3] & 0x80) != 0 {
					B[4] = state.Buffer[state.CurrentOffset]
					state.CurrentOffset++
					length = int(B[4])
				}
				length = (length << 7) + int(B[3]&0x7f)
			}
			length = (length << 7) + int(B[2]&0x7f)
		}
		length = (length << 7) + int(B[1]&0x7f)
	}
	length = (length << 6) + int((B[0] & 0x3f))
	if (B[0] & 0x80) != 0 {
		length = -length
	}
	return length
}

func (qe *QueryEngine) readCompactString(state *QueryParserState) string {
	var length = qe.readCompactInt(state)
	var stringData string

	for i := 0; i < length; i++ {
		var ch byte = state.Buffer[state.CurrentOffset]
		state.CurrentOffset++

		if ch == 0x00 {
			break
		}

		if ch == 0x1B { //skip colour codes
			i += 3
			state.CurrentOffset += 3
			continue
		}

		stringData += string(ch)
	}
	return stringData
}

func (qe *QueryEngine) Shutdown() {
	qe.connection.Close()
}

func (qe *QueryEngine) SetMonitor(monitor Engine.SyncStatusMonitor) {
	qe.monitor = monitor
}
