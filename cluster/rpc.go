package cluster

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet/rpc"
	"github.com/sniperHW/kendynet/timer"
	"github.com/sniperHW/sanguo/cluster/addr"
	"github.com/sniperHW/sanguo/cluster/rpcerr"
	"github.com/sniperHW/sanguo/codec/ss"
	"reflect"
	"sync/atomic"
	"time"
)

//只用于单元测试
var testRPCTimeout bool

type rpcManager struct {
	cluster *Cluster
	server  *rpc.RPCServer
	client  *rpc.RPCClient
}

func (this *rpcManager) onRPCRequest(peer *endPoint, from addr.LogicAddr, req *rpc.RPCRequest) {
	rpcchan := &RPCChannel{
		peer:    peer,
		to:      from,
		cluster: this.cluster,
	}

	if !this.cluster.IsStoped() {
		this.server.OnRPCMessage(rpcchan, req)
	} else {
		this.server.OnServiceStop(rpcchan, req, rpcerr.Err_RPC_ServiceStoped)
	}
}

func (this *rpcManager) onRPCResponse(msg *rpc.RPCResponse) {
	this.client.OnRPCMessage(msg)
}

func (this *rpcManager) asynCall(channel rpc.RPCChannel, method string, arg interface{}, timeout time.Duration, cb rpc.RPCResponseHandler) error {
	return this.client.AsynCall(channel, method, arg, timeout, cb)
}

func (this *rpcManager) onEndDisconnected(end *endPoint) {
	this.client.OnChannelDisconnect(&RPCChannel{
		peer:    end,
		cluster: this.cluster,
	})
}

type RPCChannel struct {
	to      addr.LogicAddr
	peer    *endPoint
	cluster *Cluster
}

func (this *RPCChannel) SendRequest(message interface{}) error {

	//peer.session已经被调用方确保非nil

	var msg interface{}

	if this.to != this.peer.addr.Logic {
		msg = ss.NewMessage(message, this.to, this.cluster.serverState.selfAddr.Logic)
	} else {
		msg = message
	}

	err := this.peer.session.Send(msg)

	if nil == err {
		this.peer.lastActive = time.Now()
	}

	return err

}

func (this *RPCChannel) SendResponse(message interface{}) error {

	this.peer.Lock()
	this.peer.Unlock()

	var msg interface{}

	if this.to != this.peer.addr.Logic {
		msg = ss.NewMessage(message, this.to, this.cluster.serverState.selfAddr.Logic)
	} else {
		msg = message
	}

	var err error

	if this.peer.session != nil {
		err = this.peer.session.Send(msg)
		if nil == err {
			this.peer.lastActive = time.Now()
		}
	} else {
		this.peer.pendingMsg = append(this.peer.pendingMsg, msg)
		//尝试与对端建立连接
		this.cluster.dial(this.peer, 0)
	}

	return err

}

func (this *RPCChannel) Name() string {
	//return fmt.Sprintf("%s <-> %s", this.cluster.serverState.selfAddr.Logic.String(), this.peer.addr.Logic.String())
	return this.peer.addr.Logic.String()
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
func (this *Cluster) RegisterMethod(arg proto.Message, handler rpc.RPCMethodHandler) {
	this.rpcMgr.server.RegisterMethod(reflect.TypeOf(arg).String(), handler)
}

func (this *Cluster) responseCbHook(callback rpc.RPCResponseHandler) rpc.RPCResponseHandler {
	atomic.AddInt32(&this.pendingRPCRequestCount, 1)
	return func(ret interface{}, err error) {
		this.queue.PostNoWait(func(ret interface{}, err error) {
			defer atomic.AddInt32(&this.pendingRPCRequestCount, -1)
			callback(ret, err)
		}, ret, err)
	}
}

/*
 * 调用peer的方法
 * 如果peer == end.addr.Logic表示直接连接调用
 * 否则，表示需要通过end将请求路由到peer
 */
func (this *Cluster) asynCall(peer addr.LogicAddr, end *endPoint, arg proto.Message, timeout uint32, callback rpc.RPCResponseHandler, donthookCallback ...bool) {
	end.Lock()
	defer end.Unlock()

	var cb rpc.RPCResponseHandler

	if len(donthookCallback) > 0 && donthookCallback[0] {
		cb = callback
	} else {
		cb = this.responseCbHook(callback)
	}

	if nil != end.session {
		if err := this.rpcMgr.asynCall(&RPCChannel{
			to:      peer,
			peer:    end,
			cluster: this,
		}, "call", arg, time.Duration(timeout)*time.Millisecond, cb); nil != err {
			//记录日志
			logger.Errorf("Call %s:%s error:%s\n", end.addr.Logic.String(), reflect.TypeOf(arg).String(), err.Error())
			cb(nil, err)
		} else {
			end.lastActive = time.Now()
		}
	} else {
		end.pendingCall = append(end.pendingCall, &rpcCall{
			arg: arg,
			cb:  cb,
			dialTimer: timer.Once(time.Duration(timeout)*time.Millisecond, func(t *timer.Timer, ctx interface{}) {
				cb(nil, rpc.ErrCallTimeout)
			}, nil),
			deadline: time.Now().Add(time.Duration(timeout) * time.Millisecond),
			to:       peer,
		})
		//尝试与对端建立连接
		this.dial(end, 0)
	}
}

/*
*  异步RPC调用
 */
func (this *Cluster) AsynCall(peer addr.LogicAddr, arg proto.Message, timeout uint32, callback rpc.RPCResponseHandler) {
	if atomic.LoadInt32(&this.serverState.started) == 0 {
		callback(nil, fmt.Errorf("cluster not start"))
	} else {
		cb := this.responseCbHook(callback)
		if peer.Empty() {
			cb(nil, fmt.Errorf("invaild peerAddr %s", peer.String()))
		} else {

			endPoint := this.serviceMgr.getEndPoint(peer)

			if nil == endPoint {
				if peer.Group() == this.serverState.selfAddr.Logic.Group() {
					//记录日志
					logger.Errorf("Call %s not found", peer.String())
					cb(nil, fmt.Errorf("%s not found", peer.String()))
					return
				} else {
					//不同服务器组，需要通告Harbor转发
					harbor := this.serviceMgr.getHarbor(peer)
					if nil == harbor {
						logger.Errorf("Call %s not found", peer.String())
						cb(nil, fmt.Errorf("%s not found", peer.String()))
						return
					} else {
						endPoint = harbor
					}
				}
			}

			this.asynCall(peer, endPoint, arg, timeout, cb, true)

		}
	}
}
