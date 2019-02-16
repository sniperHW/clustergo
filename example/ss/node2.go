package main

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/golog"
	"github.com/sniperHW/kendynet/rpc"
	"github.com/sniperHW/sanguo/cluster"
	"github.com/sniperHW/sanguo/cluster/addr"
	_ "github.com/sniperHW/sanguo/protocol/ss" //触发pb注册
	ss_rpc "github.com/sniperHW/sanguo/protocol/ss/rpc"
	ss_msg "github.com/sniperHW/sanguo/protocol/ss/ssmessage"
	"os"
)

func main() {

	if len(os.Args) != 2 {
		fmt.Println("useage node2 centeraddr")
		return
	}

	logger := golog.New("log", golog.NewOutputLogger("log", "node2", 1024*1024*50))
	kendynet.InitLogger(logger)
	cluster.InitLogger(logger)

	center_addr := os.Args[1]

	cluster.Register(&ss_msg.Echo{}, func(from addr.LogicAddr, msg proto.Message) {
		fmt.Printf("on echo req\n")
		cluster.PostMessage(from, msg)
	})

	cluster.RegisterMethod(&ss_rpc.EchoReq{}, func(replyer *rpc.RPCReplyer, arg interface{}) {
		echo := arg.(*ss_rpc.EchoReq)
		kendynet.Infof("echo message:%s\n", echo.GetMessage())
		echoResp := &ss_rpc.EchoResp{}
		echoResp.Message = proto.String("world")
		replyer.Reply(echoResp, nil)
	})

	addr, err := addr.MakeAddr("1.1.1", "localhost:8011")

	if nil != err {
		fmt.Println(err)
		return
	}

	err = cluster.Start([]string{center_addr}, addr)

	if nil == err {
		sigStop := make(chan bool)
		_, _ = <-sigStop
	} else {
		fmt.Printf("cluster Start error:%s\n", err.Error())
	}
}
