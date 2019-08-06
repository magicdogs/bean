package server

import (
	"bean/common"
	"encoding/json"
	"fmt"
	"net"
)

func Run() {
	listen, err := net.Listen("tcp", "0.0.0.0:8092")
	if err != nil {
		fmt.Printf("err %v: \r\n", err)
		return
	}
	fmt.Println("server start, listen to port 8092 wait connect..")
	defer listen.Close()
	for {
		conn, err := listen.Accept()
		if nil != err {
			fmt.Printf("err %v: \r\n", err)
			return
		}
		server := &BeanServer{
			Conn:     conn,
			Listener: make(map[string]*ListenerWrapper),
			ReadCh:   make(chan common.Message, 100),
			SendCh:   make(chan common.Message, 100),
		}
		rawMessage, err := common.ReadMessageWait(server.Conn)
		if err != nil || int8(rawMessage.Type) != 1 {
			fmt.Printf("client err or msg type wrong \r\n")
			server.Close()
			continue
		}
		var srReq common.ServiceRequest
		err = json.Unmarshal(rawMessage.Body, &srReq)
		if nil != err {
			fmt.Printf("client json format error \r\n")
			server.Close()
			continue
		}
		server.ServiceReq = &srReq
		server.Id = srReq.Id

		go server.ProcessSvrRequest()
		go common.MessageReader(server)
		go common.MessageWriter(server)
		go server.OpenSvr()
	}
}
