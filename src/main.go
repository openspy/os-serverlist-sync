package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"os-serverlist-sync/Config"
	"time"
)

const (
	SHUTDOWN_TIME_SECS int = 120
)

func invokeMsEngines(params []Config.EngineConfiguration) {

	for _, engine := range params {
		engine.ServerListEngine.Invoke()
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

	invokeMsEngines(params)

	for {
		select {
		case <-time.After(time.Duration(SHUTDOWN_TIME_SECS) * time.Second):
			shutdownEngines(params)
			return
		}
	}
}
