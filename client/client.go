package client

import (
	"bean/common"
	"net"
)

type BeanClientConfig struct {
	ServerAddr  string                  `json:"server_addr"`
	ServiceList []BeanClientServiceItem `json:"service_list"`
}

type BeanClientServiceItem struct {
	Name       string `json:"name"`
	RemotePort int    `json:"remote_port"`
	LocalAddr  string `json:"local_addr"`
}

func NewClientApplication() *BeanClient {
	client := &BeanClient{
		ProxyMap:      make(map[string]net.Conn),
		ServiceConfig: make(map[string]BeanClientServiceItem),
		CloseSign:     make(chan bool),
		RestartSign:   make(chan bool),
		ReadCh:        make(chan common.Message, 10),
		SendCh:        make(chan common.Message, 10),
		Closed:        false,
	}
	client.InitConfig()
	return client
}
