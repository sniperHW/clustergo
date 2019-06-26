package main

import (
	"fmt"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/sanguo/cluster"
	"github.com/sniperHW/sanguo/cluster/addr"
	"github.com/sniperHW/sanguo/node/node_gate"
	"github.com/sniperHW/sanguo/util"
	"os"
)

func main() {

	fmt.Println("hello")

	if len(os.Args) < 2 {
		fmt.Printf("usage ./node_game config\n")
		return
	}

	logger := util.NewLogger("log", "node_gate", 1024*1024*50)
	kendynet.InitLogger(logger)
	cluster.InitLogger(logger)

	node_gate.InitLogger(logger)

	selfAddr, _ := addr.MakeAddr("1.4.1", "localhost:8013")

	centerAddrs := []string{"localhost:8010"}

	node_gate.Init()

	err := cluster.Start(centerAddrs, selfAddr)

	if nil != err {
		node_gate.Errorf("node_gate start failed1:%s\n", err.Error())
		return
	}

	err = node_gate.Start("localhost:9012")
	if nil != err {
		node_gate.Errorf("node_gate start failed2:%s\n", err.Error())
	} else {
		node_gate.Infoln("node_gate start on %s\n", "localhost:9012")
		fmt.Printf("node_gate start on %s\n", "localhost:9012")
		sigStop := make(chan bool)
		_, _ = <-sigStop
	}
}
