package main

import "C"
import (
	"bean/client"
)

//type ClientWriter struct {
//	C *ClientApplication
//	R *data.ConnectRequest
//}
//
//func (b *ClientWriter) Write(p []byte) (n int, err error) {
//	dtReq := &data.BinDataRequestWrapper{
//		BinDataRequest: data.BinDataRequest{
//			Id:   b.R.Id,
//			Name: b.R.Name,
//		},
//		Content: p,
//	}
//	b.C.SendCh <- dtReq
//	return len(p), nil
//}

func main() {
	beanClient := client.NewClientApplication()
	beanClient.Run()
}
