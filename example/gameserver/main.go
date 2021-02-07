package main

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/golog"
	"github.com/sniperHW/sanguo/cluster"
	"github.com/sniperHW/sanguo/cluster/addr"
	ss_rpc "github.com/sniperHW/sanguo/protocol/ss/rpc"
	"github.com/sniperHW/sanguo/rpc/gateToGame"
	"os"
)

type service struct {
	logger golog.LoggerI
}

func (this *service) OnCall(replyer *gateToGame.GateToGameReplyer, req *ss_rpc.GateToGameReq) {
	this.logger.Infoln("call from ", req.GetUserid(), "message", req.GetMessage())

	replyer.Reply(&ss_rpc.GateToGameResp{
		Userid:  proto.Int64(req.GetUserid()),
		Message: proto.String("message from game"),
	})

}

func main() {

	logger := golog.New("log", golog.NewOutputLogger("log", "gameserver", 1024*1024*50))
	kendynet.InitLogger(logger)
	cluster.InitLogger(logger)

	center_addr := os.Args[1]

	gateToGame.Register(&service{
		logger: logger,
	})

	//类型2
	addr, err := addr.MakeAddr("1.2.1", "localhost:9001")

	if nil != err {
		fmt.Println(err)
		return
	}

	err = cluster.Start([]string{center_addr}, addr, nil)

	if nil == err {
		sigStop := make(chan bool)
		_, _ = <-sigStop
	} else {
		fmt.Printf("cluster Start error:%s\n", err.Error())
	}
}
