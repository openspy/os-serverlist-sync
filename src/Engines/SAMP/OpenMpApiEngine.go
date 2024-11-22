package SAMP

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"os-serverlist-sync/Engine"
)

type OpenMpApiEngineParams struct {
	Url string
}

type OpenMpApiEngine struct {
	queryEngine Engine.IQueryEngine
	params      *OpenMpApiEngineParams

	monitor   Engine.SyncStatusMonitor
	ctx       context.Context
	ctxCancel context.CancelCauseFunc
}

func (se *OpenMpApiEngine) SetQueryEngine(engine Engine.IQueryEngine) {
	se.queryEngine = engine
}

func (se *OpenMpApiEngine) SetParams(params interface{}) {
	se.params = params.(*OpenMpApiEngineParams)
}

type ServerEntry struct {
	IP string `json:"ip"`
}

func (se *OpenMpApiEngine) Invoke(monitor Engine.SyncStatusMonitor, parentCtx context.Context) {
	ctx, cancel := context.WithCancelCause(parentCtx)
	se.ctx = ctx
	se.ctxCancel = cancel

	se.monitor = monitor

	monitor.BeginServerListEngine(se)
	se.queryEngine.SetMonitor(monitor)

	client := http.DefaultClient

	go func() {
		req, err := http.NewRequest("GET", se.params.Url, nil)
		if err != nil {
			fmt.Println("Got HTTP error", err)
			se.ctxCancel(err)
			return
		}

		req = req.WithContext(se.ctx)

		res, err := client.Do(req)
		if err != nil {
			fmt.Println("Got HTTP error", err)
			se.ctxCancel(err)
			return
		}

		data, err := io.ReadAll(res.Body)
		if err != nil {
			se.ctxCancel(err)
			return
		}

		var params []ServerEntry
		json.Unmarshal(data, &params)

		for _, entry := range params {
			var addrPort netip.AddrPort
			addrPort, addrErr := netip.ParseAddrPort(entry.IP)
			if addrErr != nil {
				continue
			}
			if monitor.BeginQuery(se, se.queryEngine, addrPort) {
				se.queryEngine.Query(addrPort)
			}
		}

		se.ctxCancel(nil)
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

func (se *OpenMpApiEngine) Shutdown() {

}
