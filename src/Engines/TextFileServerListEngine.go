package Engines

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"net/netip"
	"os"
	"os-serverlist-sync/Engine"
	"strconv"
	"strings"
)

type TextFileServerListEngineParams struct {
	FilePath string
}

type TextFileServerListEngine struct {
	queryEngine Engine.IQueryEngine
	params      *TextFileServerListEngineParams
}

func (se *TextFileServerListEngine) SetQueryEngine(engine Engine.IQueryEngine) {
	se.queryEngine = engine
}

func (se *TextFileServerListEngine) SetParams(params interface{}) {
	se.params = params.(*TextFileServerListEngineParams)
}

func (se *TextFileServerListEngine) Invoke(monitor Engine.SyncStatusMonitor) {
	monitor.BeginServerListEngine(se)
	se.queryEngine.SetMonitor(monitor)

	file, err := os.Open(se.params.FilePath)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// optionally, resize scanner's capacity for lines over 64K, see next example
	for scanner.Scan() {
		fmt.Println(scanner.Text())
		var input = scanner.Text()
		var delim = strings.Index(input, ":")
		if delim == -1 {
			log.Printf("Missing port: %s\n", input)
			continue
		}

		var host = input[0:delim]
		var portstr = input[delim+1:]
		resolvedAddr, dnsErr := net.ResolveIPAddr("ip4", host)
		if dnsErr != nil {
			log.Printf("failed to resolve: %s\n", host)
		}

		port, _ := strconv.Atoi(portstr)

		var ipAddr = netip.AddrFrom4([4]byte(resolvedAddr.IP.To4()))

		var addr = netip.AddrPortFrom(ipAddr, uint16(port))

		monitor.BeginQuery(se, se.queryEngine, addr)
		se.queryEngine.Query(addr)
	}

	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
}

func (se *TextFileServerListEngine) Shutdown() {

}
