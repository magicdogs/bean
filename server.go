package main

import (
	"bean/server"
)

//
//type ServerWriter struct {
//	C *BeanServer
//	R *common.ConnectResponse
//}
//
//func (b *ServerWriter) Write(p []byte) (n int, err error) {
//	dtReq := &common.BinDataRequestWrapper{
//		BinDataRequest: common.BinDataRequest{
//			Id:   b.R.Id,
//			Name: b.R.Name,
//		},
//		Content: p,
//	}
//	b.C.SendCh <- dtReq
//	return len(p), nil
//}

func main() {
	server.Run()
}
