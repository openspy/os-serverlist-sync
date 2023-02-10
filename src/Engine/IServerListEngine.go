package Engine

type IServerListEngine interface {
	SetQueryEngine(engine IQueryEngine)
	SetParams(params interface{})
	Invoke()
	Shutdown()
}