package GameServerListerApi

import (
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

func (se *GameServerListerApiEngine) Invoke(monitor Engine.SyncStatusMonitor) {
	monitor.BeginServerListEngine(se)
	se.queryEngine.SetMonitor(monitor)

	res, err := http.Get(se.params.Url)
	if err != nil {
		fmt.Println("Got HTTP error", err)
		return
	}

	data, err := io.ReadAll(res.Body)
	if err != nil {
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

}

func (se *GameServerListerApiEngine) Shutdown() {

}
