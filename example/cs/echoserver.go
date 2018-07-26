package main

import(
	"sanguo/cs"
	"github.com/sniperHW/kendynet"
	cs_msg "sanguo/protocol/cs/message"
	codecs "sanguo/codec/cs"
	"fmt"
	"reflect"
	"github.com/golang/protobuf/proto"
	"time"
)

type handler func(kendynet.StreamSession,*codecs.Message)

type dispatcher struct {
	handlers map[string]handler
}

func (this *dispatcher) Register(msg proto.Message,h handler) {
	msgName := reflect.TypeOf(msg).String()	
	if nil == h {
		return
	}
	_,ok := this.handlers[msgName]
	if ok {
		return
	}

	this.handlers[msgName] = h
}

func (this *dispatcher) Dispatch(session kendynet.StreamSession,msg *codecs.Message) {
	if nil != msg {
		name := msg.GetName()
		handler,ok := this.handlers[name]
		if ok {
			handler(session,msg)
		}
	}
}

func (this *dispatcher) OnClose(session kendynet.StreamSession,reason string) {
	fmt.Printf("client close:%s\n",reason)
}


func (this *dispatcher) OnNewClient(session kendynet.StreamSession) {
	fmt.Printf("new client\n")	
}

func main() {
	d := &dispatcher{handlers:make(map[string]handler)}

	nextShow := time.Now().Unix()
	c := 0

	d.Register(&cs_msg.EchoToS{},func(session kendynet.StreamSession,msg *codecs.Message){
		//EchoToS := msg.GetData().(*cs_msg.EchoToS)
		//fmt.Printf("on EchoToS %d:%s\n",msg.GetSeriNo(),EchoToS.GetMsg())
		EchoToC := &cs_msg.EchoToC{}
		EchoToC.Msg = proto.String("world")
		session.Send(codecs.NewMessage(msg.GetSeriNo(),EchoToC))
	
		c++
		now := time.Now().Unix()
		if now >= nextShow {
			fmt.Printf("c:%d\n",c)
			c = 0
			nextShow = now + 1
		}

	})

	err := cs.StartTcpServer("tcp","127.0.0.1:8910",d)

	if nil != err {
		fmt.Printf("StartTcpServer failed:%s\n",err.Error())
	} else {
		fmt.Printf("server start on 127.0.0.1:8010\n")
		sigStop := make(chan bool)
		_,_ = <- sigStop
	}

}