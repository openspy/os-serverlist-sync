package OpenSpy

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type OpenSpyRedisOutputHandlerParams struct {
	Gamename string `json:"gamename"`
}

const (
	SERVER_EXPIRE_TIME_SECS int = 420
)

type OpenSpyRedisOutputHandler struct {
	params      *OpenSpyRedisOutputHandlerParams
	redisClient *redis.Client
	context     context.Context

	gameId int
}

func (oh *OpenSpyRedisOutputHandler) OnServerInfoResponse(sourceAddress net.Addr, serverProperties map[string]string) {
	//var existing server key (or create) -- create IPMAP too
	//create server keys
	//mark as "injected" server
	//create server cust keys
	//set expiry
	//add to gamename score set

	var udpAddr *net.UDPAddr = sourceAddress.(*net.UDPAddr)

	fmt.Printf("Num keys: %d (%s) (%s)\n", len(serverProperties), sourceAddress.String(), fmt.Sprintf("%d", udpAddr.Port))

	var server_key = oh.getServerKey(udpAddr)

	if server_key == nil { //this was not an injected server?? just ignore
		return
	}

	//setup standard keys
	oh.redisClient.HSet(oh.context, *server_key, []string{
		"wan_ip", udpAddr.IP.String(),
		"wan_port", fmt.Sprintf("%d", udpAddr.Port),
		//there is an id property... but is it needed / used?
		"gameid", fmt.Sprintf("%d", oh.gameId),
		"injected", "1",
	})

	//setup custom keys
	var custkeys_name = fmt.Sprintf("%scustkeys", *server_key)
	oh.redisClient.HSet(oh.context, custkeys_name, serverProperties)

	oh.redisClient.Expire(oh.context, *server_key, time.Duration(SERVER_EXPIRE_TIME_SECS)*time.Second)
	oh.redisClient.Expire(oh.context, custkeys_name, time.Duration(SERVER_EXPIRE_TIME_SECS)*time.Second)

	var ipmap_name = fmt.Sprintf("IPMAP_%s-%d", udpAddr.IP.String(), udpAddr.Port)
	oh.redisClient.Set(oh.context, ipmap_name, *server_key, time.Duration(SERVER_EXPIRE_TIME_SECS)*time.Second)
	oh.redisClient.Expire(oh.context, ipmap_name, time.Duration(SERVER_EXPIRE_TIME_SECS)*time.Second)

	oh.redisClient.ZIncrBy(oh.context, oh.params.Gamename, 1.0, *server_key)
}

func (oh *OpenSpyRedisOutputHandler) SetParams(params interface{}) {

	rdb := redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDIS_SERVER"),
		Username: os.Getenv("REDIS_USERNAME"),
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
		TLSConfig: &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: true,
			//Certificates: []tls.Certificate{cert}
		},
	})
	oh.context = context.Background()
	oh.redisClient = rdb
	oh.params = params.(*OpenSpyRedisOutputHandlerParams)

	tempConnection := redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDIS_SERVER"),
		Username: os.Getenv("REDIS_USERNAME"),
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       2,
		TLSConfig: &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: true,
			//Certificates: []tls.Certificate{cert}
		},
	})

	defer tempConnection.Close()

	//lookup gameid
	gkResult, _ := tempConnection.Get(oh.context, oh.params.Gamename).Result()
	gidResult, _ := tempConnection.HGet(oh.context, gkResult, "gameid").Result()

	gid, _ := strconv.Atoi(gidResult)

	oh.gameId = gid

	//set back to servers database
	oh.redisClient.Conn().Select(oh.context, 0)

}

/*
This will first:

	lookup a server by ip map
		if server by ip found, check its an injected server
			if not injected, return null
			if injected, return id
		if no key found
			create key + ip map, set as injected, and return server key
*/
func (oh *OpenSpyRedisOutputHandler) getServerKey(udpAddr *net.UDPAddr) *string {

	var server_key string

	var ipmap_name = fmt.Sprintf("IPMAP_%s-%d", udpAddr.IP.String(), udpAddr.Port)
	existsResult, _ := oh.redisClient.Exists(oh.context, ipmap_name).Result()

	if existsResult != 0 {

		server_key, _ := oh.redisClient.Get(oh.context, ipmap_name).Result()

		//check if the server is injected
		injectedResponse, _ := oh.redisClient.HExists(oh.context, server_key, "injected").Result()
		if !injectedResponse {
			return nil
		}

		return &server_key
	}

	result, _ := oh.redisClient.Incr(oh.context, "QRID").Result()
	server_key = fmt.Sprintf("%s_injected:%d:", oh.params.Gamename, result)

	return &server_key

}
