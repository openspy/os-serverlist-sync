package SAMP

import (
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os-serverlist-sync/Engine"
	"strconv"
)

type QueryEngineParams struct {
	SourcePort uint16 `json:"source_port"`
}

type QueryEngine struct {
	params        *QueryEngineParams
	connection    *net.UDPConn
	outputHandler Engine.IQueryOutputHandler
}

func (qe *QueryEngine) SetParams(params interface{}) {
	qe.params = params.(*QueryEngineParams)

	addr := net.UDPAddr{
		Port: int(qe.params.SourcePort),
		IP:   net.ParseIP("0.0.0.0"),
	}
	ser, err := net.ListenUDP("udp", &addr)
	if err != nil {
		println("SAMP QueryEngine bind failed:", err.Error())
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
	fmt.Printf("Send query to: %s\n", addr.String())
	writeBuffer := make([]byte, 11)
	writeBuffer[0] = 0x53
	writeBuffer[1] = 0x41
	writeBuffer[2] = 0x4D
	writeBuffer[3] = 0x50

	var ipv4_addr = destination.Addr().As4()

	writeBuffer[4] = ipv4_addr[0]
	writeBuffer[5] = ipv4_addr[1]
	writeBuffer[6] = ipv4_addr[2]
	writeBuffer[7] = ipv4_addr[3]

	binary.LittleEndian.PutUint16(writeBuffer[8:10], uint16(destination.Port()))

	writeBuffer[10] = 'i'

	qe.connection.WriteToUDP(writeBuffer, addr)
}

func (qe *QueryEngine) listen() {
	defer qe.connection.Close()

	buf := make([]byte, 1492)
	for {
		len, addr, err := qe.connection.ReadFrom(buf)
		if err != nil {
			println("GOA Recvfrom failed:", err.Error())
			break
		}

		if len < 11 {
			continue
		}

		if qe.outputHandler != nil {
			propMap := make(map[string]string)

			if buf[0] != 0x53 || buf[1] != 0x41 || buf[2] != 0x4D || buf[3] != 0x50 {
				continue
			}

			var offset = 11
			if buf[offset] == 0 {
				propMap["password"] = "0"
			}
			offset += 1

			var players = binary.LittleEndian.Uint16(buf[offset:])
			propMap["numplayers"] = strconv.Itoa(int(players))
			offset += 2

			players = binary.LittleEndian.Uint16(buf[offset:])
			propMap["maxplayers"] = strconv.Itoa(int(players))
			offset += 2

			var hostname_len = int(binary.LittleEndian.Uint32(buf[offset:]))
			offset += 4
			propMap["hostname"] = string(buf[offset : offset+hostname_len])
			offset += int(hostname_len)

			var gamemode_len = int(binary.LittleEndian.Uint32(buf[offset:]))
			offset += 4
			propMap["gamemode"] = string(buf[offset : offset+gamemode_len])
			offset += int(gamemode_len)

			var language_len = int(binary.LittleEndian.Uint32(buf[offset:]))
			offset += 4
			propMap["language"] = string(buf[offset : offset+language_len])
			offset += int(language_len)

			qe.outputHandler.OnServerInfoResponse(addr, propMap)
		}
	}
}

func (qe *QueryEngine) Shutdown() {
	qe.connection.Close()
}
