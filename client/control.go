package client

import (
	"bean/common"
	"bean/handler"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"sync"
	"time"
)

type BeanClient struct {
	Id            string
	Conn          net.Conn
	Config        *BeanClientConfig
	CloseSign     chan bool
	RestartSign   chan bool
	ServiceConfig map[string]BeanClientServiceItem
	ProxyMap      map[string]net.Conn
	ReadCh        chan common.Message
	SendCh        chan common.Message
	Closed        bool
	Mutex         sync.Mutex
}

func (c *BeanClient) Clear() {
	c.ReadCh = make(chan common.Message, 100)
	c.SendCh = make(chan common.Message, 100)
	c.Closed = false
}

func (c *BeanClient) Close() {
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
}

func (c *BeanClient) WorkConn() net.Conn {
	return c.Conn
}

func (c *BeanClient) ReaderCh() chan common.Message {
	return c.ReadCh
}

func (c *BeanClient) SenderCh() chan common.Message {
	return c.SendCh
}

func (c *BeanClient) Run() {
	go c.RunClient(false)
	for {
		select {
		case <-c.CloseSign:
			fmt.Println("system exit sign")
			return
		case <-c.RestartSign:
			c.Clear()
			time.Sleep(15 * time.Second)
			fmt.Println("client restart sign")
			go c.RunClient(true)
		}
	}
}

func (c *BeanClient) InitConfig() {
	content, err := ioutil.ReadFile("./config/client.json")
	if nil != err {
		fmt.Println("config.json read error ")
		panic(err)
	}
	var clientConfig BeanClientConfig
	err = json.Unmarshal(content, &clientConfig)
	if nil != err {
		fmt.Println("json config Unmarshal error ")
		panic(err)
	}
	c.Config = &clientConfig
}

func (c *BeanClient) RunClient(restartFlag bool) {
	conn, err := net.Dial("tcp", c.Config.ServerAddr)
	if nil != err {
		fmt.Printf("err %v: \r\n", err)
		if restartFlag {
			c.RestartSign <- true
		} else {
			c.CloseSign <- true
		}
		return
	}
	c.Conn = conn
	fmt.Println("链接到服务器成功....")
	c.LoginToServer()
	fmt.Println("登陆服务器成功....")
	go common.MessageWriter(c)
	go common.MessageReader(c)
	go c.TransportMessage()
}

func (c *BeanClient) TransportMessage() {
	ticker := time.NewTicker(10 * time.Second)
	defer func() {
		if err := recover(); err != nil {
			fmt.Println("panic error TransportMessage")
			ticker.Stop()
			c.RestartSign <- true
			return
		}
	}()
	for {
		select {
		case t := <-ticker.C:
			htReq := common.HearBeatRequest{
				SendTime: t,
			}
			c.SendCh <- &htReq
		case message := <-c.ReadCh:
			switch v := message.(type) {
			case *common.ConnectRequest:
				createPortSvr(v, c)
			case *common.BinDataRequestWrapper:
				ReadSvrMessage(v, c)
			case *common.HearBeatResponse:
				fmt.Println("heart beat resp id = " + v.Cid)
			case *common.CloseRequest:
				conn, ok := c.ProxyMap[v.Id]
				if ok {
					conn.Close()
					delete(c.ProxyMap, v.Id)
				}
			default:
			}
		}
	}
}
func (c *BeanClient) LoginToServer() {
	srReq := &common.ServiceRequest{
		Id:          handler.RandStringRunes(12),
		ServiceList: make([]common.ServiceBody, 0),
		ReqTime:     time.Now(),
	}
	for _, item := range c.Config.ServiceList {
		svrBody := common.ServiceBody{
			Name:       item.Name,
			RemotePort: item.RemotePort,
		}
		c.ServiceConfig[item.Name] = item
		srReq.ServiceList = append(srReq.ServiceList, svrBody)
	}
	err := common.WriteMessage(c.Conn, int8(1), srReq)
	if nil != err {
		fmt.Printf("wr error %+v \r\n", err)
		c.RestartSign <- true
		return
	}
	rawMessage, err := common.ReadMessageWait(c.Conn)
	if nil != err {
		fmt.Printf("rd error %+v \r\n", err)
		c.RestartSign <- true
		return
	}
	if rawMessage.Type != 2 {
		fmt.Printf("raw error %+v \r\n", err)
		c.RestartSign <- true
		return
	}
	var srResp common.ServiceResponse
	err = json.Unmarshal(rawMessage.Body, &srResp)
	if nil != err {
		fmt.Printf("json.Unmarshal error %+v \r\n", err)
		c.RestartSign <- true
		return
	}
	fmt.Printf("service open success %v: \r\n", srResp)
	c.Id = srResp.Id

}

func createPortSvr(request *common.ConnectRequest, clientApplication *BeanClient) {
	serviceConfig := clientApplication.ServiceConfig[request.Name]
	connLocal, err := net.Dial("tcp", serviceConfig.LocalAddr)
	if err != nil {
		fmt.Printf("err %v: \r\n", err)
		closeReq := &common.CloseRequest{
			Id:   request.Id,
			Name: request.Name,
		}
		clientApplication.SendCh <- closeReq
		return
	}
	crResp := &common.ConnectResponse{
		Success: true,
		Id:      request.Id,
		Name:    request.Name,
	}
	clientApplication.SendCh <- crResp
	clientApplication.ProxyMap[request.Id] = connLocal
	go ReadLocalSvrMessage(clientApplication, connLocal, request)
}

func ReadLocalSvrMessage(clientApplication *BeanClient, connLocal net.Conn, request *common.ConnectRequest) {
	defer func() {
		dtReq := &common.CloseRequest{
			Id:   request.Id,
			Name: request.Name,
		}
		clientApplication.SendCh <- dtReq
		clientApplication.Mutex.Lock()
		delete(clientApplication.ProxyMap, request.Id)
		clientApplication.Mutex.Unlock()
		connLocal.Close()
	}()
	//cwr := &common.JoinWriter{
	//	Sender: clientApplication.SendCh,
	//	Id: request.Id,
	//	Name: request.Name,
	//}
	//buf := make([]byte,512)
	//written, err := io.CopyBuffer(cwr,connLocal,buf)
	//fmt.Println("cwr = " + strconv.Itoa(int(written)))
	//if err != nil {
	//	fmt.Printf("err %v: \r\n", err)
	//	return
	//}
	for {
		bytBuf := make([]byte, 512)
		n, err := connLocal.Read(bytBuf)
		dtReq := &common.BinDataRequestWrapper{
			BinDataRequest: common.BinDataRequest{
				Id:   request.Id,
				Name: request.Name,
			},
			Content: bytBuf[:n],
		}
		if err != nil {
			fmt.Printf("err %v: \r\n", err)
			return
		}
		clientApplication.SendCh <- dtReq
	}
}

func ReadSvrMessage(dtReq *common.BinDataRequestWrapper, clientApplication *BeanClient) {
	connLocal, ok := clientApplication.ProxyMap[dtReq.Id]
	defer func() {
		closeReq := &common.CloseRequest{
			Id:   dtReq.Id,
			Name: dtReq.Name,
		}
		clientApplication.SendCh <- closeReq
		connLocal.Close()
		clientApplication.Mutex.Lock()
		delete(clientApplication.ProxyMap, dtReq.Id)
		clientApplication.Mutex.Unlock()

	}()
	if !ok {
		fmt.Printf("connLocal == nil err \r\n")
		closeReq := &common.CloseRequest{
			Id:   dtReq.Id,
			Name: dtReq.Name,
		}
		clientApplication.SendCh <- closeReq
		return
	}
	_, err := connLocal.Write(dtReq.Content)
	if err != nil {
		fmt.Printf("connLocal == nil, err %v: \r\n", err)
		return
	}
}
