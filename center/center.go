package main

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/golog"
	listener "github.com/sniperHW/kendynet/socket/listener/tcp"
	"net"
	"os"
	"reflect"
	"sanguo/center/protocol"
	"sanguo/cluster/addr"
	"sanguo/codec/ss"
	"sanguo/common"
	"sync"
	"time"
)

type MsgHandler func(kendynet.StreamSession, proto.Message)

type node struct {
	addr    addr.Addr
	session kendynet.StreamSession
}

var (
	mtx      sync.Mutex
	handlers map[string]MsgHandler
	nodes    map[addr.LogicAddr]*node
)

func brocast(msg proto.Message, exclude *map[kendynet.StreamSession]bool) {
	mtx.Lock()
	defer mtx.Unlock()
	brocastNoLock(msg, exclude)
}

func brocastNoLock(msg proto.Message, exclude *map[kendynet.StreamSession]bool) {
	for _, v := range nodes {
		if v.session != nil {
			if nil == exclude {
				v.session.Send(msg)
			} else {
				if _, ok := (*exclude)[v.session]; !ok {
					v.session.Send(msg)
				}
			}
		}
	}
}

func onSessionClose(session kendynet.StreamSession, reason string) {
	ud := session.GetUserData()
	if nil != ud {
		n := ud.(*node)
		kendynet.Infof("node lose connection %s reason:%s\n", n.addr.Logic.String(), reason)
		n.session = nil
	}
}

func onLogin(session kendynet.StreamSession, msg proto.Message) {
	mtx.Lock()
	defer mtx.Unlock()
	kendynet.Infoln("onLogin\n")
	ud := session.GetUserData()
	if nil != ud {
		return
	}

	login := msg.(*protocol.Login)

	var n *node

	logicAddr := addr.LogicAddr(login.GetLogicAddr())

	netAddr, err := net.ResolveTCPAddr("tcp4", login.GetNetAddr())

	if nil != err {
		LoginFailed := &protocol.LoginFailed{}
		LoginFailed.Msg = proto.String("invaild netAddr:" + login.GetNetAddr())
		kendynet.Infoln(LoginFailed.GetMsg())
		session.Send(LoginFailed)
		session.Close("invaild netAddr", 1)
		return
	}

	n, ok := nodes[logicAddr]
	if !ok {
		n = &node{
			addr: addr.Addr{
				Logic: logicAddr,
				Net:   netAddr,
			},
		}
		nodes[logicAddr] = n
		kendynet.Infoln("add new node", logicAddr.String())
	}

	if n.session != nil {
		//重复登录
		LoginFailed := &protocol.LoginFailed{}
		LoginFailed.Msg = proto.String("duplicate node:" + logicAddr.String())

		kendynet.Infoln(LoginFailed.GetMsg())

		session.Send(LoginFailed)
		session.Close("duplicate node", 1)
		return
	}

	n.session = session
	session.SetUserData(n)

	//记录日志
	notify := &protocol.NotifyNodeInfo{}

	notify.Nodes = append(notify.Nodes, &protocol.NodeInfo{
		LogicAddr: proto.Uint32(uint32(n.addr.Logic)),
		NetAddr:   proto.String(n.addr.Net.String()),
	})

	//将新节点的信息通告给除自己以外的其它节点
	exclude := map[kendynet.StreamSession]bool{}
	exclude[session] = true
	brocastNoLock(notify, &exclude)
	notify.Reset()

	for _, v := range nodes {
		notify.Nodes = append(notify.Nodes, &protocol.NodeInfo{
			LogicAddr: proto.Uint32(uint32(v.addr.Logic)),
			NetAddr:   proto.String(v.addr.Net.String()),
		})
	}

	//将所有节点信息发给新到节点
	err = session.Send(notify)
	if nil != err {
		kendynet.Errorln(err)
	} else {
		kendynet.Infoln("send notify to ", logicAddr.String())
	}

}

func onHeartBeat(session kendynet.StreamSession, msg proto.Message) {
	//kendynet.Infof("onHeartBeat\n")
	heartbeat := msg.(*protocol.HeartbeatToCenter)
	resp := &protocol.HeartbeatToNode{}
	resp.TimestampBack = proto.Int64(heartbeat.GetTimestamp())
	resp.Timestamp = proto.Int64(time.Now().UnixNano())
	err := session.Send(resp)
	if nil != err {
		kendynet.Errorf("send error:%s\n", err.Error())
	}
}

func dispatchMsg(session kendynet.StreamSession, msg *ss.Message) {
	if nil != msg {
		name := msg.GetName()
		handler, ok := handlers[name]
		if ok {
			handler(session, msg.GetData().(proto.Message))
		} else {
			//记录日志
			kendynet.Errorf("unkonw msg:%s\n", name)
		}
	}
}

func registerHandler(msg proto.Message, handler MsgHandler) {
	msgName := reflect.TypeOf(msg).String()
	if nil == handler {
		//记录日志
		kendynet.Errorf("Register %s failed: handler is nil\n", msgName)
		return
	}
	_, ok := handlers[msgName]
	if ok {
		//记录日志
		kendynet.Errorf("Register %s failed: duplicate handler\n", msgName)
		return
	}
	handlers[msgName] = handler
}

func main() {
	outLogger := golog.NewOutputLogger("log", "center", 1024*1024*1000)
	kendynet.InitLogger(golog.New("center", outLogger))

	handlers = map[string]MsgHandler{}
	nodes = map[addr.LogicAddr]*node{}

	registerHandler(&protocol.Login{}, onLogin)
	registerHandler(&protocol.HeartbeatToCenter{}, onHeartBeat)

	service := os.Args[1]

	//启动本地监听
	server, err := listener.New("tcp4", service)
	if server != nil {
		fmt.Printf("server running on:%s\n", service)

		err = server.Serve(func(session kendynet.StreamSession) {
			session.SetRecvTimeout(common.HeartBeat_Timeout * time.Second)
			session.SetReceiver(protocol.NewReceiver())
			session.SetEncoder(protocol.NewEncoder())
			session.SetCloseCallBack(func(sess kendynet.StreamSession, reason string) {
				onSessionClose(sess, reason)
			})
			session.Start(func(event *kendynet.Event) {
				if event.EventType == kendynet.EventTypeError {
					event.Session.Close(event.Data.(error).Error(), 0)
				} else {
					msg := event.Data.(*ss.Message)
					dispatchMsg(session, msg)
				}
			})
		})
		if nil != err {
			fmt.Printf("center start failed %s\n", err)
		}
	} else {
		fmt.Printf("center failed %s\n", err)
	}

}
