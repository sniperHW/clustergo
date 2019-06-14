package ss

import (
	"fmt"
	codess "github.com/sniperHW/sanguo/codec/ss"
	"github.com/sniperHW/sanguo/common"
	_ "github.com/sniperHW/sanguo/protocol/ss" //触发pb注册
	ss_proto "github.com/sniperHW/sanguo/protocol/ss/message"
	"sync/atomic"
	"time"

	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/socket/stream_socket/tcp"
)

var (
	server     listener
	started    int32
	dispatcher ServerDispatcher
)

type listener interface {
	Close()
	Start() error
}

type tcpListener struct {
	l *tcp.Listener
}

func newTcpListener(nettype, service string) (*tcpListener, error) {
	var err error
	l := &tcpListener{}
	l.l, err = tcp.NewListener(nettype, service)

	if nil == err {
		return l, nil
	} else {
		return nil, err
	}
}

func (this *tcpListener) Close() {
	this.l.Close()
}

func (this *tcpListener) Start() error {
	if nil == this.l {
		return fmt.Errorf("invaild listener")
	}
	return this.l.Start(func(session kendynet.StreamSession) {
		//session.SetRecvTimeout(common.HeartBeat_Timeout * time.Second)
		session.SetReceiver(codess.NewReceiver("ss", "rpc_req", "rpc_resp"))
		session.SetEncoder(codess.NewEncoder("ss", "rpc_req", "rpc_resp"))
		session.SetCloseCallBack(func(sess kendynet.StreamSession, reason string) {
			dispatcher.OnClose(sess, reason)
		})

		dispatcher.OnNewClient(session)

		session.Start(func(event *kendynet.Event) {
			if event.EventType == kendynet.EventTypeError {
				event.Session.Close(event.Data.(error).Error(), 0)
			} else {
				msg := event.Data.(*codess.Message)
				switch msg.GetData().(type) {
				case *ss_proto.Heartbeat:
					session.Send(&ss_proto.Heartbeat{})
					break
				default:
					dispatcher.Dispatch(session, msg)
					break
				}
			}
		})
	})
}

func StartTcpServer(nettype, service string, d ServerDispatcher) error {
	l, err := newTcpListener(nettype, service)
	if nil != err {
		return err
	}
	return startServer(l, d)
}

func startServer(l listener, d ServerDispatcher) error {
	if !atomic.CompareAndSwapInt32(&started, 0, 1) {
		return fmt.Errorf("server already started")
	}

	server = l
	dispatcher = d

	go func() {
		err := server.Start()
		if nil != err {
			kendynet.Errorf("server.Start() error:%s\n", err.Error())
		}
	}()

	return nil
}

func StopServer() {
	if !atomic.CompareAndSwapInt32(&started, 1, 0) {
		return
	}
	server.Close()
}
