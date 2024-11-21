package Engine

import "context"

type IServerListEngine interface {
	SetQueryEngine(engine IQueryEngine)
	SetParams(params interface{})
	Invoke(monitor SyncStatusMonitor, parentCtx context.Context)
	Shutdown()
}
