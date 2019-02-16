package main

import (
	"fmt"
	"sanguo/cluster/addr"
)

func main() {

	{
		addr, err := addr.MakeAddr("4095.2.4095", "127.0.0.1:8010")
		if nil != err {
			fmt.Println(err)
		} else {
			fmt.Println(addr.Logic.String(), addr.Logic.Group(), addr.Logic.Type(), addr.Logic.Server(), addr.Net)
		}
	}

	{
		addr, err := addr.MakeHarborAddr("4095.255.4095", "127.0.0.1:8010")
		if nil != err {
			fmt.Println(err)
		} else {
			fmt.Println(addr.Logic.String(), addr.Logic.Group(), addr.Logic.Type(), addr.Logic.Server(), addr.Net)
		}
	}

}
