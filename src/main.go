package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"os"
	"os-serverlist-sync/Config"
	"os-serverlist-sync/Engine"
	"os-serverlist-sync/Engines/OpenSpy"
	"time"
)

func invokeMsEngines(monitor Engine.SyncStatusMonitor, params []Config.EngineConfiguration, ctx context.Context) {

	for _, engine := range params {
		engine.ServerListEngine.Invoke(monitor, ctx)
	}
}

func shutdownEngines(params []Config.EngineConfiguration) {
	for _, engine := range params {
		engine.ServerListEngine.Shutdown()
		engine.QueryEngine.Shutdown()
	}
}

func applyRefreshModeInputEngine(params []Config.EngineConfiguration) []Config.EngineConfiguration {
	for i := 0; i < len(params); i++ {
		var osEngine = params[i].QueryOutputHandler.(*OpenSpy.OpenSpyRedisOutputHandler)
		var gamename = osEngine.GetGamename()

		var inputParams = &OpenSpy.OpenSpyRedisInputHandlerParams{}
		inputParams.Gamename = gamename
		var inputEngine = OpenSpy.OpenSpyRedisInputHandler{}
		inputEngine.SetParams(inputParams)
		inputEngine.SetQueryEngine(params[i].QueryEngine)

		params[i].ServerListEngine = &inputEngine
	}
	return params
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	defer cancel()
	file, err := os.Open("ms_config.json")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	byteValue, _ := io.ReadAll(file)

	var params []Config.EngineConfiguration
	json.Unmarshal(byteValue, &params)

	//check if refresh mode, if so - replace input handlers
	var refreshMode = false
	args := os.Args
	if len(args) > 1 {
		refreshMode = args[1] == "--refresh-only" //this mode will only refresh existing injected servers, without querying msEngines
	}
	if refreshMode {
		params = applyRefreshModeInputEngine(params)
	}
	//

	var monitor Engine.SyncStatusMonitor
	monitor.Init()

	ticker := time.NewTicker(2 * time.Second)

	go func() {
		for _ = range ticker.C {
			monitor.Think()
			if monitor.AllEnginesComplete() {
				cancel()
				break
			}
		}
	}()

	invokeMsEngines(monitor, params, ctx)

	select {
	case <-ctx.Done():
		log.Println("Shutdown event", ctx.Err())
		ticker.Stop()
		break
	}

	shutdownEngines(params)
	log.Printf("Exiting server list syncer\n")
}
