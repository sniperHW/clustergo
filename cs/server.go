package cs

import (
	"fmt"
	"github.com/sniperHW/kendynet"
	listener "github.com/sniperHW/kendynet/socket/listener/tcp"
	codecs "github.com/sniperHW/sanguo/codec/cs"
	_ "github.com/sniperHW/sanguo/protocol/cs" //触发pb注册
	"sync/atomic"
)

var (
	server     listener_
	started    int32
	dispatcher ServerDispatcher
)

type listener_ interface {
	Close()
	Start() error
}

type tcpListener struct {
	l *listener.Listener
}

func newTcpListener(nettype, service string) (*tcpListener, error) {
	var err error
	l := &tcpListener{}
	l.l, err = listener.New(nettype, service)

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
	return this.l.Serve(func(session kendynet.StreamSession) {
		//session.SetRecvTimeout(common.HeartBeat_Timeout_Client * time.Second)
		session.SetReceiver(codecs.NewReceiver("cs"))
		session.SetEncoder(codecs.NewEncoder("sc"))
		session.SetCloseCallBack(func(sess kendynet.StreamSession, reason string) {
			dispatcher.OnClose(sess, reason)
		})

		dispatcher.OnNewClient(session)

		session.Start(func(event *kendynet.Event) {
			if event.EventType == kendynet.EventTypeError {
				event.Session.Close(event.Data.(error).Error(), 0)
			} else {
				dispatcher.Dispatch(session, event.Data.(*codecs.Message))
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

func startServer(l listener_, d ServerDispatcher) error {
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
