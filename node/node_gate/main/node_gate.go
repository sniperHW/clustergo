package main

import (
	"fmt"
	"github.com/go-ini/ini"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/sanguo/cluster"
	"github.com/sniperHW/sanguo/cluster/addr"
	"github.com/sniperHW/sanguo/node/node_gate"
	"github.com/sniperHW/sanguo/util"
	"os"
	"strings"
)

func main() {

	if len(os.Args) < 2 {
		fmt.Printf("usage ./node_game config\n")
		return
	}

	logger := util.NewLogger("log", "node_gate", 1024*1024*50)
	kendynet.InitLogger(logger)
	cluster.InitLogger(logger)

	node_gate.InitLogger(logger)

	cfg, err := ini.LooseLoad(os.Args[1])
	if err != nil {
		node_gate.Errorf("load %s failed:%s\n", os.Args[1], err.Error())
		return
	}

	secCommon := cfg.Section("Common")
	if nil == secCommon {
		node_gate.Errorf("missing sec[Common]\n")
		return
	}

	sec := cfg.Section(os.Args[2])
	if nil == sec {
		node_gate.Errorf("missing sec[%s]\n", os.Args[2])
		return
	}

	centerAddr := secCommon.Key("centerAddr").Value()
	clusterAddr := sec.Key("clusterAddr").Value()
	externalAddr := sec.Key("externalAddr").Value()

	fmt.Println("clusterAddr")

	t := strings.Split(clusterAddr, "@")
	if len(t) != 2 {
		logger.Errorln("invaild clusterAddr")
		return
	}

	selfAddr, err := addr.MakeAddr(t[0], t[1])

	if err != nil {
		logger.Errorln("MakeAddr error:", err)
		return
	}

	centerAddrs := strings.Split(centerAddr, ",")

	node_gate.Init()

	err = cluster.Start(centerAddrs, selfAddr)

	if nil != err {
		node_gate.Errorf("node_gate start failed1:%s\n", err.Error())
		return
	}

	err = node_gate.Start(externalAddr)
	if nil != err {
		node_gate.Errorf("node_gate start failed2:%s\n", err.Error())
	} else {
		node_gate.Infoln("node_gate start on %s\n", externalAddr)
		fmt.Printf("node_gate start on %s\n", externalAddr)
		sigStop := make(chan bool)
		_, _ = <-sigStop
	}
}
