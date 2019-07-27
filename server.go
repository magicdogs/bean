package main

import (
	"bean/data"
	"bean/handler"
	"fmt"
	"net"
	"strconv"
)

func main() {
	listen, err := net.Listen("tcp", "0.0.0.0:8092")
	if err != nil{
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
		client := &data.ConnWaper{
			Conn:     conn,
			Listener: make(map[string]*data.ListenerWaper),
			Cid:      handler.RandStringRunes(12),
		}
		go OpenSvr(client)
	}
}

func OpenSvr(client *data.ConnWaper) {
	defer client.Close()
	for {
		rawMessage, err := data.ReadMessageWait(client.Conn)
		if err != nil {
			fmt.Printf("client id = %s error = %+v \r\n",client.Cid,err)
			return
		}
		message, err := data.ParseMessage(rawMessage)
		if nil != err {
			fmt.Printf("err %v: \r\n", err)
			return
		}
		switch v := message.(type) {
		case *data.PortRequest:
			go ProcessPortRequest(v, client)
		case *data.ConnectResponse:
			conn := client.Listener[v.Name].ClientMap[v.Id].Conn
			go ReadClientMessage(conn, client, v)
		case *data.BinDataRequestWrapper:
			cacheConn := client.Listener[v.Name].ClientMap[v.Id].Conn
			_, err := cacheConn.Write(v.Content)
			if nil != err {
				fmt.Printf("err %v: \r\n", err)
				cacheConn.Close()
			}
		case *data.HearBeatRequest:
			fmt.Printf("hear beat {%s} \r\n", client.Cid)
			hrResp := &data.HearBeatResponse{
				Cid: client.Cid,
			}
			err := data.WriteMessage(client.Conn, 8, hrResp)
			if nil != err {
				fmt.Printf("hear beat error  err = % v \r\n", err)
				return
			}
		default:
		}
	}

}

func ProcessPortRequest(request *data.PortRequest, client *data.ConnWaper) {
	listen, err := net.Listen("tcp", "0.0.0.0:"+strconv.Itoa(request.RemotePort))
	if nil != err {
		fmt.Printf("err %v: \r\n", err)
		client.Close()
		return
	}
	fmt.Println("server start, listen to port " + strconv.Itoa(request.RemotePort) + " wait connect..")
	//defer listen.Close()
	resp := data.PortResponse{
		Success: true,
		Name:    request.Name,
		Message: "服务启动成功.",
	}
	err = data.WriteMessage(client.Conn, 2, resp)
	if nil != err {
		fmt.Printf("err %v: \r\n", err)
		client.Close()
		return
	}
	client.Listener[request.Name] = &data.ListenerWaper{
		Name:      request.Name,
		Cid:       client.Cid,
		Listener:  listen,
		ClientMap: make(map[string]*data.ClientConn),
	}
	for {
		conn, err := listen.Accept()
		if nil != err {
			fmt.Printf("err %v: \r\n", err)
			client.Close()
			return
		}
		id := handler.RandStringRunes(12)
		//fmt.Println("new connection income for port " + strconv.Itoa(request.RemotePort) + ",id = " + id)
		client.Listener[request.Name].ClientMap[id] = &data.ClientConn{
			Conn: conn,
			Id:   id,
		}
		connReq := data.ConnectRequest{
			Ip:   conn.RemoteAddr().String(),
			Id:   id,
			Name: request.Name,
		}
		err = data.WriteMessage(client.Conn, 3, connReq)
		if nil != err {
			fmt.Printf("err %v: \r\n", err)
			client.Close()
			return
		}
	}
}

func ReadClientMessage(conn net.Conn, client *data.ConnWaper, request *data.ConnectResponse) {
	defer conn.Close()
	for {
		buf := make([]byte, 2048)
		n, err := conn.Read(buf)
		if nil != err {
			fmt.Printf("err %v: \r\n", err)
			return
		}
		buf = buf[0:n]
		dtReq := &data.BinDataRequest{
			Id:   request.Id,
			Name: request.Name,
		}
		_, err = data.WriteDataMessage(client.Conn, 5, dtReq, buf)
		if nil != err {
			fmt.Printf("err %v: \r\n", err)
			client.Conn.Close()
			return
		}
	}

}
