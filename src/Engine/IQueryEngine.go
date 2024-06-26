package Engine

import "net/netip"

type IQueryEngine interface {
	SetMonitor(monitor SyncStatusMonitor)
	SetParams(params interface{})
	SetOutputHandler(handler IQueryOutputHandler)
	Query(address netip.AddrPort)
	Shutdown()
}
