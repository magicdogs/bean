package main

import (
	"bean/data"
	"bean/handler"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strconv"
)

func main() {
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
		client := &data.ConnWrapper{
			Conn:     conn,
			Listener: make(map[string]*data.ListenerWrapper),
			ReadCh:   make(chan data.Message, 100),
			SendCh:   make(chan data.Message, 100),
		}
		rawMessage, err := data.ReadMessageWait(client.Conn)
		if err != nil || int8(rawMessage.Type) != 1 {
			fmt.Printf("client err or msg type wrong \r\n")
			client.Close()
			continue
		}
		var srReq data.ServiceRequest
		err = json.Unmarshal(rawMessage.Body, &srReq)
		if nil != err {
			fmt.Printf("client json format error \r\n")
			client.Close()
			continue
		}
		client.ServiceReq = &srReq
		client.Id = srReq.Id
		go ProcessSvrRequest(client)
		go ReadMessage(client)
		go WriteMessage(client)
		go OpenSvr(client)
	}
}

func WriteMessage(connWrapper *data.ConnWrapper) {
	defer func() {
		connWrapper.Close()
	}()
	for {
		m, ok := <-connWrapper.SendCh
		if !ok {
			return
		} else {
			fmt.Printf("write message: %+v \r\n", m)
			messageType := data.ParseMessageType(m)
			err := data.WriteMessageByType(connWrapper.Conn, int8(messageType), m)
			if err != nil {
				return
			}
		}
	}
}
func ReadMessage(connWrapper *data.ConnWrapper) {
	defer func() {
		connWrapper.Close()
	}()
	for {
		m, err := data.ReadMessageWait(connWrapper.Conn)
		if nil != err {
			fmt.Printf("read chan err %v: \r\n", err)
			return
		}
		message, err := data.ParseMessage(m)
		if nil != err {
			fmt.Printf("message err %v: \r\n", err)
			return
		}
		if err != nil {
			if err == io.EOF {
				return
			} else {
				connWrapper.Close()
				return
			}
		} else {
			fmt.Printf("read message: %+v \r\n", m)
			connWrapper.ReadCh <- message
		}
	}
}

func OpenSvr(client *data.ConnWrapper) {
	defer func() {
		client.Close()
	}()
	for {
		message, ok := <-client.ReadCh
		if !ok {
			fmt.Printf("client id = %s \r\n", client.Id)
			return
		}
		switch v := message.(type) {
		case *data.ConnectResponse:
			go ReadClientMessage(client, v)
		case *data.CloseRequest:
			clientConns, okConn := client.Listener[v.Name]
			if okConn {
				workConn, ok := clientConns.ClientMap[v.Id]
				if ok {
					client.Mutex.Lock()
					delete(clientConns.ClientMap, v.Id)
					client.Mutex.Unlock()
					workConn.Conn.Close()
				}
			}

		case *data.BinDataRequestWrapper:
			clientConns := client.Listener[v.Name].ClientMap
			workConn, ok := clientConns[v.Id]
			if !ok {
				dtReq := &data.CloseRequest{
					Id:   v.Id,
					Name: v.Name,
				}
				client.SendCh <- dtReq
				continue
			}
			_, err := workConn.Conn.Write(v.Content)
			if nil != err {
				fmt.Printf("err %v: \r\n", err)
				workConn.Conn.Close()
			}
		case *data.HearBeatRequest:
			fmt.Printf("hear beat {%s} \r\n", client.Id)
			hrResp := &data.HearBeatResponse{
				Cid: client.Id,
			}
			client.SendCh <- hrResp
		default:
			fmt.Println(v)
		}
	}

}

func ProcessSvrRequest(client *data.ConnWrapper) {
	serviceRequest := client.ServiceReq
	resp := &data.ServiceResponse{
		Success: true,
		Id:      serviceRequest.Id,
		Message: "服务启动成功.",
	}
	for _, item := range serviceRequest.ServiceList {
		listen, err := net.Listen("tcp", "0.0.0.0:"+strconv.Itoa(item.RemotePort))
		if nil != err {
			fmt.Printf("err %v: \r\n", err)
			resp.Message = "服务启动失败，端口被占用."
			resp.Success = false
			break
		}
		client.Listener[item.Name] = &data.ListenerWrapper{
			Listener:  listen,
			ClientMap: make(map[string]*data.ClientConn),
		}
		fmt.Println("server start, listen to port " + strconv.Itoa(item.RemotePort) + " wait connect..")
	}
	client.SendCh <- resp
	for n, l := range client.Listener {
		go func(n string, l net.Listener) {
			for {
				conn, err := l.Accept()
				if nil != err {
					fmt.Printf("l.Listener.Accept err %v: \r\n", err)
					return
				}
				id := handler.RandStringRunes(12)
				client.Listener[n].ClientMap[id] = &data.ClientConn{
					Id:     id,
					Conn:   conn,
					ReadCh: make(chan []byte, 100),
				}
				connReq := &data.ConnectRequest{
					Id:   id,
					Name: n,
					Ip:   conn.RemoteAddr().String(),
				}
				client.SendCh <- connReq
			}
		}(n, l.Listener)
	}
}

func ReadClientMessage(client *data.ConnWrapper, request *data.ConnectResponse) {
	workConn := client.Listener[request.Name].ClientMap[request.Id].Conn
	defer func() {
		workConn.Close()
		if err := recover(); err != nil {
			fmt.Println("panic error server ReadClientMessage")
			client.Close()
			return
		}
	}()
	for {
		buf := make([]byte, 2*1024)
		n, err := workConn.Read(buf)
		if nil != err {
			fmt.Printf("err %v: \r\n", err)
			dtReq := &data.CloseRequest{
				Id:   request.Id,
				Name: request.Name,
			}
			client.SendCh <- dtReq
			client.Mutex.Lock()
			delete(client.Listener[request.Name].ClientMap, request.Id)
			client.Mutex.Unlock()
			return
		}
		buf = buf[0:n]
		dtReq := &data.BinDataRequestWrapper{
			BinDataRequest: data.BinDataRequest{
				Id:   request.Id,
				Name: request.Name,
			},
			Content: buf,
		}
		client.SendCh <- dtReq
	}
}
