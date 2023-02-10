package Engine
import "net/netip"

type IQueryEngine interface {
	SetParams(params interface{})
	SetOutputHandler(handler IQueryOutputHandler)
	Query(address netip.AddrPort)
	Shutdown()
}