package cluster

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/rpc"
	//"github.com/sniperHW/kendynet/timer"
	"github.com/sniperHW/kendynet/util"
	"github.com/sniperHW/sanguo/cluster/addr"
	"github.com/sniperHW/sanguo/cluster/priority"
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

func (this *Cluster) onSessionEvent(end *endPoint, session kendynet.StreamSession, event *kendynet.Event, updateLastActive bool) {
	if updateLastActive {
		end.lastActive = time.Now()
	}

	switch event.Data.(type) {
	case *ss.Message:

		var err error

		msg := event.Data.(*ss.Message)

		from := msg.From()
		if addr.LogicAddr(0) == from {
			from = end.addr.Logic
		}

		data := msg.GetData()
		switch data.(type) {
		case *rpc.RPCRequest:
			err = this.queue.PostFullReturn(priority.LOW, this.rpcMgr.onRPCRequest, end, from, data.(*rpc.RPCRequest))
		case *rpc.RPCResponse:
			//response回调已经被hook必然在queue中调用
			this.rpcMgr.onRPCResponse(data.(*rpc.RPCResponse))
		case proto.Message:
			err = this.queue.PostFullReturn(priority.LOW, this.dispatch, from, session, msg.GetCmd(), data.(proto.Message))
		default:
			logger.Errorf("invaild message type:%s \n", reflect.TypeOf(data).String())
		}

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

func (this *Cluster) onEstablishServer(end *endPoint, session kendynet.StreamSession) error {

	if end.addr.Logic != this.serverState.selfAddr.Logic {
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
		session.SetReceiver(ss.NewReceiver("ss", "rpc_req", "rpc_resp", this.serverState.selfAddr.Logic))
		session.SetEncoder(ss.NewEncoder("ss", "rpc_req", "rpc_resp"))
		session.Start(func(event *kendynet.Event) {
			if event.EventType == kendynet.EventTypeError {
				event.Session.Close(event.Data.(error).Error(), 0)
			} else {
				this.onSessionEvent(end, session, event, false)
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
		this.queue.PostNoWait(priority.MID, this.onPeerDisconnected, end.addr.Logic, fmt.Errorf(reason))
		this.rpcMgr.onEndDisconnected(end)
	})

	end.session = session

	session.Start(func(event *kendynet.Event) {
		if event.EventType == kendynet.EventTypeError {
			end.closeSession(event.Data.(error).Error())
		} else {
			this.onSessionEvent(end, session, event, true)
		}
	})

	now := time.Now()

	end.lastActive = now

	//自连接不需要建立timer
	if end.addr.Logic != this.serverState.selfAddr.Logic {
		end.timer = this.RegisterTimer(common.HeartBeat_Timeout/2, end.onTimerTimeout, nil)
	}

	pendingMsg := end.pendingMsg
	end.pendingMsg = []interface{}{}
	for _, v := range pendingMsg {
		session.Send(v)
	}

	pendingCall := end.pendingCall
	end.pendingCall = []*rpcCall{}

	for _, v := range pendingCall {
		if v.dialTimer.Cancel() {
			remain := v.deadline.Sub(now)
			err := this.rpcMgr.client.AsynCall(&RPCChannel{
				to:      v.to,
				peer:    end,
				cluster: this,
			}, "rpc", v.arg, remain, v.cb)

			if nil != err {
				v.cb(nil, err)
			}
		}
	}
}
