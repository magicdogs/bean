package main

import (
	"bean/data"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"time"
)

var connMap map[string]net.Conn
var configMap map[string]data.ClientServiceConfig
var clientConfig data.ClientConfig
var closeSign,restartSign chan bool

func init(){
	connMap = make(map[string]net.Conn)
	configMap = make(map[string]data.ClientServiceConfig)
	closeSign = make(chan bool)
	restartSign = make(chan bool)
	content, err := ioutil.ReadFile("./config/client.json")
	if nil != err {
		fmt.Println("config.json read error ")
		return
	}
	err = json.Unmarshal(content, &clientConfig)
	if nil != err {
		fmt.Println("json config Unmarshal error ")
		return
	}
	fmt.Println(clientConfig)
}

func main() {
	go RunClient(false)
	for {
		select {
			case <- closeSign:
				fmt.Println("system exit sign")
				return
			case <- restartSign:
				fmt.Println("client restart sign")
				go RunClient(true)
		}
	}
}

func RunClient(restartFlag bool) {
	conn, err := net.Dial("tcp", clientConfig.ServerAddr)
	if nil != err {
		fmt.Printf("err %v: \r\n", err)
		if restartFlag {
			restartSign <- true
		} else {
			closeSign <- true
		}
		return
	}
	fmt.Println("链接到服务器成功....")
	for _, item := range clientConfig.ServiceList {
		pr := &data.PortRequest{
			Name:       item.Name,
			RemotePort: item.RemotePort,
			ReqTime:    time.Now(),
		}
		err = data.WriteMessage(conn, 1, pr)
		if err != nil{
			fmt.Printf("err %v: \r\n", err)
			return
		}
		raw, err := data.ReadMessageWait(conn)
		if err != nil{
			fmt.Printf("err %v: \r\n", err)
			return
		}
		message, err := data.ParseMessage(raw)
		if err != nil{
			fmt.Printf("err %v: \r\n", err)
			return
		}
		switch v := message.(type) {
		case *data.PortResponse:
			fmt.Printf("service open success %v: \r\n", v)
			configMap[item.Name] = item
		default:
			fmt.Printf("service open fail %v: \r\n", v)
			return
		}
	}
	go ClientHearBeat(conn)
	go TransportMessage(conn)
}

func TransportMessage(conn net.Conn) {
	defer func() {
		conn.Close()
	}()
	for {
		raw, err := data.ReadMessageWait(conn)
		if err != nil{
			fmt.Printf("conn read eof %v: \r\n", err)
			return
		}
		message, err := data.ParseMessage(raw)
		if err != nil && err.Error() == "notype"{
			continue
		}
		switch v := message.(type) {
		case *data.ConnectRequest:
			createPortSvr(v, conn)
		case *data.BinDataRequestWrapper:
			ReadSvrMessage(v)
		case *data.HearBeatResponse:
			fmt.Println("heart beat resp id = " + v.Cid)
		default:
		}
	}
}

func ClientHearBeat(conn net.Conn) {
	ticker := time.NewTicker(10 * time.Second)
	defer func() {
		ticker.Stop()
		restartSign <- true
	}()
	for {
		t := <-ticker.C
		htReq := data.HearBeatRequest{
			SendTime: t,
		}
		err := data.WriteMessage(conn, 7, &htReq)
		if nil != err {
			fmt.Printf("x3error hearbeat err = % v \r\n", err)
			conn.Close()
			return
		}
	}
}

func createPortSvr(request *data.ConnectRequest, conn net.Conn) {
	serviceConfig := configMap[request.Name]
	connLocal, err := net.Dial("tcp", serviceConfig.LocalAddr)
	if err != nil{
		fmt.Printf("err %v: \r\n", err)
		return
	}
	crResp := &data.ConnectResponse{
		Success: true,
		Id:      request.Id,
		Name:    request.Name,
	}
	err = data.WriteMessage(conn, 4, crResp)
	if err != nil{
		fmt.Printf("err %v: \r\n", err)
		conn.Close()
		return
	}
	fmt.Println(crResp)
	connMap[request.Id] = connLocal
	go ReadLocalSvrMessage(conn, connLocal, request)
}

func ReadLocalSvrMessage(conn net.Conn, connLocal net.Conn, request *data.ConnectRequest) {
	defer connLocal.Close()
	for {
		buf := make([]byte, 2048)
		n, err := connLocal.Read(buf)
		if err != nil{
			fmt.Printf("err %v: \r\n", err)
			connLocal.Close()
			return
		}
		buf = buf[0:n]
		dtReq := &data.BinDataRequest{
			Id:   request.Id,
			Name: request.Name,
		}
		_, err = data.WriteDataMessage(conn, 5, dtReq, buf)
		if err != nil{
			fmt.Printf("err %v: \r\n", err)
			conn.Close()
			return
		}
	}
}

func ReadSvrMessage(dtReq *data.BinDataRequestWrapper) {
	connLocal := connMap[dtReq.Id]
	if connLocal == nil {
		fmt.Printf("connLocal == nil err \r\n")
		return
	}
	_, err := connLocal.Write(dtReq.Content)
	if err != nil{
		connLocal.Close()
		connMap[dtReq.Id] = nil
		fmt.Printf("err %v: \r\n", err)
		return
	}
}

