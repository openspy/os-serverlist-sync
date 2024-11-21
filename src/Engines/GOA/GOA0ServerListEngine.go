package GOA

// #cgo CFLAGS: -g -Wall
// #include <stdlib.h>
// #include "gsmsalg.h"
import "C"

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log"
	"net"
	"net/netip"
	"os-serverlist-sync/Engine"
	"strings"
	"time"
	"unsafe"
)

type ServerListEngineParams struct {
	ServerAddress    string `json:"address"`
	Gamename         string `json:"gamename"`
	Secretkey        string `json:"secretkey"`
	QueryGamename    string `json:"query_gamename"`
	NoCompressedList bool   `json:"no_compressed_list"`
	MaxChallengeLen  int    `json:"max_challenge_len"` //certain old master servers (UT) send invalid KV/data and only process up to 6 bytes anyways for the challenge
	GameVer          string `json:"gamever"`
	Location         string `json:"location"`
	AttachQueryID    bool   `json:"attach_queryid"`
	AttachListFinal  bool   `json:"attach_listfinal"`
}

type ServerListEngine struct {
	connection  *net.TCPConn
	queryEngine Engine.IQueryEngine
	params      *ServerListEngineParams
	monitor     Engine.SyncStatusMonitor

	ctx       context.Context
	ctxCancel context.CancelCauseFunc
}

func (se *ServerListEngine) SetQueryEngine(engine Engine.IQueryEngine) {
	se.queryEngine = engine
}

func (se *ServerListEngine) SetParams(params interface{}) {
	se.params = params.(*ServerListEngineParams)
}

func (se *ServerListEngine) Invoke(monitor Engine.SyncStatusMonitor, parentCtx context.Context) {

	ctx, cancel := context.WithCancelCause(parentCtx)
	se.ctx = ctx
	se.ctxCancel = cancel

	se.monitor = monitor
	monitor.BeginServerListEngine(se)
	se.queryEngine.SetMonitor(monitor)

	go func() {
		log.Println("Invoke " + se.params.ServerAddress)

		conn, dialErr := net.DialTimeout("tcp", se.params.ServerAddress, 15*time.Second)

		if dialErr != nil {
			log.Println("Dial failed:", dialErr.Error())
			cancel(dialErr)
			se.monitor.EndServerListEngine(se)
			return
		}
		se.connection = conn.(*net.TCPConn)

		//wait for TCP reply, etc
		se.think()

		cancel(nil)
	}()

	go func() {
		select {
		case <-se.ctx.Done():
			se.monitor.EndServerListEngine(se)
			se.Shutdown()
			return
		}
	}()

}

func (se *ServerListEngine) think() {

	defer se.connection.Close()

	reply := make([]byte, 32)

	_, err := se.connection.Read(reply)
	if err != nil {
		log.Println("Failed to read GOA SB Auth Request:", err.Error())
		se.ctxCancel(err)
		return
	}

	var challenge string

	var max_chal_len int = se.params.MaxChallengeLen

	var secure_idx int = strings.Index(string(reply), "secure\\")

	if secure_idx == -1 {
		log.Println("GOA Missing secure property")
		return
	}

	if max_chal_len != 0 {
		challenge = string(reply)[(secure_idx + 7) : (secure_idx+7)+max_chal_len]

	} else {
		challenge = string(reply)[(secure_idx + 7):]
	}

	//write authentication
	var validation_response = se.gsmsalg(challenge)

	var authQuery = "\\gamename\\" + se.params.Gamename

	if len(se.params.GameVer) > 0 {
		authQuery += "\\gamever\\" + se.params.GameVer
	}

	if len(se.params.Location) > 0 {
		authQuery += "\\location\\" + se.params.Location
	}

	authQuery += "\\validate\\" + validation_response + "\\final\\"

	if se.params.AttachQueryID {
		authQuery += "\\queryid\\1.1\\"
	}

	var listQuery = ""
	if se.params.NoCompressedList {
		listQuery = "\\list\\\\gamename\\" + se.params.QueryGamename
	} else {
		listQuery = "\\list\\cmp\\gamename\\" + se.params.QueryGamename
	}

	if se.params.AttachListFinal {
		listQuery += "\\final\\"
	}

	_, err = se.connection.Write([]byte(authQuery + listQuery))
	if err != nil {
		log.Println("Failed to write GOA SB Auth Query:", err.Error())
		se.ctxCancel(err)
		return
	}

	if se.params.NoCompressedList {
		se.ReadUncompressedList()
	} else {
		se.ReadCompressedResponse()
	}

	se.ctxCancel(nil)

}

func (se *ServerListEngine) ReadUncompressedList() {
	//read response
	serverListResponse := make([]byte, 25)

	// 255.255.255.255:65535 - max theoretical IP - 21 bytes
	// \ip\255.255.255.255:65535 - 25 bytes

	var ipStringAccum = ""

	for {
		slLen, slErr := se.connection.Read(serverListResponse)

		if slErr != nil && !errors.Is(slErr, io.EOF) {
			log.Println("Failed to read GOA SB Server List Response:", slErr.Error())
			se.ctxCancel(slErr)
			break
		}

		if slLen == 0 {
			break
		}

		var inputStr = string(serverListResponse[:slLen])

		ipStringAccum += inputStr

		splitData := strings.Split(ipStringAccum, "\\ip\\")
		for i, s := range splitData {

			if i+1 != len(splitData) {
				if len(s) > 0 {
					se.handleIPString(s)
				}

			} else {
				ipStringAccum = "\\ip\\" + s
			}

		}
	}

	if len(ipStringAccum) >= 4 {
		ipStringAccum = ipStringAccum[4:]

		var slashIdx = strings.Index(ipStringAccum, "\\")
		if slashIdx != -1 {
			ipStringAccum = ipStringAccum[:slashIdx]
		}
		if len(ipStringAccum) > 0 {
			se.handleIPString(ipStringAccum)
		}
	}

}

func (se *ServerListEngine) handleIPString(inputStr string) {

	addr, err := netip.ParseAddrPort(inputStr)
	if err != nil {
		log.Println("GOA Failed to parse IP String:", err.Error(), "Input: ", inputStr)
		return
	}
	if se.monitor.BeginQuery(se, se.queryEngine, addr) {
		se.queryEngine.Query(addr)
	}
}

func (se *ServerListEngine) ReadCompressedResponse() {
	//read response
	serverListResponse := make([]byte, 6)

	for {
		slLen, slErr := se.connection.Read(serverListResponse)

		if slErr != nil && !errors.Is(slErr, io.EOF) {
			log.Println("Failed to read GOA SB Server List Response:", slErr.Error())
			se.monitor.EndServerListEngine(se)
			break
		}

		if slLen == 0 {
			break
		}

		if string(serverListResponse) == "\\final" {
			break
		}

		serverIP, _ := netip.AddrFromSlice(serverListResponse[0:4])
		serverPort := binary.BigEndian.Uint16(serverListResponse[4:])

		var addr = netip.AddrPortFrom(serverIP, serverPort)
		if se.monitor.BeginQuery(se, se.queryEngine, addr) {
			se.queryEngine.Query(addr)
		}
	}
}

func (se *ServerListEngine) gsmsalg(validation string) string {
	dest := C.malloc(C.sizeof_char * 32)
	defer C.free(unsafe.Pointer(dest))

	src := C.CString(validation)
	defer C.free(unsafe.Pointer(src))

	key := C.CString(se.params.Secretkey)
	defer C.free(unsafe.Pointer(key))

	C.gsseckey((*C.char)(dest), (*C.char)(src), (*C.char)(key), C.int(0))

	var result = C.GoString((*C.char)(dest))

	return result
}

func (se *ServerListEngine) Shutdown() {
	if se.connection != nil {
		se.connection.Close()
	}
}
