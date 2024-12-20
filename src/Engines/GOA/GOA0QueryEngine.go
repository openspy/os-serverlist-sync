package GOA

import (
	"log"
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
		log.Println("GOA QueryEngine bind failed:", err.Error())
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
	log.Printf("GOA Send query to: %s\n", addr.String())
	qe.connection.WriteToUDP([]byte("\\status\\"), addr)
}

func (qe *QueryEngine) listen() {
	defer qe.connection.Close()

	buf := make([]byte, 1492)
	for {
		len, addr, err := qe.connection.ReadFrom(buf)
		if err != nil {
			log.Println("GOA Recvfrom failed:", err.Error())
			break
		}

		var inputStr = string(buf[:len])

		if qe.outputHandler != nil {
			serverProps := strings.Split(inputStr, "\\")
			propMap := make(map[string]string)

			var lastKey int
			for idx, v := range serverProps[1:] {
				if idx%2 == 0 {
					lastKey = idx
				} else {
					var keyName = serverProps[lastKey+1]
					if keyName == "final" || keyName == "queryid" { //end of data stream
						continue
					}

					propMap[keyName] = v
				}
			}
			if qe.outputHandler != nil {
				qe.outputHandler.OnServerInfoResponse(addr, propMap)
			}
			qe.monitor.CompleteQuery(qe, addr.(*net.UDPAddr).AddrPort())
		}
	}
}

func (qe *QueryEngine) Shutdown() {
	qe.connection.Close()
}

func (qe *QueryEngine) SetMonitor(monitor Engine.SyncStatusMonitor) {
	qe.monitor = monitor
}
