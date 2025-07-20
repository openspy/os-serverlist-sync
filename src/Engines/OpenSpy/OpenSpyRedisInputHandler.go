package OpenSpy

import (
	"context"
	"crypto/tls"
	"net/netip"
	"os"
	"os-serverlist-sync/Engine"
	"strconv"

	"github.com/redis/go-redis/v9"
)

type OpenSpyRedisInputHandlerParams struct {
	Gamename string
}

type OpenSpyRedisInputHandler struct {
	queryEngine Engine.IQueryEngine
	params      *OpenSpyRedisInputHandlerParams

	monitor     Engine.SyncStatusMonitor
	redisClient *redis.Client

	ctx       context.Context
	ctxCancel context.CancelCauseFunc

	gameId int
}

func (oh *OpenSpyRedisInputHandler) SetQueryEngine(engine Engine.IQueryEngine) {
	oh.queryEngine = engine
}

func (oh *OpenSpyRedisInputHandler) SetParams(params interface{}) {
	oh.params = params.(*OpenSpyRedisInputHandlerParams)
}

func (oh *OpenSpyRedisInputHandler) SetupRedis() {
	redisOptions := &redis.Options{
		Addr: os.Getenv("REDIS_SERVER"),
	}

	redisUsername := os.Getenv("REDIS_USERNAME")
	redisPassword := os.Getenv("REDIS_PASSWORD")

	if len(redisUsername) > 0 {
		redisOptions.Username = redisUsername
	}
	if len(redisPassword) > 0 {
		redisOptions.Password = redisPassword
	}

	redisUseTLS := os.Getenv("REDIS_USE_TLS")

	if len(redisUseTLS) > 0 {
		useTLSInt, _ := strconv.Atoi(redisUseTLS)

		if useTLSInt == 1 {
			tlsConfig := &tls.Config{
				MinVersion: tls.VersionTLS12,
				//InsecureSkipVerify: true,
				//Certificates: []tls.Certificate{cert}
			}

			redisSkipSSLVerify := os.Getenv("REDIS_INSECURE_TLS")
			if len(redisSkipSSLVerify) > 0 {
				useInsecureTLS, _ := strconv.Atoi(redisSkipSSLVerify)

				if useInsecureTLS == 1 {
					tlsConfig.InsecureSkipVerify = true
				}
			}
			redisOptions.TLSConfig = tlsConfig
		}
	}

	redisOptions.DB = 0
	rdb := redis.NewClient(redisOptions)

	oh.redisClient = rdb

	redisGameLookupOptions := &redis.Options{}
	*redisGameLookupOptions = *redisOptions
	redisGameLookupOptions.DB = 2
	tempConnection := redis.NewClient(redisGameLookupOptions)

	defer tempConnection.Close()

	//lookup gameid
	gkResult, _ := tempConnection.Get(oh.ctx, oh.params.Gamename).Result()
	gidResult, _ := tempConnection.HGet(oh.ctx, gkResult, "gameid").Result()

	gid, _ := strconv.Atoi(gidResult)

	oh.gameId = gid

	//set back to servers database
	oh.redisClient.Conn().Select(oh.ctx, 0)
}

func (oh *OpenSpyRedisInputHandler) Invoke(monitor Engine.SyncStatusMonitor, parentCtx context.Context) {

	ctx, cancel := context.WithCancelCause(parentCtx)
	oh.ctx = ctx
	oh.ctxCancel = cancel

	oh.SetupRedis()

	oh.monitor = monitor

	monitor.BeginServerListEngine(oh)
	oh.queryEngine.SetMonitor(monitor)

	var itemsToDelete []string

	var cursor uint64 = 0
	var keys []string
	var err error
	for {
		var scanCmd = oh.redisClient.ZScan(oh.ctx, oh.params.Gamename, cursor, "*", 50)
		keys, cursor, err = scanCmd.Result()
		if err != nil {
			break
		}

		for i := 0; i < len(keys); i += 2 {
			var key = keys[i]
			gameResults, gameError := oh.redisClient.HMGet(oh.ctx, key, "wan_ip", "wan_port", "injected").Result()
			if gameError != nil {
				continue
			}

			if gameResults[0] == nil || gameResults[1] == nil || gameResults[2] == nil {
				itemsToDelete = append(itemsToDelete, key)
				continue
			}

			var wanip = gameResults[0].(string)
			var wanport = gameResults[1].(string)
			var injected = gameResults[2].(string)

			if injected != "1" {
				continue
			}

			var addrPort netip.AddrPort
			addrPort, addrErr := netip.ParseAddrPort(wanip + ":" + wanport)
			if addrErr != nil {
				continue
			}
			if monitor.BeginQuery(oh, oh.queryEngine, addrPort) {
				oh.queryEngine.Query(addrPort)
			}
		}
		if cursor == 0 {
			break
		}
	}
	if len(itemsToDelete) > 0 {
		oh.redisClient.ZRem(oh.ctx, oh.params.Gamename, itemsToDelete)
	}

	oh.ctxCancel(nil)

	go func() {
		select {
		case <-oh.ctx.Done():
			oh.monitor.EndServerListEngine(oh)
			oh.Shutdown()
			return
		}
	}()
}

func (oh *OpenSpyRedisInputHandler) Shutdown() {

}
