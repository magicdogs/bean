package common

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

type JoinWriter struct {
	Id     string
	Name   string
	Sender chan Message
}

func (b *JoinWriter) Write(p []byte) (n int, err error) {
	dtReq := &BinDataRequestWrapper{
		BinDataRequest: BinDataRequest{
			Id:   b.Id,
			Name: b.Name,
		},
		Content: p,
	}
	b.Sender <- dtReq
	return len(p), nil
}

type BeanReaderWriter interface {
	Close()
	WorkConn() net.Conn
	ReaderCh() chan Message
	SenderCh() chan Message
}

func MessageWriter(rwx BeanReaderWriter) {
	defer func() {
		rwx.Close()
	}()
	for {
		m, ok := <-rwx.SenderCh()
		if !ok {
			return
		} else {
			fmt.Printf("write message: %+v \r\n", m)
			messageType := ParseMessageType(m)
			err := WriteMessageByType(rwx.WorkConn(), int8(messageType), m)
			if err != nil {
				return
			}
		}
	}
}

func MessageReader(rwx BeanReaderWriter) {
	defer func() {
		rwx.Close()
	}()
	for {
		m, err := ReadMessageWait(rwx.WorkConn())
		if nil != err {
			rwx.Close()
			if err == io.EOF {
				fmt.Printf("read chan eof  %v: \r\n", err)
				return
			}
			fmt.Printf("read chan err %v: \r\n", err)
			return
		}
		message, err := ParseMessage(m)
		if err != nil {
			fmt.Printf("message err %v: \r\n", err)
			continue
		} else {
			rwx.ReaderCh() <- message
		}
	}
}

type Message interface {
}

type ServiceRequest struct {
	Id          string        `json:"id"`
	ServiceList []ServiceBody `json:"service_list"`
	ReqTime     time.Time     `json:"req_time"`
}

type ServiceBody struct {
	Name       string `json:"name"`
	RemotePort int    `json:"remote_port"`
}

type ServiceResponse struct {
	Id      string `json:"id"`
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type ConnectRequest struct {
	Id   string `json:"id"`
	Name string `json:"name"`
	Ip   string `json:"ip"`
}

type ConnectResponse struct {
	Id      string `json:"id"`
	Name    string `json:"name"`
	Success bool   `json:"success"`
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
		return nil, err
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
	_, err = io.ReadFull(conn, bufBody)
	if nil != err {
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

func WriteMessageByType(conn net.Conn, typ int8, msg Message) (err error) {
	if typ == 5 {
		v := msg.(*BinDataRequestWrapper)
		return WriteDataMessage(conn, typ, v.BinDataRequest, v.Content)
	} else {
		return WriteMessage(conn, typ, msg)
	}
}
func WriteDataMessage(conn net.Conn, typ int8, data Message, buf []byte) (err error) {
	bytePack, err := json.Marshal(data)
	if err != nil {
		return err
	}
	jsonLen := int32(len(bytePack))
	binLen := int32(len(buf))
	total := int32(jsonLen+binLen) + 8
	buffer := bytes.NewBuffer(nil)
	writer := bufio.NewWriter(buffer)
	writer.WriteByte(byte(typ))
	err = binary.Write(writer, binary.LittleEndian, total)
	if err != nil {
		return err
	}
	err = binary.Write(writer, binary.LittleEndian, jsonLen)
	if err != nil {
		return err
	}
	err = binary.Write(writer, binary.LittleEndian, binLen)
	if err != nil {
		return err
	}
	writer.Write(bytePack)
	writer.Write(buf)
	writer.Flush()
	n, err := conn.Write(buffer.Bytes())
	if n != len(buffer.Bytes()) {
		fmt.Println("vvvvv error")
	}
	return err
}

func WriteMessage(conn net.Conn, typ int8, data Message) (err error) {
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

func ParseMessage(rawMessage *RawMessage) (Message, error) {
	v := int(rawMessage.Type)
	switch v {
	case 1:
		var prReq ServiceRequest
		err := json.Unmarshal(rawMessage.Body, &prReq)
		return &prReq, err
	case 2:
		var prResp ServiceResponse
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

func ParseMessageType(message interface{}) int {
	switch v := message.(type) {
	case *ServiceRequest:
		return 1
	case *ServiceResponse:
		return 2
	case *ConnectRequest:
		return 3
	case *ConnectResponse:
		return 4
	case *BinDataRequest:
		return 5
	case *BinDataRequestWrapper:
		return 5
	case *CloseRequest:
		return 6
	case *HearBeatRequest:
		return 7
	case *HearBeatResponse:
		return 8
	default:
		fmt.Println(v)
		return -1
	}
}
