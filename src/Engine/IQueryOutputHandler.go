package Engine
import "net"

type IQueryOutputHandler interface {

	// Called when a UDP server responds
	// TODO: have server rules (hostname, etc)
	OnServerInfoResponse(sourceAddress net.Addr, serverProperties map[string]string)

	SetParams(params interface{})
}