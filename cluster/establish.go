package cluster

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/rpc"
	"github.com/sniperHW/sanguo/cluster/addr"
	"github.com/sniperHW/sanguo/codec/ss"
	"github.com/sniperHW/sanguo/common"
	"time"
)

func onEstablishClient(end *endPoint, session kendynet.StreamSession) {
	if nil != end.conn {
		/*
		 * 如果end.conn != nil 表示两端同时请求建立连接，本端已经作为服务端成功接受了对端的连接
		 */
		Infof("endPoint:%s already have conn\n", end.addr.Logic.String())
		session.Close("duplicate endPoint connection", 0)
		return
	}
	onEstablish(end, session, true)

}

func onEstablishServer(peer addr.LogicAddr, session kendynet.StreamSession) error {

	if peer != selfAddr.Logic {

		end := getEndPoint(peer)
		if nil == end {
			return fmt.Errorf("invaild endPoint")
		}

		end.mtx.Lock()
		defer end.mtx.Unlock()

		if end.conn != nil {

			/*
			 *如果end.conn != nil 表示两端同时请求建立连接，本端已经作为客户端成功与对端建立连接
			 */

			return fmt.Errorf("duplicate endPoint connection")
		}

		onEstablish(end, session, false)

	} else {

		f := func(args []interface{}) {
			event := args[0].(*kendynet.Event)
			switch event.Data.(type) {
			case *ss.Message:
				msg := event.Data.(*ss.Message)

				from := msg.From()
				Debugln("msg from", from.String())
				if addr.LogicAddr(0) == from {
					from = peer
				}
				if msg.GetName() == "rpc" {
					onRPCMessage(getEndPoint(selfAddr.Logic), from, msg.GetData())
				} else {
					dispatchServer(from, session, msg.GetName(), msg.GetData().(proto.Message))
				}
				break
			default:
				Errorf("invaild message type\n")
				break
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
				queue.PostNoWait(f, event)
			}
		})
	}

	return nil
}

func onEstablish(end *endPoint, session kendynet.StreamSession, isClient bool) {

	conn := &connection{
		session: session,
	}

	var onTimeout func()

	onTimeout = func() {
		end.mtx.Lock()
		defer end.mtx.Unlock()
		if end.conn != nil {
			heartbeat := &Heartbeat{}
			heartbeat.Timestamp1 = proto.Int64(time.Now().UnixNano())
			session.Send(heartbeat)
			conn.timer = RegisterTimer(time.Now().Add(time.Second*(common.HeartBeat_Timeout/2)), onTimeout)
		}
	}

	if isClient {
		conn.timer = RegisterTimer(time.Now().Add(time.Second*(common.HeartBeat_Timeout/2)), onTimeout)
	}

	session.SetRecvTimeout(common.HeartBeat_Timeout * time.Second)
	session.SetReceiver(ss.NewReceiver("ss", "rpc_req", "rpc_resp", selfAddr.Logic))
	session.SetEncoder(ss.NewEncoder("ss", "rpc_req", "rpc_resp"))
	session.SetCloseCallBack(func(sess kendynet.StreamSession, reason string) {
		end.mtx.Lock()
		defer end.mtx.Unlock()
		if nil != conn.timer {
			UnregisterTimer(conn.timer)
		}
		err := fmt.Errorf(reason)
		end.conn = nil
		Infof("%s disconnected error:%s\n", end.addr.Logic.String(), reason)
		queue.PostNoWait(func() { dispatchPeerDisconnected(end.addr.Logic, err) })
	})
	end.conn = conn

	f := func(args []interface{}) {

		msg := args[0].(*ss.Message)

		from := msg.From()

		if addr.LogicAddr(0) == from {
			from = end.addr.Logic
		}

		if msg.GetName() == "rpc" {
			onRPCMessage(end, from, msg.GetData())
		} else {
			if isClient {
				dispatchClient(from, session, msg.GetName(), msg.GetData().(proto.Message))
			} else {
				dispatchServer(from, session, msg.GetName(), msg.GetData().(proto.Message))
			}
		}

	}

	session.Start(func(event *kendynet.Event) {
		if event.EventType == kendynet.EventTypeError {
			event.Session.Close(event.Data.(error).Error(), 0)
		} else {
			switch event.Data.(type) {
			case *ss.Message:
				queue.PostNoWait(f, event.Data.(*ss.Message))
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
	Infof("Flush pending message:%d\n", len(pendingMsg))
	for _, v := range pendingMsg {
		session.Send(v)
	}
	pendingCall := end.pendingCall
	end.pendingCall = []*rpcCall{}

	now := time.Now()

	Infof("Flush pending call:%d\n", len(pendingCall))

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
	Infoln("onEstablish", end.addr.Logic.String(), end.conn)
}
