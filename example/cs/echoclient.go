package main

import(
	"sanguo/cs"
	"github.com/sniperHW/kendynet"
	cs_msg "sanguo/protocol/cs/message"
	codecs "sanguo/codec/cs"
	"fmt"
	"reflect"
	"github.com/golang/protobuf/proto"
	//"time"
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
	fmt.Printf("connection lose:%s\n",reason)
}

func (this *dispatcher) OnEstablish(session kendynet.StreamSession) {
	fmt.Printf("OnEstablish\n")	
	/*go func() {
		i := 0
		for {
			i = i + 1
			EchoToS := &cs_msg.EchoToS{}
			EchoToS.Msg = proto.String("hello")
			err := session.Send(codecs.NewMessage(uint16(i%512),EchoToS))
			if nil != err {
				fmt.Printf("send error:%s\n",err.Error())
			}
			time.Sleep(time.Second*1)
		}		
	}()*/
	for i:=0; i < 10;i++ {
		EchoToS := &cs_msg.EchoToS{}
		EchoToS.Msg = proto.String("hello")
		err := session.Send(codecs.NewMessage(uint16(i%512),EchoToS))
		if nil != err {
			fmt.Printf("send error:%s\n",err.Error())
		}		
	}
}

func (this *dispatcher) OnConnectFailed(peerAddr string,err error) {
	fmt.Printf("OnConnectFailed\n")	
}

func main() {
	d := &dispatcher{handlers:make(map[string]handler)}
	d.Register(&cs_msg.EchoToC{},func(session kendynet.StreamSession,msg *codecs.Message){
		//EchoToC := msg.GetData().(*cs_msg.EchoToC)
		//fmt.Printf("on EchoToC %d:%s\n",msg.GetSeriNo(),EchoToC.GetMsg())
		EchoToS := &cs_msg.EchoToS{}
		EchoToS.Msg = proto.String("hello")
		err := session.Send(codecs.NewMessage(uint16(0),EchoToS))
		if nil != err {
			fmt.Printf("send error:%s\n",err.Error())
		}
	})

	cs.DialTcp("127.0.0.1:8910",0,d)
	sigStop := make(chan bool)
	_,_ = <- sigStop

}