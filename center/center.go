package center

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/event"
	"github.com/sniperHW/kendynet/golog"
	"github.com/sniperHW/kendynet/rpc"
	listener "github.com/sniperHW/kendynet/socket/listener/tcp"
	"github.com/sniperHW/sanguo/center/protocol"
	"github.com/sniperHW/sanguo/cluster/addr"
	"github.com/sniperHW/sanguo/codec/ss"
	"github.com/sniperHW/sanguo/common"
	"net"
	"reflect"
	"time"
)

type MsgHandler func(kendynet.StreamSession, proto.Message)

type node struct {
	addr          addr.Addr
	session       kendynet.StreamSession
	exportService uint32
	heartBeatTime time.Time
}

var (
	queue            *event.EventQueue
	handlers         map[uint16]MsgHandler
	nodes            map[addr.LogicAddr]*node
	heartBeatTimeout int64 = 5
)

var (
	LoginOK     = int32(0)
	LoginFailed = int32(1)
)

func tick() {
	ticker := time.NewTicker(time.Second)
	for {
		now := <-ticker.C
		queue.PostNoWait(func() {
			for logicAddr, node := range nodes {
				now = time.Now()
				if now.Unix()-node.heartBeatTime.Unix() > heartBeatTimeout {
					removeNode(logicAddr)
				}
			}
		})
	}
}

func removeNode(logicAddr addr.LogicAddr) {
	msg := &protocol.NodeLeave{
		Nodes: []uint32{uint32(logicAddr)},
	}
	delete(nodes, logicAddr)
	brocast(msg, nil)
}

func brocast(msg proto.Message, exclude *map[kendynet.StreamSession]bool) {
	for _, v := range nodes {
		if v.session != nil {
			if nil == exclude {
				kendynet.Infoln("brocast notify to", v.addr.Logic, msg)
				v.session.Send(msg)
			} else {
				if _, ok := (*exclude)[v.session]; !ok {
					kendynet.Infoln("brocast notify to", v.addr.Logic, msg)
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
		//removeNode(n.addr.Logic)
		kendynet.Infof("node lose connection %s reason:%s\n", n.addr.Logic.String(), reason)
		n.session = nil
	}
}

func onLogin(replyer *rpc.RPCReplyer, req interface{}) {
	channel := replyer.GetChannel().(*RPCChannel)
	session := channel.session

	ud := session.GetUserData()
	if nil != ud {
		return
	}

	login := req.(*protocol.Login)

	var n *node

	logicAddr := addr.LogicAddr(login.GetLogicAddr())

	netAddr, err := net.ResolveTCPAddr("tcp4", login.GetNetAddr())

	if nil != err {
		loginRet := &protocol.LoginRet{
			ErrCode: proto.Int32(LoginFailed),
			Msg:     proto.String("invaild netAddr:" + login.GetNetAddr()),
		}
		replyer.Reply(loginRet, nil)
		kendynet.Infoln(loginRet.GetMsg())
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
			exportService: login.GetExportService(),
		}
		nodes[logicAddr] = n
		kendynet.Infoln("add new node", logicAddr.String())
	}

	if n.session != nil {
		//重复登录
		loginRet := &protocol.LoginRet{
			ErrCode: proto.Int32(LoginFailed),
			Msg:     proto.String("duplicate node:" + logicAddr.String()),
		}
		replyer.Reply(loginRet, nil)
		kendynet.Infoln(loginRet.GetMsg())
		session.Close("duplicate node", 1)
		return
	}

	n.session = session
	n.addr.Net = netAddr //更新网络地址
	n.heartBeatTime = time.Now()
	session.SetUserData(n)

	kendynet.Infoln("onLogin", logicAddr.String(), n.exportService)

	loginRet := &protocol.LoginRet{
		ErrCode: proto.Int32(LoginOK),
	}

	replyer.Reply(loginRet, nil)

	//记录日志
	nodeAdd := &protocol.NodeAdd{}

	nodeAdd.Nodes = append(nodeAdd.Nodes, &protocol.NodeInfo{
		LogicAddr:     proto.Uint32(uint32(n.addr.Logic)),
		NetAddr:       proto.String(n.addr.Net.String()),
		ExportService: proto.Uint32(n.exportService),
	})

	//将新节点的信息通告给除自己以外的其它节点
	exclude := map[kendynet.StreamSession]bool{}
	exclude[session] = true
	brocast(nodeAdd, &exclude)

	//将所有节点信息(包括自己)发给新到节点

	notify := &protocol.NotifyNodeInfo{}

	for _, v := range nodes {
		notify.Nodes = append(notify.Nodes, &protocol.NodeInfo{
			LogicAddr:     proto.Uint32(uint32(v.addr.Logic)),
			NetAddr:       proto.String(v.addr.Net.String()),
			ExportService: proto.Uint32(n.exportService),
		})
	}

	err = session.Send(notify)
	if nil != err {
		kendynet.Errorln(err)
	} else {
		kendynet.Infoln("send notify to ", logicAddr.String(), notify)
	}

}

func onHeartBeat(session kendynet.StreamSession, msg proto.Message) {
	ud := session.GetUserData()
	if nil == ud {
		kendynet.Infof("onHeartBeat,session is not login")
		return
	}
	node := ud.(*node)
	node.heartBeatTime = time.Now()

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
		data := msg.GetData()
		switch data.(type) {
		case *rpc.RPCRequest:
			onRPCRequest(session, data.(*rpc.RPCRequest))
		case *rpc.RPCResponse:
			onRPCResponse(data.(*rpc.RPCResponse))
		case proto.Message:
			cmd := msg.GetCmd()
			handler, ok := handlers[cmd]
			if ok {
				handler(session, msg.GetData().(proto.Message))
			} else {
				//记录日志
				kendynet.Errorf("unknow cmd:%s\n", cmd)
			}
		default:
			kendynet.Errorf("invaild message type:%s \n", reflect.TypeOf(data).String())
		}
	}
}

func registerHandler(cmd uint16, handler MsgHandler) {
	if nil == handler {
		//记录日志
		kendynet.Errorf("Register %d failed: handler is nil\n", cmd)
		return
	}
	_, ok := handlers[cmd]
	if ok {
		//记录日志
		kendynet.Errorf("Register %d failed: duplicate handler\n", cmd)
		return
	}
	handlers[cmd] = handler
}

func Start(service string) {
	outLogger := golog.NewOutputLogger("log", "center", 1024*1024*1000)
	kendynet.InitLogger(golog.New("center", outLogger))

	handlers = map[uint16]MsgHandler{}
	nodes = map[addr.LogicAddr]*node{}
	go tick()

	registerMethod(&protocol.Login{}, onLogin)

	registerHandler(protocol.CENTER_HeartbeatToCenter, onHeartBeat)

	queue = event.NewEventQueue()

	go func() {
		queue.Run()
	}()

	//启动本地监听
	server, err := listener.New("tcp4", service)
	if server != nil {
		fmt.Printf("server running on:%s\n", service)

		err = server.Serve(func(session kendynet.StreamSession) {
			session.SetRecvTimeout(common.HeartBeat_Timeout * time.Second)
			session.SetReceiver(protocol.NewReceiver())
			session.SetEncoder(protocol.NewEncoder())
			session.SetCloseCallBack(func(sess kendynet.StreamSession, reason string) {
				queue.PostNoWait(func() {
					onSessionClose(sess, reason)
				})
			})
			session.Start(func(event *kendynet.Event) {
				if event.EventType == kendynet.EventTypeError {
					event.Session.Close(event.Data.(error).Error(), 0)
				} else {
					msg := event.Data.(*ss.Message)
					queue.PostNoWait(func() {
						dispatchMsg(session, msg)
					})
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
