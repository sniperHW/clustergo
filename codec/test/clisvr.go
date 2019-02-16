package main

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/socket/stream_socket/tcp"
	"github.com/sniperHW/sanguo/codec/cs"
	"github.com/sniperHW/sanguo/codec/pb"
	"github.com/sniperHW/sanguo/codec/test/testproto"
	"os"
	"os/signal"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"
)

func server(service string) {

	clientcount := int32(0)
	packetcount := int32(0)

	go func() {
		for {
			time.Sleep(time.Second)
			tmp2 := atomic.LoadInt32(&packetcount)
			atomic.StoreInt32(&packetcount, 0)
			fmt.Printf("clientcount:%d,packetcount:%d\n", clientcount, tmp2)
		}
	}()

	toC := &testproto.ChatToC{}
	toC.Response = proto.String("Hello")

	server, err := tcp.NewListener("tcp4", service)
	if server != nil {
		fmt.Printf("server running on:%s\n", service)
		err = server.Start(func(session kendynet.StreamSession) {
			atomic.AddInt32(&clientcount, 1)
			session.SetReceiver(cs.NewReceiver("cs"))
			session.SetCloseCallBack(func(sess kendynet.StreamSession, reason string) {
				fmt.Printf("client close:%s\n", reason)
				atomic.AddInt32(&clientcount, -1)
			})
			session.Start(func(event *kendynet.Event) {
				if event.EventType == kendynet.EventTypeError {
					event.Session.Close(event.Data.(error).Error(), 0)
				} else {
					atomic.AddInt32(&packetcount, int32(1))
					msg := event.Data.(cs.Message)
					toS := msg.GetData().(*testproto.ChatToS)
					if toS.GetContent() != "Hello" {
						fmt.Println("error")
					}
					//fmt.Println(toS.GetContent())
					session.Send(cs.NewMessage(0, toC))
				}
			})
		})

		if nil != err {
			fmt.Printf("TcpServer start failed %s\n", err)
		}

	} else {
		fmt.Printf("NewTcpServer failed %s\n", err)
	}
}

func client(service string, count int) {

	client, err := tcp.NewConnector("tcp4", service)

	if err != nil {
		fmt.Printf("NewTcpClient failed:%s\n", err.Error())
		return
	}

	for i := 0; i < count; i++ {
		session, err := client.Dial(time.Second * 10)
		if err != nil {
			fmt.Printf("Dial error:%s\n", err.Error())
		} else {
			session.SetReceiver(cs.NewReceiver("sc"))
			session.SetEncoder(cs.NewEncoder("cs"))

			toS := &testproto.ChatToS{}
			toS.Content = proto.String("Hello")

			session.Start(func(event *kendynet.Event) {
				if event.EventType == kendynet.EventTypeError {
					event.Session.Close(event.Data.(error).Error(), 0)
				} else {
					session.Send(cs.NewMessage(0, toS))
				}
			})
			session.Send(cs.NewMessage(0, toS))
		}
	}
}

func main() {

	pb.Register("cs", &testproto.ChatToS{}, 1)
	pb.Register("sc", &testproto.ChatToC{}, 1)

	if len(os.Args) < 3 {
		fmt.Printf("usage ./clisvr [server|client|both] ip:port clientcount\n")
		return
	}

	mode := os.Args[1]

	if !(mode == "server" || mode == "client" || mode == "both") {
		fmt.Printf("usage ./clisvr [server|client|both] ip:port clientcount\n")
		return
	}

	service := os.Args[2]

	c := make(chan os.Signal)
	signal.Notify(c, syscall.SIGINT) //监听指定信号
	if mode == "server" || mode == "both" {
		go server(service)
	}

	if mode == "client" || mode == "both" {
		if len(os.Args) < 4 {
			fmt.Printf("usage ./clisvr [server|client|both] ip:port clientcount\n")
			return
		}
		connectioncount, err := strconv.Atoi(os.Args[3])
		if err != nil {
			fmt.Printf(err.Error())
			return
		}
		//让服务器先运行
		time.Sleep(10000000)
		go client(service, connectioncount)

	}

	_ = <-c //阻塞直至有信号传入

	return

}
