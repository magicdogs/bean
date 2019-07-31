package main

import "C"
import (
	"bean/data"
	"bean/handler"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"sync"
	"time"
)

type ClientConfig struct {
	ServerAddr  string                `json:"server_addr"`
	ServiceList []ClientServiceConfig `json:"service_list"`
}

type ClientServiceConfig struct {
	Name       string `json:"name"`
	RemotePort int    `json:"remote_port"`
	LocalAddr  string `json:"local_addr"`
}

type ClientWriter struct {
	C *ClientApplication
	R *data.ConnectRequest
}

func (b *ClientWriter) Write(p []byte) (n int, err error) {
	dtReq := &data.BinDataRequestWrapper{
		BinDataRequest: data.BinDataRequest{
			Id:   b.R.Id,
			Name: b.R.Name,
		},
		Content: p,
	}
	b.C.SendCh <- dtReq
	return len(p), nil
}

type ClientApplication struct {
	Id            string
	Conn          net.Conn
	Config        *ClientConfig
	CloseSign     chan bool
	RestartSign   chan bool
	ServiceConfig map[string]ClientServiceConfig
	ProxyMap      map[string]net.Conn
	ReadCh        chan data.Message
	SendCh        chan data.Message
	Closed        bool
	Mutex         sync.Mutex
}

func (c *ClientApplication) Clear() {
	c.ReadCh = make(chan data.Message, 100)
	c.SendCh = make(chan data.Message, 100)
	c.Closed = false
}
func (c *ClientApplication) Close() {
	c.Mutex.Lock()
	if !c.Closed {
		close(c.ReadCh)
		close(c.SendCh)
		c.Closed = true
	}
	c.Mutex.Unlock()
	if c.Conn != nil {
		c.Conn.Close()
	}
	for _, v := range c.ProxyMap {
		if v != nil {
			v.Close()
		}
	}
	//c.ProxyMap = nil
	//close(c.ReadCh)
	//close(c.SendCh)
}

func NewClientApplication() *ClientApplication {
	clientApplication := &ClientApplication{
		ProxyMap:      make(map[string]net.Conn),
		ServiceConfig: make(map[string]ClientServiceConfig),
		CloseSign:     make(chan bool),
		RestartSign:   make(chan bool),
		ReadCh:        make(chan data.Message, 10),
		SendCh:        make(chan data.Message, 10),
		Closed:        false,
	}
	return clientApplication
}

func InitConfig() *ClientConfig {
	content, err := ioutil.ReadFile("./config/client.json")
	if nil != err {
		fmt.Println("config.json read error ")
		panic(err)
	}
	var clientConfig ClientConfig
	err = json.Unmarshal(content, &clientConfig)
	if nil != err {
		fmt.Println("json config Unmarshal error ")
		panic(err)
	}
	return &clientConfig
}

func CWriteMessage(client *ClientApplication) {
	defer func() {
		client.Close()
	}()
	for {
		m, ok := <-client.SendCh
		if !ok {
			return
		} else {
			messageType := data.ParseMessageType(m)
			err := data.WriteMessageByType(client.Conn, int8(messageType), m)
			if err != nil {
				return
			}
		}
	}
}
func CReadMessage(client *ClientApplication) {
	defer func() {
		client.Close()
	}()
	for {
		m, err := data.ReadMessageWait(client.Conn)
		if nil != err {
			client.Close()
			if err == io.EOF {
				fmt.Printf("read chan eof  %v: \r\n", err)
				return
			}
			fmt.Printf("read chan err %v: \r\n", err)
			return
		}
		message, err := data.ParseMessage(m)
		if err != nil {
			fmt.Printf("message err %v: \r\n", err)
			continue
		} else {
			client.ReadCh <- message
		}
	}
}

func main() {
	clientApplication := NewClientApplication()
	clientApplication.Config = InitConfig()
	go RunClient(clientApplication, false)
	for {
		select {
		case <-clientApplication.CloseSign:
			fmt.Println("system exit sign")
			return
		case <-clientApplication.RestartSign:
			clientApplication.Clear()
			time.Sleep(15 * time.Second)
			fmt.Println("client restart sign")
			go RunClient(clientApplication, true)
		}
	}
}

func RunClient(clientApplication *ClientApplication, restartFlag bool) {
	conn, err := net.Dial("tcp", clientApplication.Config.ServerAddr)
	if nil != err {
		fmt.Printf("err %v: \r\n", err)
		if restartFlag {
			clientApplication.RestartSign <- true
		} else {
			clientApplication.CloseSign <- true
		}
		return
	}
	clientApplication.Conn = conn
	fmt.Println("链接到服务器成功....")
	srReq := &data.ServiceRequest{
		Id:          handler.RandStringRunes(12),
		ServiceList: make([]data.ServiceBody, 0),
		ReqTime:     time.Now(),
	}
	for _, item := range clientApplication.Config.ServiceList {
		svrBody := data.ServiceBody{
			Name:       item.Name,
			RemotePort: item.RemotePort,
		}
		clientApplication.ServiceConfig[item.Name] = item
		srReq.ServiceList = append(srReq.ServiceList, svrBody)
	}
	err = data.WriteMessage(clientApplication.Conn, int8(1), srReq)
	if nil != err {
		fmt.Printf("wr error %+v \r\n", err)
		clientApplication.RestartSign <- true
		return
	}
	rawMessage, err := data.ReadMessageWait(clientApplication.Conn)
	if nil != err {
		fmt.Printf("rd error %+v \r\n", err)
		clientApplication.RestartSign <- true
		return
	}
	if rawMessage.Type != 2 {
		fmt.Printf("raw error %+v \r\n", err)
		clientApplication.RestartSign <- true
		return
	}
	var srResp data.ServiceResponse
	err = json.Unmarshal(rawMessage.Body, &srResp)
	if nil != err {
		fmt.Printf("json.Unmarshal error %+v \r\n", err)
		clientApplication.RestartSign <- true
		return
	}
	fmt.Printf("service open success %v: \r\n", srResp)
	clientApplication.Id = srResp.Id
	go CWriteMessage(clientApplication)
	go CReadMessage(clientApplication)
	go TransportMessage(clientApplication)
}

func TransportMessage(clientApplication *ClientApplication) {
	ticker := time.NewTicker(10 * time.Second)
	defer func() {
		if err := recover(); err != nil {
			fmt.Println("panic error TransportMessage")
			ticker.Stop()
			clientApplication.RestartSign <- true
			return
		}
	}()
	for {
		select {
		case t := <-ticker.C:
			htReq := data.HearBeatRequest{
				SendTime: t,
			}
			clientApplication.SendCh <- &htReq
		case message := <-clientApplication.ReadCh:
			switch v := message.(type) {
			case *data.ConnectRequest:
				createPortSvr(v, clientApplication)
			case *data.BinDataRequestWrapper:
				ReadSvrMessage(v, clientApplication)
			case *data.HearBeatResponse:
				fmt.Println("heart beat resp id = " + v.Cid)
			case *data.CloseRequest:
				conn, ok := clientApplication.ProxyMap[v.Id]
				if ok {
					conn.Close()
					delete(clientApplication.ProxyMap, v.Id)
				}
			default:
			}
		}
	}
}

func createPortSvr(request *data.ConnectRequest, clientApplication *ClientApplication) {
	serviceConfig := clientApplication.ServiceConfig[request.Name]
	connLocal, err := net.Dial("tcp", serviceConfig.LocalAddr)
	if err != nil {
		fmt.Printf("err %v: \r\n", err)
		closeReq := &data.CloseRequest{
			Id:   request.Id,
			Name: request.Name,
		}
		clientApplication.SendCh <- closeReq
		return
	}
	crResp := &data.ConnectResponse{
		Success: true,
		Id:      request.Id,
		Name:    request.Name,
	}
	clientApplication.SendCh <- crResp
	clientApplication.ProxyMap[request.Id] = connLocal
	go ReadLocalSvrMessage(clientApplication, connLocal, request)
}

func ReadLocalSvrMessage(clientApplication *ClientApplication, connLocal net.Conn, request *data.ConnectRequest) {
	defer connLocal.Close()
	for {
		//cwr := &ClientWriter{
		//	C:clientApplication,
		//	R:request,
		//}
		//written, err := io.Copy(cwr, connLocal)
		//fmt.Println("cwr = " + strconv.Itoa(int(written)))
		bytBuf := make([]byte, 32*1024)
		n, err := connLocal.Read(bytBuf)
		dtReq := &data.BinDataRequestWrapper{
			BinDataRequest: data.BinDataRequest{
				Id:   request.Id,
				Name: request.Name,
			},
			Content: bytBuf[:n],
		}
		if err != nil {
			fmt.Printf("err %v: \r\n", err)
			dtReq := &data.CloseRequest{
				Id:   request.Id,
				Name: request.Name,
			}
			clientApplication.SendCh <- dtReq
			delete(clientApplication.ProxyMap, request.Id)
			connLocal.Close()
			clientApplication.SendCh <- dtReq
			return
		}
		clientApplication.SendCh <- dtReq
	}
}

func ReadSvrMessage(dtReq *data.BinDataRequestWrapper, clientApplication *ClientApplication) {
	connLocal, ok := clientApplication.ProxyMap[dtReq.Id]
	if !ok {
		fmt.Printf("connLocal == nil err \r\n")
		closeReq := &data.CloseRequest{
			Id:   dtReq.Id,
			Name: dtReq.Name,
		}
		clientApplication.SendCh <- closeReq
		return
	}
	_, err := connLocal.Write(dtReq.Content)
	if err != nil {
		fmt.Printf("connLocal == nil err \r\n")
		closeReq := &data.CloseRequest{
			Id:   dtReq.Id,
			Name: dtReq.Name,
		}
		clientApplication.SendCh <- closeReq
		connLocal.Close()
		delete(clientApplication.ProxyMap, dtReq.Id)
		fmt.Printf("err %v: \r\n", err)
		return
	}
}
