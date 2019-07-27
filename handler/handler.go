package handler

import (
	"log"
	"math/rand"
	"net"
	"os"
	"syscall"
)

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func RandStringRunes(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func IsCaredNetError(err error,msg string) bool {
	if nil == err {
		return false
	}
	log.Printf("msg = %s ,IsCaredNetError  error = %+v \r\n", msg,err)
	if err.Error() == "EOF" {
		return true
	}
	netErr, ok := err.(net.Error)
	if !ok {
		return false
	}
	if netErr.Timeout() {
		log.Println("timeout")
		return true
	}
	opErr, ok := netErr.(*net.OpError)
	if !ok {
		return false
	}
	switch t := opErr.Err.(type) {
		case *net.DNSError:
			log.Printf("net.DNSError:%+v", t)
			return true
		case *os.SyscallError:
			log.Printf("os.SyscallError:%+v", t)
			if errno, ok := t.Err.(syscall.Errno); ok {
				switch errno {
					case syscall.ECONNREFUSED:
						log.Println("connect refused")
						return true
					case syscall.ETIMEDOUT:
						log.Println("timeout")
						return true
					case syscall.WSAECONNRESET:
						log.Println("wsaeconn reset")
						return true
					case 10048:
						log.Println("bind port already in use")
						return true
					}
			}
		default:
			return true
	}
	return false
}
