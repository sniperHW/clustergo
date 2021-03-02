package cs

import (
	"errors"
	"github.com/sniperHW/kendynet"
	codecs "github.com/sniperHW/sanguo/codec/cs"
	constant "github.com/sniperHW/sanguo/common"
	"github.com/sniperHW/sanguo/network"
	_ "github.com/sniperHW/sanguo/protocol/cs" //触发pb注册
	cs_proto "github.com/sniperHW/sanguo/protocol/cs/message"
	"net"
	"sync"
	"time"
)

type server struct {
	startOnce  sync.Once
	closeOnce  sync.Once
	dispatcher ServerDispatcher
	listener   net.Listener
	nettype    string
	service    string
}

func newServer(nettype, service string, d ServerDispatcher) *server {
	return &server{
		dispatcher: d,
		nettype:    nettype,
		service:    service,
	}
}

func (this *server) Start() (err error) {
	this.startOnce.Do(func() {
		this.listener, serve, err = network.Listen(this.nettype, this.service, func(conn net.Conn) {
			go func() {

				session := network.CreateSession(conn)

				if !this.dispatcher.OnAuthenticate(session) {
					session.Close(errors.New("authenticate"), 0)
					return
				}

				session.SetRecvTimeout(time.Second * 3)
				session.SetInBoundProcessor(codecs.NewReceiver("cs"))
				session.SetEncoder(codecs.NewEncoder("sc"))
				session.SetCloseCallBack(func(sess kendynet.StreamSession, reason error) {
					this.dispatcher.OnClose(sess, reason)
				})

				this.dispatcher.OnNewClient(session)
				recved := false

				session.BeginRecv(func(s kendynet.StreamSession, m interface{}) {
					if !recved {
						recved = true
						session.SetRecvTimeout(constant.HeartBeat_Timeout_Client * time.Second)
					}
					msg := m.(*codecs.Message)
					switch msg.GetData().(type) {
					case *cs_proto.HeartbeatToS:
						this.dispatcher.Dispatch(session, msg)
						break
					default:
						this.dispatcher.Dispatch(session, msg)
						break
					}
				})
			}()
		})
		if nil != err {
			go serve()
			return
		}
	})
	return
}

func (this *server) Close() {
	this.closeOnce.Do(func() {
		this.listener.Close()
	})
}

var defaultServer *server
var startOnce sync.Once
var closeOnce sync.Once

func StartTcpServer(nettype, service string, d ServerDispatcher) (err error) {
	startOnce.Do(func() {
		defaultServer = newServer(nettype, service, d)
		err = defaultServer.Start()
	})
	return
}

func StopServer() {
	closeOnce.Do(func() {
		if nil != defaultServer {
			defaultServer.Close()
		}
	})
}
