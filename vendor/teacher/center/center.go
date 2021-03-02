package center

import (
	"errors"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/event"
	"github.com/sniperHW/kendynet/golog"
	"github.com/sniperHW/kendynet/rpc"
	"github.com/sniperHW/sanguo/center/constant"
	"github.com/sniperHW/sanguo/center/protocol"
	center_rpc "github.com/sniperHW/sanguo/center/rpc"
	"github.com/sniperHW/sanguo/cluster/addr"
	"github.com/sniperHW/sanguo/codec/ss"
	"github.com/sniperHW/sanguo/common"
	"github.com/sniperHW/sanguo/network"
	"net"
	"reflect"
	"sync/atomic"
	"time"
)

type MsgHandler func(kendynet.StreamSession, proto.Message)

type node struct {
	addr          addr.Addr
	session       kendynet.StreamSession
	exportService uint32
	heartBeatTime time.Time
}

type Center struct {
	queue     *event.EventQueue
	handlers  map[uint16]MsgHandler
	nodes     map[addr.LogicAddr]*node
	rpcServer *rpc.RPCServer
	logger    golog.LoggerI
	l         net.Listener
	stoped    int32
	started   int32
	selfAddr  string
	die       chan struct{}
}

func (this *Center) tick() {
	ticker := time.NewTicker(time.Second)
	for {
		now := <-ticker.C
		err := this.queue.PostNoWait(0, func() {
			for logicAddr, node := range this.nodes {
				now = time.Now()
				if now.Unix()-node.heartBeatTime.Unix() > constant.HeartBeatTimeout {
					this.removeNode(logicAddr)
				}
			}
		})
		if nil != err {
			return
		}
	}
}

func (this *Center) removeNode(logicAddr addr.LogicAddr) {
	msg := &protocol.NodeLeave{
		Nodes: []uint32{uint32(logicAddr)},
	}
	this.logger.Infoln("removeNode", logicAddr)
	delete(this.nodes, logicAddr)
	this.brocast(msg, nil)
}

func (this *Center) brocast(msg proto.Message, exclude *map[kendynet.StreamSession]bool) {
	for _, v := range this.nodes {
		if v.session != nil {
			if nil == exclude {
				this.logger.Infoln("brocast notify to", v.addr.Logic, msg)
				v.session.Send(msg)
			} else {
				if _, ok := (*exclude)[v.session]; !ok {
					this.logger.Infoln(this.selfAddr, "brocast notify to", v.addr.Logic, msg)
					v.session.Send(msg)
				}
			}
		}
	}
}

func (this *Center) onSessionClose(session kendynet.StreamSession, reason error) {
	ud := session.GetUserData()
	if nil != ud {
		n := ud.(*node)
		//removeNode(n.addr.Logic)
		this.logger.Infof("node lose connection %s reason:%s\n", n.addr.Logic.String(), reason.Error())
		n.session = nil
	}
}

func (this *Center) onLogin(replyer *rpc.RPCReplyer, req interface{}) {

	session := replyer.GetChannel().(*center_rpc.RPCChannel).GetSession()

	ud := session.GetUserData()
	if nil != ud {
		return
	}

	login := req.(*protocol.Login)

	var n *node

	logicAddr := addr.LogicAddr(login.GetLogicAddr())

	netAddr, err := net.ResolveTCPAddr("tcp", login.GetNetAddr())

	if nil != err {
		loginRet := &protocol.LoginRet{
			ErrCode: proto.Int32(constant.LoginFailed),
			Msg:     proto.String("invaild netAddr:" + login.GetNetAddr()),
		}
		replyer.Reply(loginRet, nil)
		this.logger.Infoln(loginRet.GetMsg())
		session.Close(errors.New("invaild netAddr"), 1)
		return
	}

	n, ok := this.nodes[logicAddr]
	if !ok {
		n = &node{
			addr: addr.Addr{
				Logic: logicAddr,
				Net:   netAddr,
			},
			exportService: login.GetExportService(),
		}
		this.nodes[logicAddr] = n
		this.logger.Infoln("add new node", logicAddr.String(), "exportService", 1 == n.exportService)
	}

	if n.session != nil {
		//重复登录
		loginRet := &protocol.LoginRet{
			ErrCode: proto.Int32(constant.LoginFailed),
			Msg:     proto.String("duplicate node:" + logicAddr.String()),
		}
		replyer.Reply(loginRet, nil)
		this.logger.Infoln(loginRet.GetMsg())
		session.Close(errors.New("duplicate node"), 1)
		return
	}

	n.session = session
	n.addr.Net = netAddr //更新网络地址
	n.heartBeatTime = time.Now()
	session.SetUserData(n)

	this.logger.Infoln(this.selfAddr, "onLogin", logicAddr.String(), "exportService", 1 == n.exportService)

	loginRet := &protocol.LoginRet{
		ErrCode: proto.Int32(constant.LoginOK),
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
	this.brocast(nodeAdd, &exclude)

	//将所有节点信息(包括自己)发给新到节点

	notify := &protocol.NotifyNodeInfo{}

	for _, v := range this.nodes {
		notify.Nodes = append(notify.Nodes, &protocol.NodeInfo{
			LogicAddr:     proto.Uint32(uint32(v.addr.Logic)),
			NetAddr:       proto.String(v.addr.Net.String()),
			ExportService: proto.Uint32(v.exportService),
		})
	}

	err = session.Send(notify)
	if nil != err {
		this.logger.Errorln(err)
	} else {
		this.logger.Infoln(this.selfAddr, "send notify to ", logicAddr.String(), notify)
	}

}

func (this *Center) onHeartBeat(session kendynet.StreamSession, msg proto.Message) {
	ud := session.GetUserData()
	if nil == ud {
		this.logger.Infof("onHeartBeat,session is not login")
		return
	}
	node := ud.(*node)
	node.heartBeatTime = time.Now()
	//kendynet.Infoln("onHeartBeat", node)

	heartbeat := msg.(*protocol.HeartbeatToCenter)
	resp := &protocol.HeartbeatToNode{}
	resp.TimestampBack = proto.Int64(heartbeat.GetTimestamp())
	resp.Timestamp = proto.Int64(time.Now().UnixNano())
	err := session.Send(resp)
	if nil != err {
		this.logger.Errorf("send error:%s\n", err.Error())
	}
}

func (this *Center) onRemoveNode(session kendynet.StreamSession, msg proto.Message) {
	this.logger.Infoln("onRemoveNode")
	removeNode := msg.(*protocol.RemoveNode)
	for _, v := range removeNode.Nodes {
		this.removeNode(addr.LogicAddr(v))
	}
}

func (this *Center) dispatchMsg(session kendynet.StreamSession, msg *ss.Message) {
	if nil != msg {
		data := msg.GetData()
		switch data.(type) {
		case *rpc.RPCRequest:

			center_rpc.OnRPCRequest(this.rpcServer, session, data.(*rpc.RPCRequest))

		//case *rpc.RPCResponse:
		//	center_rpc.OnRPCResponse(rpcServer.data.(*rpc.RPCResponse))
		case proto.Message:
			cmd := msg.GetCmd()
			handler, ok := this.handlers[cmd]
			if ok {
				handler(session, msg.GetData().(proto.Message))
			} else {
				//记录日志
				this.logger.Errorf("unknow cmd:%s\n", cmd)
			}
		default:
			this.logger.Errorf("invaild message type:%s \n", reflect.TypeOf(data).String())
		}
	}
}

func (this *Center) registerHandler(cmd uint16, handler MsgHandler) {
	if nil == handler {
		//记录日志
		this.logger.Errorf("Register %d failed: handler is nil\n", cmd)
		return
	}
	_, ok := this.handlers[cmd]
	if ok {
		//记录日志
		this.logger.Errorf("Register %d failed: duplicate handler\n", cmd)
		return
	}
	this.handlers[cmd] = handler
}

func (this *Center) Stop() {
	if atomic.CompareAndSwapInt32(&this.stoped, 0, 1) {
		this.l.Close()
		this.queue.PostNoWait(0, func() {
			for _, v := range this.nodes {
				if nil != v.session {
					v.session.Close(errors.New("center stop"), 0)
				}
			}
		})
		this.queue.Close()
		<-this.die
	}
}

func (this *Center) Start(service string, l golog.LoggerI) {

	if !atomic.CompareAndSwapInt32(&this.started, 0, 1) {
		return
	}

	this.selfAddr = service
	this.logger = l
	this.handlers = map[uint16]MsgHandler{}
	this.nodes = map[addr.LogicAddr]*node{}
	this.rpcServer = center_rpc.NewServer()
	this.die = make(chan struct{})
	go this.tick()

	center_rpc.RegisterMethod(this.rpcServer, &protocol.Login{}, this.onLogin)

	this.registerHandler(protocol.CENTER_HeartbeatToCenter, this.onHeartBeat)

	this.registerHandler(protocol.CENTER_RemoveNode, this.onRemoveNode)

	this.queue = event.NewEventQueue()

	go func() {
		this.queue.Run()
		this.die <- struct{}{}
	}()

	//启动本地监听

	server, serve, err := network.Listen("tcp", service, func(conn net.Conn) {
		session := network.CreateSession(conn)
		session.SetRecvTimeout(common.HeartBeat_Timeout * time.Second)
		session.SetInBoundProcessor(protocol.NewReceiver())
		session.SetEncoder(protocol.NewEncoder())
		session.SetCloseCallBack(func(sess kendynet.StreamSession, reason error) {

			this.queue.PostNoWait(0, this.onSessionClose, sess, reason)
		}).BeginRecv(func(s kendynet.StreamSession, m interface{}) {
			msg := m.(*ss.Message)
			this.queue.PostNoWait(0, this.dispatchMsg, session, msg)
		})
	})

	if nil == err {
		this.l = server
		fmt.Printf("server running on:%s\n", service)
		go serve()
	} else {
		fmt.Printf("center failed %s\n", err.Error())
	}

}

func New() *Center {
	return &Center{}
}

var defaultCenter *Center = New()

func Stop() {
	defaultCenter.Stop()
}

func Start(service string, l golog.LoggerI) {
	kendynet.InitLogger(l)
	defaultCenter.Start(service, l)
}
