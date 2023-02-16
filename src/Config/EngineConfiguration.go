package Config

import (
	"encoding/json"
	"log"
	"os-serverlist-sync/Engine"
	"os-serverlist-sync/Engines"
	"os-serverlist-sync/Engines/GOA"
	"os-serverlist-sync/Engines/OpenSpy"
	"os-serverlist-sync/Engines/UT2K"
)

type EngineConfiguration struct {
	QueryEngine        Engine.IQueryEngine
	ServerListEngine   Engine.IServerListEngine
	QueryOutputHandler Engine.IQueryOutputHandler
}

type MsEngineBlock struct {
	Name   string      `json:"name"`
	Params interface{} `json:"params"`
}

type QueryEngineBlock struct {
	Name   string      `json:"name"`
	Params interface{} `json:"params"`
}

type OutputEngineBlock struct {
	Name   string      `json:"name"`
	Params interface{} `json:"params"`
}

type EngineConfigurationPlain struct {
	MsEngine     MsEngineBlock     `json:"MsEngine"`
	QueryEngine  QueryEngineBlock  `json:"QueryEngine"`
	OutputEngine OutputEngineBlock `json:"OutputEngine"`
}

func (b *MsEngineBlock) UnmarshalJSON(data []byte) error {

	var typ struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &typ); err != nil {
		log.Printf("got err: %s\n", err)
		return err
	}

	switch typ.Name {
	case "goa0":
		b.Params = new(GOA.ServerListEngineParams)
		break
	case "ut2k":
		b.Params = new(UT2K.UTMSServerListEngineParams)
		break
	case "file":
		b.Params = new(Engines.TextFileServerListEngineParams)
		break
	}

	type tmp MsEngineBlock // avoids infinite recursion
	return json.Unmarshal(data, (*tmp)(b))
}

func (b *QueryEngineBlock) UnmarshalJSON(data []byte) error {

	var typ struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &typ); err != nil {
		log.Printf("got err: %s\n", err)
		return err
	}

	switch typ.Name {
	case "goa0":
		b.Params = new(GOA.QueryEngineParams)
		break
	case "ut2k":
		b.Params = new(UT2K.QueryEngineParams)
		break
	}

	type tmp QueryEngineBlock // avoids infinite recursion
	return json.Unmarshal(data, (*tmp)(b))
}

func (b *OutputEngineBlock) UnmarshalJSON(data []byte) error {

	var typ struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &typ); err != nil {
		log.Printf("got err: %s\n", err)
		return err
	}

	switch typ.Name {
	case "OSRedisOutput":
		b.Params = new(OpenSpy.OpenSpyRedisOutputHandlerParams)
		break
	}

	type tmp QueryEngineBlock // avoids infinite recursion
	return json.Unmarshal(data, (*tmp)(b))
}

func (b *EngineConfiguration) UnmarshalJSON(data []byte) error {
	var typ EngineConfigurationPlain

	if err := json.Unmarshal(data, &typ); err != nil {
		log.Printf("got err: %s\n", err)
		return err
	}

	switch typ.MsEngine.Name {
	case "goa0":
		b.ServerListEngine = &GOA.ServerListEngine{}
	case "ut2k":
		b.ServerListEngine = &UT2K.UTMSServerListEngine{}
	case "file":
		b.ServerListEngine = &Engines.TextFileServerListEngine{}
	}
	b.ServerListEngine.SetParams(typ.MsEngine.Params)

	switch typ.QueryEngine.Name {
	case "goa0":
		b.QueryEngine = &GOA.QueryEngine{}
	case "ut2k":
		b.QueryEngine = &UT2K.QueryEngine{}
	}
	b.QueryEngine.SetParams(typ.QueryEngine.Params)

	switch typ.OutputEngine.Name {
	case "OSRedisOutput":
		b.QueryOutputHandler = &OpenSpy.OpenSpyRedisOutputHandler{}
	}

	if b.QueryOutputHandler != nil {
		b.QueryOutputHandler.SetParams(typ.OutputEngine.Params)
	}

	b.ServerListEngine.SetQueryEngine(b.QueryEngine)

	b.QueryEngine.SetOutputHandler(b.QueryOutputHandler)

	return nil
}
