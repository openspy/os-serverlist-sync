package GameServerListerApi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"os-serverlist-sync/Engine"
)

type GameServerListerApiEngineParams struct {
	Url string
}

type GameServerListerApiEngine struct {
	queryEngine Engine.IQueryEngine
	params      *GameServerListerApiEngineParams

	monitor   Engine.SyncStatusMonitor
	ctx       context.Context
	ctxCancel context.CancelCauseFunc
}

func (se *GameServerListerApiEngine) SetQueryEngine(engine Engine.IQueryEngine) {
	se.queryEngine = engine
}

func (se *GameServerListerApiEngine) SetParams(params interface{}) {
	se.params = params.(*GameServerListerApiEngineParams)
}

type ServerEntry struct {
	IP        string `json:"ip"`
	QueryPort int    `json:"queryPort"`
}

func (se *GameServerListerApiEngine) Invoke(monitor Engine.SyncStatusMonitor, parentCtx context.Context) {
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
			addr, addrErr := netip.ParseAddr(entry.IP)
			if addrErr != nil {
				continue
			}
			addrPort = netip.AddrPortFrom(addr, uint16(entry.QueryPort))
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

func (se *GameServerListerApiEngine) Shutdown() {

}
