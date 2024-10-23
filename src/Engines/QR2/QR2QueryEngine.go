package QR2

/*
	TODO: pre-query ip verify
	player keys
	team keys
*/

import (
	"log"
	"net"
	"net/netip"
	"os"
	"os-serverlist-sync/Engine"
)

type QueryEngineParams struct {
	PrequeryIpVerify bool   `json:"prequery_ip_verify"`
	SourcePort       uint16 `json:"source_port"`
}

type QueryEngine struct {
	params        *QueryEngineParams
	connection    *net.UDPConn
	outputHandler Engine.IQueryOutputHandler
	monitor       Engine.SyncStatusMonitor
}

// Since this is UDP, do not associate the state with the engine itself! only pass by args!
type QueryParserState struct {
	TotalLength   int
	CurrentOffset int
	Buffer        []byte
}

func (qe *QueryEngine) SetParams(params interface{}) {
	qe.params = params.(*QueryEngineParams)

	addr := net.UDPAddr{
		Port: int(qe.params.SourcePort),
		IP:   net.ParseIP("0.0.0.0"),
	}
	ser, err := net.ListenUDP("udp", &addr)
	if err != nil {
		log.Println("QR2 QueryEngine bind failed:", err.Error())
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
	log.Printf("QR2 Send query to: %s\n", addr.String())
	writeBuffer := make([]byte, 11)
	writeBuffer[0] = 0xfe
	writeBuffer[1] = 0xfd
	writeBuffer[7] = 0xff

	qe.connection.WriteToUDP(writeBuffer, addr)
}

func (qe *QueryEngine) readString(state *QueryParserState) string {
	var stringData string

	for i := state.CurrentOffset; i < state.TotalLength; i++ {
		var ch byte = state.Buffer[state.CurrentOffset]
		state.CurrentOffset++

		if ch == 0 {
			break
		}

		stringData += string(ch)
	}
	return stringData
}

func (qe *QueryEngine) listen() {
	defer qe.connection.Close()

	buf := make([]byte, 1492)
	for {
		bufLen, addr, err := qe.connection.ReadFrom(buf)
		if err != nil {
			log.Println("QR2 Recvfrom failed:", err.Error())
			break
		}

		if bufLen < 6 {
			continue
		}

		propMap := make(map[string]string)
		var udpAddr *net.UDPAddr = addr.(*net.UDPAddr)

		var state QueryParserState
		state.Buffer = buf

		state.TotalLength = bufLen
		state.CurrentOffset = 5 //skip key data for now

		for {
			var serverKey = qe.readString(&state)
			var serverValue = qe.readString(&state)

			if len(serverKey) == 0 {
				break
			}
			propMap[serverKey] = serverValue
		}

		if qe.outputHandler != nil {
			qe.outputHandler.OnServerInfoResponse(addr, propMap)
		}
		qe.monitor.CompleteQuery(qe, udpAddr.AddrPort())
	}
}

func (qe *QueryEngine) Shutdown() {
	qe.connection.Close()
}

func (qe *QueryEngine) SetMonitor(monitor Engine.SyncStatusMonitor) {
	qe.monitor = monitor
}
