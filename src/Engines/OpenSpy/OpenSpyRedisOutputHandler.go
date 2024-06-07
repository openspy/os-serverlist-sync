package OpenSpy

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type OpenSpyRedisOutputHandlerParams struct {
	Gamename   string      `json:"gamename"`
	InjectKeys interface{} `json:"injectKeys"`
}

const (
	SERVER_EXPIRE_TIME_SECS int = 900
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

	var server_key = oh.getServerKey(udpAddr)

	if server_key == nil { //this was not an injected server?? just ignore
		return
	}

	log.Printf("Num keys (%s): %d (%s) (%s)\n", *server_key, len(serverProperties), sourceAddress.String(), fmt.Sprintf("%d", udpAddr.Port))

	//setup standard keys
	oh.redisClient.HSet(oh.context, *server_key, []string{
		"wan_ip", udpAddr.IP.String(),
		"wan_port", fmt.Sprintf("%d", udpAddr.Port),
		//there is an id property... but is it needed / used?
		"gameid", fmt.Sprintf("%d", oh.gameId),
		"allow_unsolicited_udp", "1",
		"injected", "1",
	})

	if oh.params.InjectKeys != nil {
		for k, v := range oh.params.InjectKeys.(map[string]interface{}) {
			serverProperties[k] = v.(string)
		}
	}

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

	oh.context = context.Background()
	oh.redisClient = rdb
	oh.params = params.(*OpenSpyRedisOutputHandlerParams)

	redisGameLookupOptions := &redis.Options{}
	*redisGameLookupOptions = *redisOptions
	redisGameLookupOptions.DB = 2
	tempConnection := redis.NewClient(redisGameLookupOptions)

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

	result, err := oh.redisClient.Incr(oh.context, "QRID").Result()
	if err != nil {
		log.Fatal(err)
	}
	server_key = fmt.Sprintf("%s_injected:%d:", oh.params.Gamename, result)

	return &server_key

}
