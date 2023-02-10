package GOA

// #cgo CFLAGS: -g -Wall
// #include <stdlib.h>
// #include "gsmsalg.h"
import "C"

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os-serverlist-sync/Engine"
	"unsafe"
)

type ServerListEngineParams struct {
	ServerAddress string `json:"address"`
	Gamename      string `json:"gamename"`
	Secretkey     string `json:"secretkey"`
	QueryGamename string `json:"query_gamename"`
}

type ServerListEngine struct {
	connection  *net.TCPConn
	queryEngine Engine.IQueryEngine
	params      *ServerListEngineParams
}

func (se *ServerListEngine) SetQueryEngine(engine Engine.IQueryEngine) {
	se.queryEngine = engine
}

func (se *ServerListEngine) SetParams(params interface{}) {
	se.params = params.(*ServerListEngineParams)
}

func (se *ServerListEngine) Invoke() {

	fmt.Println("Invoke " + se.params.ServerAddress)

	servAddr := se.params.ServerAddress
	tcpAddr, err := net.ResolveTCPAddr("tcp", servAddr)
	if err != nil {
		println("ResolveTCPAddr failed:", err.Error())
		return
	}

	conn, dialErr := net.DialTCP("tcp", nil, tcpAddr)
	if dialErr != nil {
		println("Dial failed:", dialErr.Error())
		return
	}
	se.connection = conn

	//wait for TCP reply, etc
	se.think()
}

func (se *ServerListEngine) think() {

	defer se.connection.Close()

	reply := make([]byte, 32)

	_, err := se.connection.Read(reply)
	if err != nil {
		println("Failed to read GOA SB Auth Request:", err.Error())
		return
	}

	var challenge = string(reply)[15:]

	//write authentication
	var validation_response = se.gsmsalg(challenge)

	var authQuery = "\\gamename\\" + se.params.Gamename + "\\validate\\" + validation_response + "\\final\\"

	var listQuery = "\\list\\cmp\\gamename\\" + se.params.QueryGamename
	_, err = se.connection.Write([]byte(authQuery + listQuery))
	if err != nil {
		println("Failed to write GOA SB Auth Query:", err.Error())
		return
	}

	//read response
	serverListResponse := make([]byte, 6)

	for {
		slLen, slErr := se.connection.Read(serverListResponse)

		if slErr != nil && !errors.Is(slErr, io.EOF) {
			println("Failed to read GOA SB Server List Response:", slErr.Error())
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

		se.queryEngine.Query(addr)
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
