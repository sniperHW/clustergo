package main

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/golog"
	"github.com/sniperHW/sanguo/cluster"
	"github.com/sniperHW/sanguo/cluster/addr"
	codecs "github.com/sniperHW/sanguo/codec/cs"
	"github.com/sniperHW/sanguo/cs"
	"github.com/sniperHW/sanguo/protocol/cmdEnum"
	cs_message "github.com/sniperHW/sanguo/protocol/cs/message"
	ss_rpc "github.com/sniperHW/sanguo/protocol/ss/rpc"
	"github.com/sniperHW/sanguo/rpc/gateToGame"
	"os"
	"sync/atomic"
)

type dispatcher struct {
	idCounter int64
	logger    golog.LoggerI
}

func (this *dispatcher) Dispatch(session kendynet.StreamSession, msg *codecs.Message) {
	userid := session.GetUserData().(int64)

	cmd := msg.GetCmd()

	this.logger.Infoln("Dispatch", cmd)

	if cmd == cmdEnum.CS_Echo {
		//gameserver的类型是2
		peer, err := cluster.Random(2)
		if nil != err {
			session.Send(codecs.NewMessage(cmd, &cs_message.EchoToC{
				Msg: proto.String("send to game failed"),
			}))
		} else {
			req := &ss_rpc.GateToGameReq{
				Userid:  proto.Int64(userid),
				Message: proto.String(msg.GetData().(*cs_message.EchoToS).GetMsg()),
			}

			resp, err := gateToGame.SyncCall(peer, req, 5000)

			if nil != err {
				session.Send(codecs.NewMessage(cmd, &cs_message.EchoToC{
					Msg: proto.String("send to game failed"),
				}))
			} else {

				session.Send(codecs.NewMessage(cmd, &cs_message.EchoToC{
					Msg: proto.String(resp.GetMessage()),
				}))
			}
		}
	}

}

func (this *dispatcher) OnClose(session kendynet.StreamSession, reason string) {
	this.logger.Infoln("OnClose", reason)
}

func (this *dispatcher) OnNewClient(session kendynet.StreamSession) {
	this.logger.Infoln("OnNewClient")
	userid := atomic.AddInt64(&this.idCounter, 1)
	session.SetUserData(userid)
}

func main() {

	logger := golog.New("log", golog.NewOutputLogger("log", "gateserver", 1024*1024*50))
	kendynet.InitLogger(logger)
	cluster.InitLogger(logger)

	center_addr := os.Args[1]

	addr, err := addr.MakeAddr("1.1.1", "localhost:9000")

	if nil != err {
		fmt.Println(err)
		return
	}

	err = cluster.Start([]string{center_addr}, addr)

	if nil == err {
		cs.StartTcpServer("tcp", "localhost:8110", &dispatcher{logger: logger})
		sigStop := make(chan bool)
		_, _ = <-sigStop
	} else {
		fmt.Printf("cluster Start error:%s\n", err.Error())
	}
}
