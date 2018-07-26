package main

import(
	codess "sanguo/codec/ss"
	_ "sanguo/protocol/ss" //触发pb注册
	ss_proto "sanguo/protocol/ss/message"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/socket/stream_socket/tcp"
	"github.com/golang/protobuf/proto"
	"fmt"
	"time"
	"github.com/sniperHW/kendynet/golog"
)


func server() {

	nextShow := time.Now().Unix()
	c := 0

	l,err := tcp.NewListener("tcp","127.0.0.1:9110")
	if nil == err {
		fmt.Printf("server running\n")
		l.Start(func(session kendynet.StreamSession) {
			fmt.Printf("new client\n")
			session.SetReceiver(codess.NewReceiver("ss","rpc_req","rpc_resp"))
			session.SetEncoder(codess.NewEncoder("ss","rpc_req","rpc_resp"))
			session.SetCloseCallBack(func (sess kendynet.StreamSession, reason string) {
			})
			session.Start(func (event *kendynet.Event) {
				if event.EventType == kendynet.EventTypeError {
					event.Session.Close(event.Data.(error).Error(),0)
				} else {
					msg := event.Data.(*codess.Message)
    				session.Send(msg.GetData())
    				c++
					now := time.Now().Unix()
					if now >= nextShow {
						fmt.Printf("c:%d\n",c)
						c = 0
						nextShow = now + 1
					}
				}
			})
		})		
	}
}

func client() {
	client,err := tcp.NewConnector("tcp4","127.0.0.1:9110")

	if err != nil {
		fmt.Printf("NewTcpClient failed:%s\n",err.Error())
		return
	}

	session,err := client.Dial(time.Second * 10)
	if err != nil {
		fmt.Printf("Dial error:%s\n",err.Error())
	} else {

		fmt.Printf("dial ok\n")

		session.SetReceiver(codess.NewReceiver("ss","rpc_req","rpc_resp"))
		session.SetEncoder(codess.NewEncoder("ss","rpc_req","rpc_resp"))
		session.SetCloseCallBack(func (sess kendynet.StreamSession, reason string) {
			fmt.Printf("client close:%s\n",reason)
		})
		session.Start(func (event *kendynet.Event) {
			if event.EventType == kendynet.EventTypeError {
				event.Session.Close(event.Data.(error).Error(),0)
			} else {
				msg := event.Data.(*codess.Message)
				event.Session.Send(msg.GetData())
			}
		})

		for i := 0; i < 10;i++ {
			Echo := &ss_proto.Echo{}
			Echo.Message = proto.String("hello")
			err := session.Send(Echo)
			if nil != err {
				fmt.Printf("%s\n",err.Error())
			}
		}
	}	
}


func main() {

	outLogger := golog.NewOutputLogger("log","benmark",1024*1024*50)
	kendynet.InitLogger(outLogger,"benmark")

	go server()
	time.Sleep(time.Second)
	go client()
	sigStop := make(chan bool)
	_,_ = <- sigStop
}
