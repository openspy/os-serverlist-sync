package Engines

import (
	"bufio"
	"fmt"
	"log"
	"net/netip"
	"os"
	"os-serverlist-sync/Engine"
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

func (se *TextFileServerListEngine) Invoke() {
	file, err := os.Open(se.params.FilePath)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// optionally, resize scanner's capacity for lines over 64K, see next example
	for scanner.Scan() {
		fmt.Println(scanner.Text())

		addr, parseErr := netip.ParseAddrPort(scanner.Text())
		if parseErr != nil {
			log.Printf("Failed to parse address %s\n", scanner.Text())
			continue
		}

		se.queryEngine.Query(addr)
	}

	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
}

func (se *TextFileServerListEngine) Shutdown() {

}
