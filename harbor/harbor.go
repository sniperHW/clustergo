package main

import (
	"fmt"
	"github.com/sniperHW/sanguo/cluster"
	"github.com/sniperHW/sanguo/cluster/addr"
	_ "github.com/sniperHW/sanguo/protocol/ss" //触发pb注册
	"os"
	"strings"

	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/golog"
)

func main() {
	logger := golog.New("log", golog.NewOutputLogger("log", "harbor", 1024*1024*50))
	kendynet.InitLogger(logger)
	cluster.InitLogger(logger)

	if len(os.Args) < 4 {
		fmt.Println("useage harbor centers logicAddr netAddr")
		return
	}

	centerAddrs := os.Args[1]
	selfLogic := os.Args[2]
	selfNet := os.Args[3]

	selfAddr, err := addr.MakeHarborAddr(selfLogic, selfNet)

	logger.Infoln("self addr", selfLogic)

	if nil != err {
		fmt.Println(err)
	} else {
		err = cluster.Start(strings.Split(centerAddrs, "@"), selfAddr, nil)
		if nil != err {
			fmt.Println(err)
		} else {
			sigStop := make(chan bool)
			_, _ = <-sigStop
		}
	}
}
