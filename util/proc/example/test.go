package main

import (
	"fmt"
	"github.com/sniperHW/sanguo/util/proc"
)

func main() {

	procList, _ := proc.GetProcs("teacher")

	for _, v := range procList {
		fmt.Println(v.User, v.Pid, v.CommandName, v.FullCommand)
	}

	//fmt.Println(procList)

}
