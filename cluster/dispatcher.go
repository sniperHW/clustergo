package cluster

import (
	"github.com/sniperHW/sanguo/cluster/addr"
	"runtime"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
)

type MsgHandler func(addr.LogicAddr, proto.Message)

var mtxHandler sync.Mutex
var handlers = map[uint16]MsgHandler{}
var onPeerDisconnected func(addr.LogicAddr, error)

func RegisterPeerDisconnected(cb func(addr.LogicAddr, error)) {
	defer mtxHandler.Unlock()
	mtxHandler.Lock()
	onPeerDisconnected = cb
}

func Register(cmd uint16, handler MsgHandler) {
	defer mtxHandler.Unlock()
	mtxHandler.Lock()

	if nil == handler {
		//记录日志
		Errorf("Register %d failed: handler is nil\n", cmd)
		return
	}
	_, ok := handlers[cmd]
	if ok {
		//记录日志
		Errorf("Register %d failed: duplicate handler\n", cmd)
		return
	}

	handlers[cmd] = handler
}

func pcall(handler MsgHandler, from addr.LogicAddr, cmd uint16, msg proto.Message) {
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 65535)
			l := runtime.Stack(buf, false)
			Errorf("error on Dispatch:%d\nstack:%v,%s\n", cmd, r, buf[:l])
		}
	}()
	handler(from, msg)
}

func dispatch(from addr.LogicAddr, session kendynet.StreamSession, cmd uint16, msg proto.Message) { //from addr.LogicAddr, cmd uint16, msg proto.Message) {
	if IsStoped() {
		return
	}

	if nil != msg {
		switch msg.(type) {
		case *Heartbeat:
			if msg.(*Heartbeat).GetOriginSender() != uint32(selfAddr.Logic) {
				heartbeat := msg.(*Heartbeat)
				heartbeat_resp := &Heartbeat{}
				heartbeat_resp.OriginSender = proto.Uint32(msg.(*Heartbeat).GetOriginSender())
				heartbeat_resp.Timestamp1 = proto.Int64(time.Now().UnixNano())
				heartbeat_resp.Timestamp2 = proto.Int64(heartbeat.GetTimestamp1())
				session.Send(heartbeat_resp)
			}
			break
		default:
			mtxHandler.Lock()
			handler, ok := handlers[cmd]
			mtxHandler.Unlock()
			if ok {
				pcall(handler, from, cmd, msg)
			} else {
				//记录日志
				Errorf("unkonw cmd:%d\n", cmd)
			}
			break
		}
	}
}

func dispatchPeerDisconnected(peer addr.LogicAddr, err error) {
	var h func(addr.LogicAddr, error)
	mtxHandler.Lock()
	h = onPeerDisconnected
	mtxHandler.Unlock()
	if nil != h {
		h(peer, err)
	}
}
