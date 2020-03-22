package cluster

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/rpc"
	//"github.com/sniperHW/kendynet/timer"
	"github.com/sniperHW/kendynet/util"
	center_proto "github.com/sniperHW/sanguo/center/protocol"
	"github.com/sniperHW/sanguo/cluster/addr"
	"github.com/sniperHW/sanguo/codec/ss"
	"github.com/sniperHW/sanguo/common"
	"reflect"
	"time"
)

func onEstablishClient(end *endPoint, session kendynet.StreamSession) {
	if nil != end.session {
		/*
		 * 如果end.conn != nil 表示两端同时请求建立连接，本端已经作为服务端成功接受了对端的连接
		 */
		Infof("endPoint:%s already have connection\n", end.addr.Logic.String())
		session.Close("duplicate endPoint connection", 0)
		return
	}

	//不主动触发心跳，超时回收连接
	/*
		RegisterTimer(time.Second*(common.HeartBeat_Timeout/2), func(t *timer.Timer, _ interface{}) {
			heartbeat := &Heartbeat{}
			heartbeat.Timestamp1 = proto.Int64(time.Now().UnixNano())
			heartbeat.OriginSender = proto.Uint32(uint32(selfAddr.Logic))
			if kendynet.ErrSocketClose == session.Send(heartbeat) {
				t.Cancel()
			}
		}, nil)
	*/

	onEstablish(end, session)

}

func onEstablishServer(nodeInfo *center_proto.NodeInfo, session kendynet.StreamSession) error {

	if nodeInfo.GetLogicAddr() != uint32(selfAddr.Logic) {

		end := getEndPoint(addr.LogicAddr(nodeInfo.GetLogicAddr()))

		if nil == end {
			return ERR_INVAILD_ENDPOINT
		}

		end.mtx.Lock()
		defer end.mtx.Unlock()

		if end.session != nil {
			/*
			 *如果end.session != nil 表示两端同时请求建立连接，本端已经作为客户端成功与对端建立连接
			 */
			return ERR_DUP_CONN
		}

		onEstablish(end, session)

	} else {

		//自连接

		f := func(args []interface{}) {
			msg := args[0].(*ss.Message)
			from := msg.From()
			if addr.LogicAddr(0) == from {
				from = addr.LogicAddr(nodeInfo.GetLogicAddr())
			}

			data := msg.GetData()
			switch data.(type) {
			case *rpc.RPCRequest:
				onRPCRequest(getEndPoint(selfAddr.Logic), from, data.(*rpc.RPCRequest))
			case *rpc.RPCResponse:
				onRPCResponse(data.(*rpc.RPCResponse))
			case proto.Message:
				dispatch(from, session, msg.GetCmd(), data.(proto.Message))
			default:
				Errorf("invaild message type:%s \n", reflect.TypeOf(data).String())
			}
		}

		//自连接不需要建立endpoint
		session.SetRecvTimeout(common.HeartBeat_Timeout * time.Second)
		session.SetReceiver(ss.NewReceiver("ss", "rpc_req", "rpc_resp", selfAddr.Logic))
		session.SetEncoder(ss.NewEncoder("ss", "rpc_req", "rpc_resp"))
		session.Start(func(event *kendynet.Event) {
			if event.EventType == kendynet.EventTypeError {
				event.Session.Close(event.Data.(error).Error(), 0)
			} else {
				err := queue.PostFullReturn(f, event.Data.(*ss.Message))
				if err == util.ErrQueueFull {
					Errorln("event queue full discard message")
				}
			}
		})
	}

	return nil
}

func onEstablish(end *endPoint, session kendynet.StreamSession) {

	Infoln("onEstablish", end.addr.Logic.String(), session.LocalAddr(), session.RemoteAddr())

	session.SetRecvTimeout(common.HeartBeat_Timeout * time.Second)
	session.SetReceiver(ss.NewReceiver("ss", "rpc_req", "rpc_resp", selfAddr.Logic))
	session.SetEncoder(ss.NewEncoder("ss", "rpc_req", "rpc_resp"))
	session.SetCloseCallBack(func(sess kendynet.StreamSession, reason string) {
		end.mtx.Lock()
		defer end.mtx.Unlock()
		end.session = nil
		Infoln("disconnected error:", end.addr.Logic.String(), reason, sess.LocalAddr(), sess.RemoteAddr())
		err := fmt.Errorf(reason)
		queue.PostNoWait(func() { dispatchPeerDisconnected(end.addr.Logic, err) })
	})
	end.session = session

	f := func(args []interface{}) {
		msg := args[0].(*ss.Message)
		from := msg.From()
		if addr.LogicAddr(0) == from {
			from = end.addr.Logic
		}

		data := msg.GetData()
		switch data.(type) {
		case *rpc.RPCRequest:
			onRPCRequest(end, from, data.(*rpc.RPCRequest))
		case *rpc.RPCResponse:
			onRPCResponse(data.(*rpc.RPCResponse))
		case proto.Message:
			dispatch(from, session, msg.GetCmd(), data.(proto.Message))
		default:
			Errorf("invaild message type:%s \n", reflect.TypeOf(data).String())
		}
	}

	session.Start(func(event *kendynet.Event) {
		if event.EventType == kendynet.EventTypeError {
			event.Session.Close(event.Data.(error).Error(), 0)
		} else {
			switch event.Data.(type) {
			case *ss.Message:
				err := queue.PostFullReturn(f, event.Data.(*ss.Message))
				if err == util.ErrQueueFull {
					Errorln("event queue full discard message")
				}
				break
			case *ss.RelayMessage:
				if selfAddr.Logic.Type() == harbarType {
					onRelayMessage(event.Data.(*ss.RelayMessage))
				}
				break
			default:
				Errorf("invaild message type\n")
				break
			}
		}
	})

	pendingMsg := end.pendingMsg
	end.pendingMsg = []interface{}{}
	for _, v := range pendingMsg {
		session.Send(v)
	}
	pendingCall := end.pendingCall
	end.pendingCall = []*rpcCall{}

	now := time.Now()

	for _, v := range pendingCall {
		if now.After(v.deadline) {
			//连接消费的事件已经超过请求的超时时间
			queue.PostNoWait(func() { v.cb(nil, rpc.ErrCallTimeout) })
		} else {
			remain := v.deadline.Sub(now)
			if remain <= time.Millisecond {
				//剩余1毫秒,几乎肯定超时，没有调用的必要
				queue.PostNoWait(func() { v.cb(nil, rpc.ErrCallTimeout) })
			} else {
				err := rpcClient.AsynCall(&RPCChannel{
					to:   v.to,
					peer: end,
				}, "rpc", v.arg, remain, v.cb)
				if nil != err {
					queue.PostNoWait(func() { v.cb(nil, err) })
				}
			}
		}
	}

}
