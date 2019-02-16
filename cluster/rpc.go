package cluster

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet/rpc"
	"github.com/sniperHW/sanguo/cluster/addr"
	"github.com/sniperHW/sanguo/codec/ss"
	"reflect"
	"sync/atomic"
	"time"
)

var rpcServer *rpc.RPCServer
var rpcClient *rpc.RPCClient

type RPCChannel struct {
	to   addr.LogicAddr
	peer *endPoint
}

func (this *RPCChannel) SendRequest(message interface{}) error {
	if this.peer.conn != nil {
		if this.to != this.peer.addr.Logic {
			return this.peer.conn.session.Send(ss.NewMessage("rpc", message, this.to, selfAddr.Logic))
		} else {
			return this.peer.conn.session.Send(message)
		}
	} else {
		return fmt.Errorf("no connection")
	}
}

func (this *RPCChannel) SendResponse(message interface{}) error {
	if this.peer.conn != nil {
		if this.to != this.peer.addr.Logic {
			return this.peer.conn.session.Send(ss.NewMessage("rpc", message, this.to, selfAddr.Logic))
		} else {
			return this.peer.conn.session.Send(message)
		}
	} else {
		return fmt.Errorf("no connection")
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

func onRPCMessage(peer *endPoint, from addr.LogicAddr, msg interface{}) {
	switch msg.(type) {
	case *rpc.RPCRequest:
		rpcchan := &RPCChannel{
			peer: peer,
			to:   from,
		}
		rpcServer.OnRPCMessage(rpcchan, msg.(*rpc.RPCRequest))
		break
	case *rpc.RPCResponse:
		rpcClient.OnRPCMessage(msg.(*rpc.RPCResponse))
		break
	default:
		Errorf("invaild message type\n")
		break
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

	if nil != endPoint.conn {
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
