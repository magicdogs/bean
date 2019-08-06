package main

import "C"
import (
	"bean/client"
)

func main() {
	beanClient := client.NewClientApplication()
	beanClient.Run()
}
