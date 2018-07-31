package cluster

import (
	"github.com/sniperHW/kendynet"
	"github.com/golang/protobuf/proto"
	"sanguo/codec/ss"
	"sanguo/common"
	center_proto "sanguo/center/protocol"
	"github.com/sniperHW/kendynet/socket/stream_socket/tcp"
	"reflect"
	"sync/atomic"
	"time"
)

var (
	center_handlers map[string]MsgHandler
)

func RegisterCenterMsgHandler(msg proto.Message,handler MsgHandler) {
	msgName := reflect.TypeOf(msg).String()		
	if nil == handler {
		//记录日志
		Errorf("Register %s failed: handler is nil\n",msgName)
		return
	}

	_,ok := center_handlers[msgName]
	if ok {
		//记录日志
		Errorf("Register %s failed: duplicate handler\n",msgName)
		return
	}

	center_handlers[msgName] = handler
}

func dispatchCenterMsg(session kendynet.StreamSession,msg *ss.Message) {
	if nil != msg {
		name := msg.GetName()
		handler,ok := center_handlers[name]
		if ok {
			pcall(handler,name,session,msg.GetData())
		} else {
			//记录日志
			Errorf("unknow msg:%s\n",name)
		}
	}
}

func connectCenter(addr string,self Service) {
	connector,err := tcp.NewConnector("tcp4",addr)
	if nil == err {
		go func () {
			for {
				session,err := connector.Dial(time.Second * 3)
				Errorf("connect\n")
				if err  != nil {
					time.Sleep(time.Millisecond * 1000)
				} else {
					stoped := int32(0)
					session.SetReceiver(center_proto.NewReceiver())
					session.SetEncoder(center_proto.NewEncoder())
					session.SetCloseCallBack(func (sess kendynet.StreamSession, reason string) {
						Infof("center disconnected %s self:%s\n",reason,self.ToPeerID().ToString())
						queue.Post(onCenterLose)
						connectCenter(addr,self)
						atomic.StoreInt32(&stoped,1)
					})
					session.Start(func (event *kendynet.Event) {
						if event.EventType == kendynet.EventTypeError {
							Errorf("disconnected\n")
							event.Session.Close(event.Data.(error).Error(),0)
						} else {
							queue.Post(func (){
								dispatchCenterMsg(session,event.Data.(*ss.Message))
							})
						}
					})
					//发送login
					login := &center_proto.Login{}
					login.Tt = proto.String(self.tt)
					login.Ip = proto.String(self.ip)
					login.Port = proto.Int32(self.port)
					session.Send(login)

					for {
						if 0 == atomic.LoadInt32(&stoped) {
							//发送心跳
							heartbeat := &center_proto.HeartbeatToCenter{}
							heartbeat.Timestamp = proto.Int64(time.Now().UnixNano())
							session.Send(heartbeat)
						}
						time.Sleep(time.Second * (common.HeartBeat_Timeout/2))
					}
					return
				}
			}			
		}()
	} else {
		Errorf("NewConnector failed:%s\n",err.Error())
	}
}

func onCenterLose() {
	for _,v := range(idEndPointMap) {
		if nil != v.conn {
			v.conn.session.Close("lose center",0)
		}
	}
	idEndPointMap = make(map[PeerID]*endPoint)	
	ttEndPointMap = make(map[string]ttMap)
}

func centerInit() {
	RegisterCenterMsgHandler(&center_proto.HeartbeatToNode{},func (session kendynet.StreamSession, msg proto.Message) {
		//心跳响应暂时不处理
		//kendynet.Infof("HeartbeatToNode\n")
	})

	RegisterCenterMsgHandler(&center_proto.NotifyNodeInfo{},func (session kendynet.StreamSession, msg proto.Message) {
		queue.Post(func () {
			NotifyNodeInfo := msg.(*center_proto.NotifyNodeInfo)
			Infof("process NotifyNodeInfo %d\n",len(NotifyNodeInfo.Nodes))	
			for _,v := range(NotifyNodeInfo.Nodes) {
				addEndPoint(&endPoint{
					tt   : v.GetTt(),
					ip   : v.GetIp(),
					port : v.GetPort(),
				})
			}
		})
	})


	RegisterCenterMsgHandler(&center_proto.NodeLose{},func (session kendynet.StreamSession, msg proto.Message) {
		queue.Post(func () {
			NodeLose := msg.(*center_proto.NodeLose)
			for _,v := range(NodeLose.Nodes) {
				s := Service{
					tt : v.GetTt(),
					ip : v.GetIp(),
					port : v.GetPort(),
				}
				remEndPoint(s.ToPeerID())
			}
		})		
	})

	RegisterCenterMsgHandler(&center_proto.LoginFailed{},func (session kendynet.StreamSession, msg proto.Message) {
		queue.Post(func () {
			LoginFailed := msg.(*center_proto.LoginFailed)
			Errorf("login center failed:%s",LoginFailed.GetMsg())	
		})		
	})	
}