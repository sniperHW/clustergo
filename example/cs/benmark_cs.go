package main

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	//"github.com/sniperHW/kendynet/golog"
	connector "github.com/sniperHW/kendynet/socket/connector/tcp"
	listener "github.com/sniperHW/kendynet/socket/listener/tcp"
	codecs "sanguo/codec/cs"
	_ "sanguo/protocol/cs" //触发pb注册
	cs_proto "sanguo/protocol/cs/message"
	"time"
)

func server() {

	nextShow := time.Now().Unix()
	c := 0

	l, err := listener.New("tcp", "127.0.0.1:9110")
	if nil == err {
		fmt.Printf("server running\n")
		l.Serve(func(session kendynet.StreamSession) {
			fmt.Printf("new client\n")
			session.SetReceiver(codecs.NewReceiver("cs"))
			session.SetEncoder(codecs.NewEncoder("sc"))
			session.SetCloseCallBack(func(sess kendynet.StreamSession, reason string) {
			})
			session.Start(func(event *kendynet.Event) {
				if event.EventType == kendynet.EventTypeError {
					event.Session.Close(event.Data.(error).Error(), 0)
				} else {

					msg := event.Data.(*codecs.Message)

					EchoToC := &cs_proto.EchoToC{}
					EchoToC.Msg = proto.String(msg.GetData().(*cs_proto.EchoToS).GetMsg())
					session.Send(codecs.NewMessage(1, EchoToC))
					c++
					now := time.Now().Unix()
					if now >= nextShow {
						fmt.Printf("c:%d\n", c)
						c = 0
						nextShow = now + 1
					}
				}
			})
		})
	}
}

func client() {
	client, err := connector.New("tcp4", "127.0.0.1:9110")

	if err != nil {
		fmt.Printf("NewTcpClient failed:%s\n", err.Error())
		return
	}

	session, err := client.Dial(time.Second * 10)
	if err != nil {
		fmt.Printf("Dial error:%s\n", err.Error())
	} else {

		fmt.Printf("dial ok\n")

		session.SetReceiver(codecs.NewReceiver("sc"))
		session.SetEncoder(codecs.NewEncoder("cs"))
		session.SetCloseCallBack(func(sess kendynet.StreamSession, reason string) {
			fmt.Printf("client close:%s\n", reason)
		})
		session.Start(func(event *kendynet.Event) {
			if event.EventType == kendynet.EventTypeError {
				event.Session.Close(event.Data.(error).Error(), 0)
			} else {
				msg := event.Data.(*codecs.Message)
				//event.Session.Send(msg.GetData())
				EchoToS := &cs_proto.EchoToS{}
				EchoToS.Msg = proto.String(msg.GetData().(*cs_proto.EchoToC).GetMsg())
				session.Send(codecs.NewMessage(uint16(0), EchoToS)) //.SetCompress())
			}
		})

		for i := 0; i < 50; i++ {
			EchoToS := &cs_proto.EchoToS{}
			EchoToS.Msg = proto.String("hello")
			err := session.Send(codecs.NewMessage(uint16(0), EchoToS)) //.SetCompress())
			if nil != err {
				fmt.Printf("%s\n", err.Error())
			}
		}

		for i := 0; i < 10; i++ {
			EchoToS := &cs_proto.EchoToS{}
			EchoToS.Msg = proto.String("hello fasdfdasfasdfasfasfeasdfasdffasfasfsadf")
			err := session.Send(codecs.NewMessage(uint16(0), EchoToS)) //.SetCompress())
			if nil != err {
				fmt.Printf("%s\n", err.Error())
			}
		}

	}
}

func main() {

	//outLogger := golog.NewOutputLogger("log", "benmark", 1024*1024*50)
	//kendynet.InitLogger(outLogger, "benmark")

	go server()
	time.Sleep(time.Second)
	go client()
	sigStop := make(chan bool)
	_, _ = <-sigStop
}
