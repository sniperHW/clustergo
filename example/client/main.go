package main

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	codecs "github.com/sniperHW/sanguo/codec/cs"
	"github.com/sniperHW/sanguo/cs"
	"github.com/sniperHW/sanguo/protocol/cmdEnum"
	cs_message "github.com/sniperHW/sanguo/protocol/cs/message"
	"time"
)

type dispatcher struct {
}

func (this *dispatcher) Dispatch(session kendynet.StreamSession, msg *codecs.Message) {
	cmd := msg.GetCmd()
	if cmd == cmdEnum.CS_Echo {
		fmt.Println(msg.GetData().(*cs_message.EchoToC).GetMsg())
		time.Sleep(time.Second)
		err := session.Send(codecs.NewMessage(cmdEnum.CS_Echo, &cs_message.EchoToS{
			Msg: proto.String("hello"),
		}))

		if nil != err {
			fmt.Println(err)
		}
	}
}

func (this *dispatcher) OnClose(kendynet.StreamSession, string) {

}

func (this *dispatcher) OnEstablish(session kendynet.StreamSession) {
	fmt.Println("OnEstablish")

	err := session.Send(codecs.NewMessage(cmdEnum.CS_Echo, &cs_message.EchoToS{
		Msg: proto.String("hello"),
	}))

	if nil != err {
		fmt.Println(err)
	}
}

func (this *dispatcher) OnConnectFailed(peerAddr string, err error) {
	fmt.Println("OnConnectFailed", err)
}

func main() {
	fmt.Println("start client")
	cs.DialTcp("localhost:8110", time.Second, &dispatcher{})
	sigStop := make(chan bool)
	_, _ = <-sigStop
}
