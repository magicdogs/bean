package server

import (
	"bean/common"
	"bean/handler"
	"fmt"
	"net"
	"strconv"
	"sync"
)

type ListenerWrapper struct {
	ClientMap map[string]*ClientConn
	Listener  net.Listener
}

type ClientConn struct {
	Id     string
	Conn   net.Conn
	ReadCh chan []byte
}

type BeanServer struct {
	Id         string
	Conn       net.Conn
	Listener   map[string]*ListenerWrapper
	ReadCh     chan common.Message
	SendCh     chan common.Message
	ServiceReq *common.ServiceRequest
	Closed     bool
	Mutex      sync.Mutex
}

func (s *BeanServer) Close() {
	s.Mutex.Lock()
	if !s.Closed {
		close(s.ReadCh)
		close(s.SendCh)
		s.Closed = true
	}
	s.Mutex.Unlock()
	for _, v := range s.Listener {
		for _, m := range v.ClientMap {
			if m.Conn != nil {
				m.Conn.Close()
			}
		}
		if v.Listener != nil {
			v.Listener.Close()
		}
	}
	if s.Conn != nil {
		s.Conn.Close()
	}
}

func (s *BeanServer) WorkConn() net.Conn {
	return s.Conn
}

func (s *BeanServer) ReaderCh() chan common.Message {
	return s.ReadCh
}

func (s *BeanServer) SenderCh() chan common.Message {
	return s.SendCh
}

func (s *BeanServer) ProcessSvrRequest() {
	serviceRequest := s.ServiceReq
	resp := &common.ServiceResponse{
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
		s.Listener[item.Name] = &ListenerWrapper{
			Listener:  listen,
			ClientMap: make(map[string]*ClientConn),
		}
		fmt.Println("server start, listen to port " + strconv.Itoa(item.RemotePort) + " wait connect..")
	}
	s.SendCh <- resp
	for n, l := range s.Listener {
		go func(n string, l net.Listener) {
			for {
				conn, err := l.Accept()
				if nil != err {
					fmt.Printf("l.Listener.Accept err %v: \r\n", err)
					return
				}
				id := handler.RandStringRunes(12)
				s.Listener[n].ClientMap[id] = &ClientConn{
					Id:     id,
					Conn:   conn,
					ReadCh: make(chan []byte, 100),
				}
				connReq := &common.ConnectRequest{
					Id:   id,
					Name: n,
					Ip:   conn.RemoteAddr().String(),
				}
				s.SendCh <- connReq
			}
		}(n, l.Listener)
	}
}

func (s *BeanServer) OpenSvr() {
	defer func() {
		s.Close()
	}()
	for {
		message, ok := <-s.ReadCh
		if !ok {
			fmt.Printf("client id = %s \r\n", s.Id)
			return
		}
		switch v := message.(type) {
		case *common.ConnectResponse:
			go ReadClientMessage(s, v)
		case *common.CloseRequest:
			clientConns, okConn := s.Listener[v.Name]
			if okConn {
				workConn, ok := clientConns.ClientMap[v.Id]
				if ok {
					s.Mutex.Lock()
					delete(clientConns.ClientMap, v.Id)
					s.Mutex.Unlock()
					workConn.Conn.Close()
				}
			}

		case *common.BinDataRequestWrapper:
			clientConns := s.Listener[v.Name].ClientMap
			workConn, ok := clientConns[v.Id]
			if !ok {
				dtReq := &common.CloseRequest{
					Id:   v.Id,
					Name: v.Name,
				}
				s.SendCh <- dtReq
				continue
			}
			_, err := workConn.Conn.Write(v.Content)
			if nil != err {
				fmt.Printf("err %v: \r\n", err)
				workConn.Conn.Close()
			}
		case *common.HearBeatRequest:
			fmt.Printf("hear beat {%s} \r\n", s.Id)
			hrResp := &common.HearBeatResponse{
				Cid: s.Id,
			}
			s.SendCh <- hrResp
		default:
			fmt.Println(v)
		}
	}

}

func ReadClientMessage(client *BeanServer, request *common.ConnectResponse) {
	defer func() {
		dtReq := &common.CloseRequest{
			Id:   request.Id,
			Name: request.Name,
		}
		client.SendCh <- dtReq
		client.Mutex.Lock()
		delete(client.Listener[request.Name].ClientMap, request.Id)
		client.Mutex.Unlock()
		if err := recover(); err != nil {
			fmt.Println("panic error server ReadClientMessage")
			client.Close()
			return
		}
	}()
	channel, ok := client.Listener[request.Name].ClientMap[request.Id]
	if !ok {
		fmt.Println("ReadClientMessage error , workConn not exists in map.")
		return
	}
	workConn := channel.Conn
	defer workConn.Close()
	//swr := &common.JoinWriter{
	//	Sender: client.SendCh,
	//	Id: request.Id,
	//	Name: request.Name,
	//}
	//buf := make([]byte, 512)
	//n, err := io.CopyBuffer(swr, workConn, buf)
	//fmt.Println("swr n = " + strconv.Itoa(int(n)))
	//if nil != err {
	//	fmt.Printf("err %v: \r\n", err)
	//	return
	//}
	for {
		buf := make([]byte, 512)
		n, err := workConn.Read(buf)
		dtReq := &common.BinDataRequestWrapper{
			BinDataRequest: common.BinDataRequest{
				Id:   request.Id,
				Name: request.Name,
			},
			Content: buf[0:n],
		}
		client.SendCh <- dtReq
		fmt.Println("swr n = " + strconv.Itoa(int(n)))
		if nil != err {
			fmt.Printf("err %v: \r\n", err)
			return
		}
	}

}
