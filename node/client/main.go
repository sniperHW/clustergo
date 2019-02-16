package main

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	codecs "github.com/sniperHW/sanguo/codec/cs"
	"github.com/sniperHW/sanguo/cs"
	cs_msg "github.com/sniperHW/sanguo/protocol/cs/message"
	"os"
	"reflect"
	"time"
)

var userID string

type handler func(kendynet.StreamSession, *codecs.Message)

type dispatcher struct {
	handlers map[string]handler
}

func (this *dispatcher) Register(msg proto.Message, h handler) {
	msgName := reflect.TypeOf(msg).String()
	if nil == h {
		return
	}
	_, ok := this.handlers[msgName]
	if ok {
		return
	}

	this.handlers[msgName] = h
}

func (this *dispatcher) Dispatch(session kendynet.StreamSession, msg *codecs.Message) {
	if nil != msg {
		name := msg.GetName()
		handler, ok := this.handlers[name]
		if ok {
			handler(session, msg)
		}
	}
}

func (this *dispatcher) OnClose(session kendynet.StreamSession, reason string) {
	fmt.Printf("connection lose:%s\n", reason)
}

func (this *dispatcher) OnEstablish(session kendynet.StreamSession) {
	fmt.Printf("OnEstablish\n")

	login := &cs_msg.GameLoginToS{
		UserID:   proto.String(userID),
		Token:    proto.String("12345"),
		ServerID: proto.Int32(1),
	}

	err := session.Send(codecs.NewMessage(0, login))
	if nil != err {
		fmt.Printf("send error:%s\n", err.Error())
	}
}

func onLoginResp(session kendynet.StreamSession, msg *codecs.Message) {
	loginResp := msg.GetData().(*cs_msg.GameLoginToC)
	fmt.Println(*loginResp)

	if loginResp.GetCode() == cs_msg.EnumType_OK {

		go func() {
			for {
				err := session.Send(codecs.NewMessage(0, &cs_msg.EchoToS{
					Msg: proto.String("hello"),
				}))
				if nil != err {
					fmt.Printf("send error:%s\n", err.Error())
					break
				}
				time.Sleep(time.Second)
			}
		}()

	}

}

func onEchoResp(session kendynet.StreamSession, msg *codecs.Message) {
	echoResp := msg.GetData().(*cs_msg.EchoToC)
	fmt.Println("onEchoResp", echoResp.GetMsg())
}

func (this *dispatcher) OnConnectFailed(peerAddr string, err error) {
	fmt.Printf("OnConnectFailed\n")
}

func main() {

	if len(os.Args) < 2 {
		fmt.Printf("usage ./testClient userID\n")
		return
	}

	userID = os.Args[1]

	d := &dispatcher{handlers: make(map[string]handler)}
	d.Register(&cs_msg.GameLoginToC{}, onLoginResp)
	d.Register(&cs_msg.EchoToC{}, onEchoResp)

	cs.DialTcp("localhost:9012", 0, d)
	sigStop := make(chan bool)
	_, _ = <-sigStop

}
