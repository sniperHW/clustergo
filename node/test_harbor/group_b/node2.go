package main

import (
	"fmt"
	"github.com/sniperHW/sanguo/cluster"
	"github.com/sniperHW/sanguo/cluster/addr"
	_ "github.com/sniperHW/sanguo/protocol/ss" //触发pb注册
	ss_rpc "github.com/sniperHW/sanguo/protocol/ss/rpc"
	ss_msg "github.com/sniperHW/sanguo/protocol/ss/ssmessage"
	"os"

	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/golog"
	"github.com/sniperHW/kendynet/rpc"
)

func main() {

	logger := golog.New("log", golog.NewOutputLogger("log", "node2", 1024*1024*50))
	kendynet.InitLogger(logger)
	cluster.InitLogger(logger)

	center_addr := os.Args[1]

	cluster.Register(&ss_msg.Echo{}, func(from addr.LogicAddr, msg proto.Message) {
		fmt.Printf("on echo req from:%s\n", from.String())
		cluster.PostMessage(from, msg)
	})

	cluster.RegisterMethod(&ss_rpc.EchoReq{}, func(replyer *rpc.RPCReplyer, arg interface{}) {
		echo := arg.(*ss_rpc.EchoReq)
		kendynet.Infof("echo message:%s\n", echo.GetMessage())
		echoResp := &ss_rpc.EchoResp{}
		echoResp.Message = proto.String("world")
		replyer.Reply(echoResp, nil)
	})

	addr, err := addr.MakeAddr("2.2.1", "localhost:9001")

	if nil != err {
		fmt.Println(err)
		return
	}

	err = cluster.Start([]string{center_addr}, addr, 1)

	if nil == err {
		sigStop := make(chan bool)
		_, _ = <-sigStop
	} else {
		fmt.Printf("cluster Start error:%s\n", err.Error())
	}
}
