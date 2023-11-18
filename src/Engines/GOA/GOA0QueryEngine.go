package GOA

import (
	"fmt"
	"net"
	"net/netip"
	"os"
	"os-serverlist-sync/Engine"
	"strings"
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
		println("GOA QueryEngine bind failed:", err.Error())
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
	qe.connection.WriteToUDP([]byte("\\status\\"), addr)
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

		buf[len] = 0

		if qe.outputHandler != nil {
			serverProps := strings.Split(string(buf), "\\")
			propMap := make(map[string]string)

			var lastKey int
			for idx, v := range serverProps[1:] {
				if idx%2 == 0 {
					lastKey = idx
				} else {
					var keyName = serverProps[lastKey+1]
					if keyName == "final" || keyName == "queryid" { //end of data stream
						break
					}

					propMap[keyName] = v
				}
			}
			qe.outputHandler.OnServerInfoResponse(addr, propMap)
		}
	}
}

func (qe *QueryEngine) Shutdown() {
	qe.connection.Close()
}
