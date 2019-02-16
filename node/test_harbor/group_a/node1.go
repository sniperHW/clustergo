package main

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/golog"
	"os"
	"sanguo/cluster"
	"sanguo/cluster/addr"
	_ "sanguo/protocol/ss" //触发pb注册
	ss_rpc "sanguo/protocol/ss/rpc"
	ss_msg "sanguo/protocol/ss/ssmessage"
	"time"
)

func main() {
	logger := golog.New("log", golog.NewOutputLogger("log", "node1", 1024*1024*50))
	kendynet.InitLogger(logger)
	cluster.InitLogger(logger)

	center_addr := os.Args[1]

	cluster.Register(&ss_msg.Echo{}, func(from addr.LogicAddr, msg proto.Message) {
		fmt.Printf("on echo resp\n")
	})

	selfAddr, err := addr.MakeAddr("1.1.1", "localhost:9000")

	if nil != err {
		fmt.Println(err)
	} else {

		err = cluster.Start([]string{center_addr}, selfAddr)

		peer, _ := addr.MakeLogicAddr("2.1.1")

		if nil == err {
			go func() {
				for i := 0; ; i++ {

					time.Sleep(time.Second)
					echo := &ss_msg.Echo{}
					echo.Message = proto.String("hello")
					cluster.PostMessage(peer, echo)

					echoReq := &ss_rpc.EchoReq{}
					echoReq.Message = proto.String(fmt.Sprintf("hello:%d", i))

					fmt.Println("send req", echoReq.GetMessage())

					//异步调用
					cluster.AsynCall(peer, echoReq, 10, func(result interface{}, err error) {
						if nil == err {
							resp := result.(*ss_rpc.EchoResp)
							kendynet.Infof("echoRpc response %s\n", resp.GetMessage())
						} else {
							kendynet.Infoln(echoReq.GetMessage(), err)
						}
					})
				}
			}()
			sigStop := make(chan bool)
			_, _ = <-sigStop
		} else {
			fmt.Printf("cluster Start error:%s\n", err.Error())
		}
	}
}
