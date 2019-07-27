package data

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
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

type ConnWaper struct {
	Conn     net.Conn
	Listener map[string]*ListenerWaper
	Cid      string
}

type ListenerWaper struct {
	Name      string
	Cid       string
	ClientMap map[string]*ClientConn
	Listener  net.Listener
}

type ClientConn struct {
	Conn net.Conn
	Id   string
}

func (p *ConnWaper) Close() {
	p.Conn.Close()
	for _, v := range p.Listener {
		for _, m := range v.ClientMap {
			m.Conn.Close()
		}
		v.Listener.Close()
	}
}

type PortRequest struct {
	Name       string    `json:"name"`
	RemotePort int       `json:"remote_port"`
	ReqTime    time.Time `json:"req_time"`
}

type PortResponse struct {
	Name    string `json:"name"`
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type ConnectRequest struct {
	Ip   string `json:"ip"`
	Id   string `json:"id"`
	Name string `json:"name"`
}

type ConnectResponse struct {
	Success bool   `json:"success"`
	Id      string `json:"id"`
	Name    string `json:"name"`
}

type BinDataRequest struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

type BinDataRequestWrapper struct {
	BinDataRequest
	Content []byte
}

type CloseRequest struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

type HearBeatRequest struct {
	SendTime time.Time `json:"send_time"`
}

type HearBeatResponse struct {
	Cid string
}

type RawMessage struct {
	Type   byte
	Body   []byte
	Length int32
}

func ReadMessageWait(conn net.Conn) (*RawMessage, error) {
	buffer := make([]byte, 1)
	_, err := conn.Read(buffer)
	if err != nil {
		fmt.Println("rr1 io.eof")
		return nil,err
	}
	typ := uint8(buffer[0])
	var length int32
	err = binary.Read(conn, binary.LittleEndian, &length)
	if nil != err {
		fmt.Println("rr2 io.eof")
		return nil, err
	}
	if length < 0 || length > 100*1024*1024 {
		fmt.Println("rr3 length error")
		return nil, errors.New("package size limit")
	}
	fmt.Printf("read message type = %d, message length = %d \r\n", typ, length)
	bufBody := make([]byte, length)
	_, err = io.ReadFull(conn,bufBody)
	if nil != err{
		fmt.Println("ffxxxxxxxxx error" + err.Error())
		return nil, err
	}
	raw := &RawMessage{
		Type:   typ,
		Body:   bufBody,
		Length: length,
	}
	return raw, nil
}

func WriteDataMessage(conn net.Conn, typ int8, data *BinDataRequest, buf []byte) (n int, err error) {
	bytePack, err := json.Marshal(data)
	if err != nil {
		return -2, err
	}
	jsonLen := int32(len(bytePack))
	binLen := int32(len(buf))
	total := int32(jsonLen+binLen) + 8
	buffer := bytes.NewBuffer(nil)
	writer := bufio.NewWriter(buffer)
	writer.WriteByte(byte(typ))
	err = binary.Write(writer, binary.LittleEndian, total)
	if err != nil {
		return -1, err
	}
	err = binary.Write(writer, binary.LittleEndian, jsonLen)
	if err != nil {
		return -3, err
	}
	err = binary.Write(writer, binary.LittleEndian, binLen)
	if err != nil {
		return -4, err
	}
	writer.Write(bytePack)
	writer.Write(buf)
	writer.Flush()
	n, err = conn.Write(buffer.Bytes())
	if n != len(buffer.Bytes()) {
		fmt.Println("vvvvv error")
	}
	return n, err
}

func WriteMessage(conn net.Conn, typ int8, data interface{}) (err error) {
	bytePack, err := json.Marshal(data)
	if err != nil {
		return err
	}
	length := int32(len(bytePack))
	buffer := bytes.NewBuffer(nil)
	writer := bufio.NewWriter(buffer)
	writer.WriteByte(byte(typ))
	err = binary.Write(writer, binary.LittleEndian, length)
	if err != nil {
		return err
	}
	writer.Write(bytePack)
	writer.Flush()
	fmt.Printf("write message type = %d, message length = %d \r\n", typ, length)
	bytLen := len(buffer.Bytes())
	n, err := conn.Write(buffer.Bytes())
	if n != bytLen {
		log.Printf("write len error")
	}
	return err
}

func ParseMessage(rawMessage *RawMessage) (interface{}, error) {
	v := int(rawMessage.Type)
	switch v {
	case 1:
		var prReq PortRequest
		err := json.Unmarshal(rawMessage.Body, &prReq)
		return &prReq, err
	case 2:
		var prResp PortResponse
		err := json.Unmarshal(rawMessage.Body, &prResp)
		return &prResp, err
	case 3:
		var crReq ConnectRequest
		err := json.Unmarshal(rawMessage.Body, &crReq)
		return &crReq, err
	case 4:
		var crResp ConnectResponse
		err := json.Unmarshal(rawMessage.Body, &crResp)
		return &crResp, err
	case 5:
		buffer := bytes.NewBuffer(rawMessage.Body)
		var jsonLen, binLen int32
		err := binary.Read(buffer, binary.LittleEndian, &jsonLen)
		if nil != err {
			fmt.Println(err)
			return nil, err
		}
		err = binary.Read(buffer, binary.LittleEndian, &binLen)
		if nil != err {
			fmt.Println(err)
			return nil, err
		}
		jsonBuf := make([]byte, jsonLen)
		_, err = buffer.Read(jsonBuf)
		if nil != err {
			fmt.Println(err)
			return nil, err
		}
		var dtResp BinDataRequest
		err = json.Unmarshal(jsonBuf, &dtResp)
		if nil != err {
			fmt.Println(err)
			return nil, err
		}
		binBuf := make([]byte, binLen)
		_, err = buffer.Read(binBuf)
		if nil != err {
			fmt.Println(err)
			return nil, err
		}
		binWrapper := BinDataRequestWrapper{
			BinDataRequest: dtResp,
			Content:        binBuf,
		}
		err = json.Unmarshal(jsonBuf, &dtResp)
		if nil != err {
			fmt.Println(err)
			return nil, err
		}
		return &binWrapper, err
	case 6:
		var closeReq CloseRequest
		err := json.Unmarshal(rawMessage.Body, &closeReq)
		return &closeReq, err
	case 7:
		var hearBeatReq HearBeatRequest
		err := json.Unmarshal(rawMessage.Body, &hearBeatReq)
		return &hearBeatReq, err
	case 8:
		var hearBeatResp HearBeatResponse
		err := json.Unmarshal(rawMessage.Body, &hearBeatResp)
		return &hearBeatResp, err
	default:
		return nil, errors.New("notype")
	}
}
