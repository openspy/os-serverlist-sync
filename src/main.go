package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"os-serverlist-sync/Config"
	"os-serverlist-sync/Engine"
	"time"
)

const (
	SHUTDOWN_TIME_SECS int = 120
)

func invokeMsEngines(monitor Engine.SyncStatusMonitor, params []Config.EngineConfiguration) {

	for _, engine := range params {
		engine.ServerListEngine.Invoke(monitor)
	}
}

func shutdownEngines(params []Config.EngineConfiguration) {
	for _, engine := range params {
		engine.ServerListEngine.Shutdown()
	}
}

func main() {
	file, err := os.Open("ms_config.json")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	byteValue, _ := ioutil.ReadAll(file)

	var params []Config.EngineConfiguration
	json.Unmarshal(byteValue, &params)

	var monitor Engine.SyncStatusMonitor
	monitor.Init()

	invokeMsEngines(monitor, params)

	for {
		if monitor.AllEnginesComplete() {
			break
		}
		time.Sleep(2 * time.Second)
		monitor.Think()
	}

	shutdownEngines(params)
}
