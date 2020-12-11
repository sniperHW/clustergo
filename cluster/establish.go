package cluster

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/rpc"
	//"github.com/sniperHW/kendynet/timer"
	"github.com/sniperHW/kendynet/util"
	"github.com/sniperHW/sanguo/cluster/addr"
	"github.com/sniperHW/sanguo/codec/ss"
	"github.com/sniperHW/sanguo/common"
	"reflect"
	"time"
)

func (this *Cluster) onEstablishClient(end *endPoint, session kendynet.StreamSession) {

	end.Lock()
	defer end.Unlock()
	end.dialing = false

	if nil != end.session {
		/*
		 * 如果end.conn != nil 表示两端同时请求建立连接，本端已经作为服务端成功接受了对端的连接
		 */
		logger.Infof("endPoint:%s already have connection\n", end.addr.Logic.String())
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

	this.onEstablish(end, session)

}

func (this *Cluster) onEstablishServer(end *endPoint, session kendynet.StreamSession) error {

	if end.addr.Logic != this.serverState.selfAddr.Logic {

		if end != this.serviceMgr.getEndPoint(addr.LogicAddr(end.addr.Logic)) {
			return ERR_INVAILD_ENDPOINT
		}

		end.Lock()
		defer end.Unlock()

		if end.session != nil {
			/*
			 *如果end.session != nil 表示两端同时请求建立连接，本端已经作为客户端成功与对端建立连接
			 */
			return ERR_DUP_CONN
		}

		this.onEstablish(end, session)
	} else {

		//自连接server

		f := func(args []interface{}) {
			msg := args[0].(*ss.Message)
			from := msg.From()
			if addr.LogicAddr(0) == from {
				from = end.addr.Logic
			}

			data := msg.GetData()
			switch data.(type) {
			case *rpc.RPCRequest:
				this.rpcMgr.onRPCRequest(end, from, data.(*rpc.RPCRequest))
			case *rpc.RPCResponse:
				this.rpcMgr.onRPCResponse(data.(*rpc.RPCResponse))
			case proto.Message:
				this.dispatch(from, session, msg.GetCmd(), data.(proto.Message))
			default:
				logger.Errorf("invaild message type:%s \n", reflect.TypeOf(data).String())
			}
		}

		session.SetReceiver(ss.NewReceiver("ss", "rpc_req", "rpc_resp", this.serverState.selfAddr.Logic))
		session.SetEncoder(ss.NewEncoder("ss", "rpc_req", "rpc_resp"))
		session.Start(func(event *kendynet.Event) {
			if event.EventType == kendynet.EventTypeError {
				event.Session.Close(event.Data.(error).Error(), 0)
			} else {
				err := this.queue.PostFullReturn(f, event.Data.(*ss.Message))
				if err == util.ErrQueueFull {
					logger.Errorln("event queue full discard message")
				}
			}
		})
	}

	return nil
}

func (this *Cluster) onEstablish(end *endPoint, session kendynet.StreamSession) {

	logger.Infoln("onEstablish", end, this.serverState.selfAddr.Logic.String(), "<--->", end.addr.Logic.String(), session.LocalAddr(), session.RemoteAddr())

	session.SetReceiver(ss.NewReceiver("ss", "rpc_req", "rpc_resp", this.serverState.selfAddr.Logic))
	session.SetEncoder(ss.NewEncoder("ss", "rpc_req", "rpc_resp"))
	session.SetCloseCallBack(func(sess kendynet.StreamSession, reason string) {
		logger.Infoln("disconnected error:", end.addr.Logic.String(), reason, sess.LocalAddr(), sess.RemoteAddr())
		err := fmt.Errorf(reason)
		this.queue.PostNoWait(func() {
			this.onPeerDisconnected(end.addr.Logic, err)
		})
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
			this.rpcMgr.onRPCRequest(end, from, data.(*rpc.RPCRequest))
		case *rpc.RPCResponse:
			this.rpcMgr.onRPCResponse(data.(*rpc.RPCResponse))
		case proto.Message:
			this.dispatch(from, session, msg.GetCmd(), data.(proto.Message))
		default:
			logger.Errorf("invaild message type:%s \n", reflect.TypeOf(data).String())
		}
	}

	session.Start(func(event *kendynet.Event) {
		if event.EventType == kendynet.EventTypeError {
			end.closeSession(event.Data.(error).Error())
		} else {
			end.lastActive = time.Now()
			switch event.Data.(type) {
			case *ss.Message:
				err := this.queue.PostFullReturn(f, event.Data.(*ss.Message))
				if err == util.ErrQueueFull {
					logger.Errorln("event queue full discard message")
				}
				break
			case *ss.RelayMessage:
				if this.serverState.selfAddr.Logic.Type() == harbarType {
					this.onRelayMessage(event.Data.(*ss.RelayMessage))
				}
				break
			default:
				logger.Errorf("invaild message type\n")
				break
			}
		}
	})

	now := time.Now()

	end.lastActive = now

	//自连接不需要建立timer
	if end.addr.Logic != this.serverState.selfAddr.Logic {
		end.timer = this.RegisterTimer(common.HeartBeat_Timeout*time.Second/2, end.onTimerTimeout, nil)
	}

	pendingMsg := end.pendingMsg
	end.pendingMsg = []interface{}{}
	for _, v := range pendingMsg {
		session.Send(v)
	}

	pendingCall := end.pendingCall
	end.pendingCall = []*rpcCall{}

	for _, v := range pendingCall {

		if now.After(v.deadline) {
			//连接消费的事件已经超过请求的超时时间
			this.queue.PostNoWait(func() { v.cb(nil, rpc.ErrCallTimeout) })
		} else {
			remain := v.deadline.Sub(now)
			if remain <= time.Millisecond {
				//剩余1毫秒,几乎肯定超时，没有调用的必要
				this.queue.PostNoWait(func() { v.cb(nil, rpc.ErrCallTimeout) })
			} else {
				err := this.rpcMgr.client.AsynCall(&RPCChannel{
					to:      v.to,
					peer:    end,
					cluster: this,
				}, "rpc", v.arg, remain, v.cb)

				if nil != err {
					this.queue.PostNoWait(func() { v.cb(nil, err) })
				}
			}
		}
	}

}
