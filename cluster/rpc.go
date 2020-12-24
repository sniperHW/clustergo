package cluster

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet/rpc"
	"github.com/sniperHW/sanguo/cluster/addr"
	"github.com/sniperHW/sanguo/cluster/rpcerr"
	"github.com/sniperHW/sanguo/codec/ss"
	"reflect"
	"sync/atomic"
	"time"
)

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
		this.server.DirectReplyError(rpcchan, req, rpcerr.Err_RPC_ServiceStoped)
	}
}

func (this *rpcManager) onRPCResponse(msg *rpc.RPCResponse) {
	this.client.OnRPCMessage(msg)
}

func (this *rpcManager) asynCall(channel rpc.RPCChannel, method string, arg interface{}, timeout time.Duration, cb rpc.RPCResponseHandler) error {
	return this.client.AsynCall(channel, method, arg, timeout, cb)
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
	return fmt.Sprintf("%s <-> %s", this.cluster.serverState.selfAddr.Logic.String(), this.peer.addr.Logic.String())
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

func (this *Cluster) asynCall(end *endPoint, arg proto.Message, timeout uint32, callback rpc.RPCResponseHandler) {
	end.Lock()
	defer end.Unlock()

	cb := callback

	if nil != end.session {
		if err := this.rpcMgr.asynCall(&RPCChannel{
			to:      end.addr.Logic,
			peer:    end,
			cluster: this,
		}, "call", arg, time.Duration(timeout)*time.Millisecond, cb); nil != err {
			//记录日志
			logger.Errorf("Call %s:%s error:%s\n", end.addr.Logic.String(), reflect.TypeOf(arg).String(), err.Error())
			this.queue.PostNoWait(func() { cb(nil, err) })
		} else {
			end.lastActive = time.Now()
		}
	} else {
		end.pendingCall = append(end.pendingCall, &rpcCall{
			arg:      arg,
			cb:       cb,
			deadline: time.Now().Add(time.Duration(timeout) * time.Millisecond),
			to:       end.addr.Logic,
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
		panic("cluster not started")
	}

	cb := callback

	if peer.Empty() {
		this.queue.PostNoWait(func() { cb(nil, fmt.Errorf("invaild peerAddr %s", peer.String())) })
		return
	}

	endPoint := this.serviceMgr.getEndPoint(peer)

	if nil == endPoint {
		if peer.Group() == this.serverState.selfAddr.Logic.Group() {
			//记录日志
			logger.Errorf("Call %s not found", peer.String())
			this.queue.PostNoWait(func() { cb(nil, fmt.Errorf("%s not found", peer.String())) })
			return
		} else {
			//不同服务器组，需要通告Harbor转发
			harbor := this.serviceMgr.getHarbor(peer)
			if nil == harbor {
				logger.Errorf("Call %s not found", peer.String())
				this.queue.PostNoWait(func() { cb(nil, fmt.Errorf("%s not found", peer.String())) })
				return
			} else {
				endPoint = harbor
			}
		}
	}

	endPoint.Lock()
	defer endPoint.Unlock()

	if nil != endPoint.session {
		if err := this.rpcMgr.asynCall(&RPCChannel{
			to:      peer,
			peer:    endPoint,
			cluster: this,
		}, "call", arg, time.Duration(timeout)*time.Millisecond, cb); nil != err {
			//记录日志
			logger.Errorf("Call %s:%s error:%s\n", peer.String(), reflect.TypeOf(arg).String(), err.Error())
			this.queue.PostNoWait(func() { cb(nil, err) })
		} else {
			endPoint.lastActive = time.Now()
		}
	} else {
		endPoint.pendingCall = append(endPoint.pendingCall, &rpcCall{
			arg:      arg,
			cb:       cb,
			deadline: time.Now().Add(time.Duration(timeout) * time.Millisecond),
			to:       peer,
		})
		//logger.Infoln("add pendingCall", len(endPoint.pendingCall), endPoint)
		//尝试与对端建立连接
		this.dial(endPoint, 0)
	}
}

func onMissingRPCMethod(_ string, replyer *rpc.RPCReplyer) {
	replyer.Reply(nil, rpcerr.Err_RPC_InvaildMethod)
}
