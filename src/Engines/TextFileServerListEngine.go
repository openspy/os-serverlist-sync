package Engines

import (
	"bufio"
	"context"
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

	monitor   Engine.SyncStatusMonitor
	ctx       context.Context
	ctxCancel context.CancelCauseFunc
}

func (se *TextFileServerListEngine) SetQueryEngine(engine Engine.IQueryEngine) {
	se.queryEngine = engine
}

func (se *TextFileServerListEngine) SetParams(params interface{}) {
	se.params = params.(*TextFileServerListEngineParams)
}

func (se *TextFileServerListEngine) resolveAddr(host string) (net.IP, error) {
	var resolver net.Resolver

	names, err := resolver.LookupIP(se.ctx, "ip4", host)

	if err != nil {
		return nil, err
	}

	if len(names) < 1 {
		return nil, nil
	}
	return names[0], nil

}

func (se *TextFileServerListEngine) Invoke(monitor Engine.SyncStatusMonitor, parentCtx context.Context) {
	ctx, cancel := context.WithCancelCause(parentCtx)
	se.ctx = ctx
	se.ctxCancel = cancel

	se.monitor = monitor
	monitor.BeginServerListEngine(se)
	se.queryEngine.SetMonitor(monitor)

	go func() {
		file, err := os.Open(se.params.FilePath)
		if err != nil {
			log.Fatal(err)
			se.ctxCancel(err)
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		// optionally, resize scanner's capacity for lines over 64K, see next example
		for scanner.Scan() {
			log.Println(scanner.Text())
			var input = scanner.Text()
			var delim = strings.Index(input, ":")
			if delim == -1 {
				log.Printf("Missing port: %s\n", input)
				continue
			}

			var host = input[0:delim]
			var portstr = input[delim+1:]
			resolvedAddr, dnsErr := se.resolveAddr(host)
			if dnsErr != nil || resolvedAddr == nil {
				log.Printf("failed to resolve: %s\n", host)
				continue
			}

			port, _ := strconv.Atoi(portstr)

			var ipAddr = netip.AddrFrom4([4]byte(resolvedAddr.To4()))

			var addr = netip.AddrPortFrom(ipAddr, uint16(port))

			if monitor.BeginQuery(se, se.queryEngine, addr) {
				se.queryEngine.Query(addr)
			}
		}

		if err := scanner.Err(); err != nil {
			se.ctxCancel(err)
			log.Fatal(err)
		} else {
			se.ctxCancel(nil)
		}
	}()

	go func() {
		select {
		case <-se.ctx.Done():
			se.monitor.EndServerListEngine(se)
			se.Shutdown()
			return
		}
	}()
}

func (se *TextFileServerListEngine) Shutdown() {

}
