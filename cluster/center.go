package cluster

import (
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	connector "github.com/sniperHW/kendynet/socket/connector/tcp"
	"reflect"
	center_proto "sanguo/center/protocol"
	"sanguo/cluster/addr"
	"sanguo/codec/ss"
	"sanguo/common"
	"time"
)

type centerHandler func(kendynet.StreamSession, proto.Message)

var centerHandlers map[string]centerHandler = map[string]centerHandler{}

func RegisterCenterMsgHandler(msg proto.Message, handler centerHandler) {
	msgName := reflect.TypeOf(msg).String()
	if nil == handler {
		//记录日志
		Errorf("Register %s failed: handler is nil\n", msgName)
		return
	}

	_, ok := centerHandlers[msgName]
	if ok {
		//记录日志
		Errorf("Register %s failed: duplicate handler\n", msgName)
		return
	}

	centerHandlers[msgName] = handler
}

func dispatchCenterMsg(args []interface{}) {
	session := args[0].(kendynet.StreamSession)
	msg := args[1].(*ss.Message)
	name := msg.GetName()
	handler, ok := centerHandlers[name]
	if ok {
		handler(session, msg.GetData().(proto.Message))
	} else {
		//记录日志
		Errorf("unknow msg:%s\n", name)
	}
}

type center struct {
	addr     string
	selfAddr addr.Addr
}

func (this *center) connect() {
	c, err := connector.New("tcp4", this.addr)
	if nil == err {
		go func() {
			for {
				session, err := c.Dial(time.Second * 3)
				Infof("connect to center:%s\n", this.addr)
				if err != nil {
					time.Sleep(time.Millisecond * 1000)
				} else {
					session.SetReceiver(center_proto.NewReceiver())
					session.SetEncoder(center_proto.NewEncoder())

					done := make(chan struct{}, 1)

					session.SetCloseCallBack(func(sess kendynet.StreamSession, reason string) {
						Infof("center disconnected %s self:%s\n", reason, this.selfAddr.Logic.String())
						done <- struct{}{}
						this.connect()
					})
					session.Start(func(event *kendynet.Event) {
						if event.EventType == kendynet.EventTypeError {
							event.Session.Close(event.Data.(error).Error(), 0)
						} else {
							msg := event.Data.(*ss.Message)
							queue.PostNoWait(dispatchCenterMsg, session, msg)
						}
					})
					//发送login
					session.Send(&center_proto.Login{
						LogicAddr: proto.Uint32(uint32(this.selfAddr.Logic)),
						NetAddr:   proto.String(this.selfAddr.Net.String()),
					})

					ticker := time.NewTicker(time.Second * (common.HeartBeat_Timeout / 2))
					for {
						select {
						case <-done:
							ticker.Stop()
							return
						case <-ticker.C:
							//发送心跳
							session.Send(&center_proto.HeartbeatToCenter{
								Timestamp: proto.Int64(time.Now().UnixNano()),
							})
						}
					}
				}
			}
		}()
	} else {
		Errorf("NewConnector failed:%s\n", err.Error())
	}
}

func connectCenter(centerAddrs []string, selfAddr addr.Addr) {
	centers := map[string]bool{}
	for _, v := range centerAddrs {
		if _, ok := centers[v]; !ok {
			centers[v] = true
			c := &center{
				addr:     v,
				selfAddr: selfAddr,
			}
			c.connect()
		}
	}
}

func centerInit() {
	RegisterCenterMsgHandler(&center_proto.HeartbeatToNode{}, func(session kendynet.StreamSession, msg proto.Message) {
		//心跳响应暂时不处理
		//kendynet.Infof("HeartbeatToNode\n")
	})

	RegisterCenterMsgHandler(&center_proto.NotifyNodeInfo{}, func(session kendynet.StreamSession, msg proto.Message) {
		kendynet.Debugln("NotifyNodeInfo")
		NotifyNodeInfo := msg.(*center_proto.NotifyNodeInfo)
		for _, v := range NotifyNodeInfo.Nodes {
			addEndPoint(v)
		}
	})

	RegisterCenterMsgHandler(&center_proto.NodeLeave{}, func(session kendynet.StreamSession, msg proto.Message) {
		NodeLeave := msg.(*center_proto.NodeLeave)
		for _, v := range NodeLeave.Nodes {
			removeEndPoint(addr.LogicAddr(v))
		}
	})

	RegisterCenterMsgHandler(&center_proto.LoginFailed{}, func(session kendynet.StreamSession, msg proto.Message) {
		LoginFailed := msg.(*center_proto.LoginFailed)
		Errorf("login center failed:%s", LoginFailed.GetMsg())
	})
}
