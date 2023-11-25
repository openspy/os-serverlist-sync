package Engine

import (
	"container/list"
	"log"
	"net/netip"
	"time"
)

type QueryEngineListItem struct {
	address       netip.AddrPort
	engine        IQueryEngine
	listEngine    IServerListEngine
	lastPerformed time.Time
	numAttempts   int
}

const (
	MAX_ATTEMPTS  int = 5
	RETRY_SECONDS     = 30
)

type SyncStatusMonitor struct {
	serverEngineList *list.List
	queryList        *list.List
}

func (m *SyncStatusMonitor) Init() {
	m.serverEngineList = list.New()
	m.queryList = list.New()
}

func (m *SyncStatusMonitor) BeginServerListEngine(engine IServerListEngine) {
	m.serverEngineList.PushFront(engine)
}

func (m *SyncStatusMonitor) EndServerListEngine(engine IServerListEngine) {
	for element := m.serverEngineList.Front(); element != nil; element = element.Next() {
		var c IServerListEngine = element.Value.(IServerListEngine)
		if c == engine {
			m.serverEngineList.Remove(element)
		}
	}
}
func (m *SyncStatusMonitor) engineHasPendingQueries(listEngine IServerListEngine) bool {
	for element := m.queryList.Front(); element != nil; element = element.Next() {
		var c *QueryEngineListItem = element.Value.(*QueryEngineListItem)
		if c.listEngine == listEngine {
			return true
		}
	}
	return false
}
func (m *SyncStatusMonitor) BeginQuery(listEngine IServerListEngine, engine IQueryEngine, address netip.AddrPort) bool {

	//check for duplicate entry
	var addr = address.Addr().As4()
	var ip4Addr = netip.AddrFrom4(addr) //remove ipv6 portion :ffff: (which makes it different)

	for element := m.queryList.Front(); element != nil; element = element.Next() {
		var c *QueryEngineListItem = element.Value.(*QueryEngineListItem)
		if c.engine == engine && c.address.Addr().Compare(ip4Addr) == 0 && c.address.Port() == address.Port() {
			return false
		}
	}

	//no duplicate... proceed

	var queryItem = &QueryEngineListItem{}
	queryItem.address = address
	queryItem.engine = engine
	queryItem.listEngine = listEngine
	queryItem.lastPerformed = time.Now()
	queryItem.numAttempts = 1

	m.queryList.PushFront(queryItem)
	return true
}

func (m *SyncStatusMonitor) CompleteQuery(engine IQueryEngine, address netip.AddrPort) {
	var addr = address.Addr().As4()
	var ip4Addr = netip.AddrFrom4(addr) //remove ipv6 portion :ffff: (which makes it different)

	var slEngine IServerListEngine = nil

	for element := m.queryList.Front(); element != nil; element = element.Next() {
		var c *QueryEngineListItem = element.Value.(*QueryEngineListItem)
		if c.engine == engine && c.address.Addr().Compare(ip4Addr) == 0 && c.address.Port() == address.Port() {
			m.queryList.Remove(element)
			slEngine = c.listEngine
		}
	}

	if slEngine != nil {
		if !m.engineHasPendingQueries(slEngine) {
			m.EndServerListEngine(slEngine)
		}
	}
}

func (m *SyncStatusMonitor) AllEnginesComplete() bool {
	return m.serverEngineList.Len() == 0 && m.queryList.Len() == 0
}

func (m *SyncStatusMonitor) Think() {
	var toComplete []*QueryEngineListItem
	now := time.Now()
	for element := m.queryList.Front(); element != nil; element = element.Next() {
		var c *QueryEngineListItem = element.Value.(*QueryEngineListItem)
		var diff = now.Sub(c.lastPerformed)

		var max = time.Duration(RETRY_SECONDS) * time.Second
		if c.numAttempts > MAX_ATTEMPTS {
			toComplete = append(toComplete, c)
		} else if diff > max {
			c.lastPerformed = time.Now()
			c.numAttempts = c.numAttempts + 1
			c.engine.Query(c.address)
		}
	}
	for _, c := range toComplete {
		m.CompleteQuery(c.engine, c.address)
		log.Printf("abandon query: %s\n", c.address.String())
	}
}
