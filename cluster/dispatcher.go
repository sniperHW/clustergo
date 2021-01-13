package cluster

import (
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/sanguo/cluster/addr"
	cluster_proto "github.com/sniperHW/sanguo/cluster/proto"
	"runtime"
	"sync"
	"time"
)

type MsgHandler func(addr.LogicAddr, proto.Message)

type msgManager struct {
	sync.RWMutex
	cbPeerDisconnected func(addr.LogicAddr, error)
	msgHandlers        map[uint16]MsgHandler
}

func (this *msgManager) register(cmd uint16, handler MsgHandler) {
	this.Lock()
	defer this.Unlock()

	if nil == handler {
		//记录日志
		logger.Errorf("Register %d failed: handler is nil\n", cmd)
		return
	}
	_, ok := this.msgHandlers[cmd]
	if ok {
		//记录日志
		logger.Errorf("Register %d failed: duplicate handler\n", cmd)
		return
	}

	this.msgHandlers[cmd] = handler
}

func (this *msgManager) dispatch(from addr.LogicAddr, cmd uint16, msg proto.Message) {
	this.RLock()
	handler, ok := this.msgHandlers[cmd]
	this.RUnlock()
	if ok {
		pcall(handler, from, cmd, msg)
	} else {
		//记录日志
		logger.Errorf("unkonw cmd:%d\n", cmd)
	}
}

func (this *msgManager) setPeerDisconnected(cb func(addr.LogicAddr, error)) {
	defer this.Unlock()
	this.Lock()
	this.cbPeerDisconnected = cb
}

func (this *msgManager) onPeerDisconnected(peer addr.LogicAddr, err error) {
	this.RLock()
	h := this.cbPeerDisconnected
	this.RUnlock()
	if nil != h {
		defer func() {
			if r := recover(); r != nil {
				buf := make([]byte, 65535)
				l := runtime.Stack(buf, false)
				logger.Errorf("error on onPeerDisconnected\nstack:%v,%s\n", r, buf[:l])
			}
		}()
		h(peer, err)
	}
}

func (this *Cluster) onPeerDisconnected(peer addr.LogicAddr, err error) {
	this.msgMgr.onPeerDisconnected(peer, err)
}

func (this *Cluster) SetPeerDisconnected(cb func(addr.LogicAddr, error)) {
	this.msgMgr.setPeerDisconnected(cb)
}

func (this *Cluster) Register(cmd uint16, handler MsgHandler) {
	this.msgMgr.register(cmd, handler)
}

func pcall(handler MsgHandler, from addr.LogicAddr, cmd uint16, msg proto.Message) {
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 65535)
			l := runtime.Stack(buf, false)
			logger.Errorf("error on Dispatch:%d\nstack:%v,%s\n", cmd, r, buf[:l])
		}
	}()
	handler(from, msg)
}

func (this *Cluster) dispatch(from addr.LogicAddr, session kendynet.StreamSession, cmd uint16, msg proto.Message) { //from addr.LogicAddr, cmd uint16, msg proto.Message) {
	if this.IsStoped() {
		return
	}

	if nil != msg {
		switch msg.(type) {
		case *cluster_proto.Heartbeat:
			if msg.(*cluster_proto.Heartbeat).GetOriginSender() != uint32(this.serverState.selfAddr.Logic) {
				heartbeat := msg.(*cluster_proto.Heartbeat)
				heartbeat_resp := &cluster_proto.Heartbeat{}
				heartbeat_resp.OriginSender = proto.Uint32(msg.(*cluster_proto.Heartbeat).GetOriginSender())
				heartbeat_resp.Timestamp1 = proto.Int64(time.Now().UnixNano())
				heartbeat_resp.Timestamp2 = proto.Int64(heartbeat.GetTimestamp1())
				session.Send(heartbeat_resp)
			}
			break
		default:
			this.msgMgr.dispatch(from, cmd, msg)
			break
		}
	}
}
