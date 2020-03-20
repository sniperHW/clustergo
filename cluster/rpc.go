package cluster

import (
	"fmt"
	"github.com/sniperHW/sanguo/cluster/addr"
	"github.com/sniperHW/sanguo/codec/ss"
	"reflect"
	"sync/atomic"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet/rpc"
)

var rpcServer *rpc.RPCServer
var rpcClient *rpc.RPCClient

var pendingRPCReqCount int32

type RPCChannel struct {
	to   addr.LogicAddr
	peer *endPoint
}

func (this *RPCChannel) SendRequest(message interface{}) error {
	if this.peer.session != nil {
		if this.to != this.peer.addr.Logic {
			return this.peer.session.Send(ss.NewMessage(message, this.to, selfAddr.Logic))
		} else {
			return this.peer.session.Send(message)
		}
	} else {
		return ERR_RPC_NO_CONN
	}
}

func (this *RPCChannel) SendResponse(message interface{}) error {
	atomic.AddInt32(&pendingRPCReqCount, -1)
	if this.peer.session != nil {
		if this.to != this.peer.addr.Logic {
			return this.peer.session.Send(ss.NewMessage(message, this.to, selfAddr.Logic))
		} else {
			return this.peer.session.Send(message)
		}
	} else {
		return ERR_RPC_NO_CONN
	}
}

func (this *RPCChannel) Name() string {
	return fmt.Sprintf("%s <-> %s", selfAddr.Logic.String(), this.peer.addr.Logic.String())
}

func (this *RPCChannel) PeerAddr() addr.LogicAddr {
	return this.peer.addr.Logic
}

type encoder struct {
}

func (this *encoder) Encode(message rpc.RPCMessage) (interface{}, error) {
	return message, nil
}

type decoder struct {
}

func (this *decoder) Decode(o interface{}) (rpc.RPCMessage, error) {
	return o.(rpc.RPCMessage), nil
}

/*
*  注册RPC服务,无锁保护，务必在初始化时完成
 */
func RegisterMethod(arg proto.Message, handler rpc.RPCMethodHandler) {
	rpcServer.RegisterMethod(reflect.TypeOf(arg).String(), handler)
}

func onRPCRequest(peer *endPoint, from addr.LogicAddr, msg *rpc.RPCRequest) {
	if !IsStoped() {
		atomic.AddInt32(&pendingRPCReqCount, 1)
		rpcchan := &RPCChannel{
			peer: peer,
			to:   from,
		}
		rpcServer.OnRPCMessage(rpcchan, msg)
	}
}

func onRPCResponse(msg *rpc.RPCResponse) {
	rpcClient.OnRPCMessage(msg)
}

func asynCall(end *endPoint, arg proto.Message, timeout uint32, cb rpc.RPCResponseHandler) {
	end.mtx.Lock()
	defer end.mtx.Unlock()

	if nil != end.session {
		if err := rpcClient.AsynCall(&RPCChannel{
			to:   end.addr.Logic,
			peer: end,
		}, "call", arg, time.Duration(timeout)*time.Millisecond, cb); nil != err {
			//记录日志
			Errorf("Call %s:%s error:%s\n", end.addr.Logic.String(), reflect.TypeOf(arg).String(), err.Error())
			queue.PostNoWait(func() { cb(nil, err) })
		}
	} else {
		end.pendingCall = append(end.pendingCall, &rpcCall{
			arg:      arg,
			cb:       cb,
			deadline: time.Now().Add(time.Duration(timeout) * time.Millisecond),
			to:       end.addr.Logic,
		})
		//尝试与对端建立连接
		dial(end)
	}
}

/*
*  异步RPC调用
 */
func AsynCall(peer addr.LogicAddr, arg proto.Message, timeout uint32, cb rpc.RPCResponseHandler) {

	if atomic.LoadInt32(&started) == 0 {
		panic("cluster not started")
	}

	endPoint := getEndPoint(peer)

	if nil == endPoint {
		if peer.Group() == selfAddr.Logic.Group() {
			//记录日志
			Errorf("Call %s not found", peer.String())
			queue.PostNoWait(func() { cb(nil, fmt.Errorf("%s not found", peer.String())) })
			return
		} else {
			//不同服务器组，需要通告Harbor转发
			harbor := getHarbor()
			if nil == harbor {
				Errorf("Call %s not found", peer.String())
				queue.PostNoWait(func() { cb(nil, fmt.Errorf("%s not found", peer.String())) })
				return
			} else {
				endPoint = harbor
			}
		}
	}

	endPoint.mtx.Lock()
	defer endPoint.mtx.Unlock()

	if nil != endPoint.session {
		if err := rpcClient.AsynCall(&RPCChannel{
			to:   peer,
			peer: endPoint,
		}, "call", arg, time.Duration(timeout)*time.Millisecond, cb); nil != err {
			//记录日志
			Errorf("Call %s:%s error:%s\n", peer.String(), reflect.TypeOf(arg).String(), err.Error())
			queue.PostNoWait(func() { cb(nil, err) })
		}
	} else {
		endPoint.pendingCall = append(endPoint.pendingCall, &rpcCall{
			arg:      arg,
			cb:       cb,
			deadline: time.Now().Add(time.Duration(timeout) * time.Millisecond),
			to:       peer,
		})
		//尝试与对端建立连接
		dial(endPoint)
	}
}
