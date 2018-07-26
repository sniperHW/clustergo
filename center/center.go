package main

import (
	"sanguo/codec/ss"
	"sanguo/center/protocol"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/util"
	"github.com/sniperHW/kendynet/socket/stream_socket/tcp"
	"os"
	"fmt"
	"time"
	"reflect"
	"github.com/sniperHW/kendynet/golog"	
	"sanguo/common"
)

type MsgHandler func (kendynet.StreamSession,proto.Message)

type node struct {
	tt        string
	ip        string 
	port      int32
}

func makeStr(tt,ip string,port int32) string {
	return fmt.Sprintf("%s@%s:%d",tt,ip,port)	
}

func (this node) toStr() string {
	return makeStr(this.tt,this.ip,this.port)
}

type sessionData struct {
	nodeInfo    *node
}

var (
	queue *util.BlockQueue
	handlers map[string]MsgHandler
	sessions map[kendynet.StreamSession]kendynet.StreamSession
)


func brocast(msg proto.Message,exclude *map[kendynet.StreamSession]bool) {
	for _,v := range(sessions) {
		if nil == exclude {
			v.Send(msg)
		} else {
			if _,ok := (*exclude)[v]; !ok {
				v.Send(msg)
			}
		}
	}
}

func onSessionClose(session kendynet.StreamSession,reason string) {
	delete(sessions,session)	
	data := session.GetUserData().(*sessionData)
	if nil != data.nodeInfo {
		kendynet.Infof("node lose %s reason:%s\n",data.nodeInfo.toStr(),reason)
		//向所有节点通告有节点离线
		notify := &protocol.NodeLose{}
		node := &protocol.NodeInfo{}
		node.Tt = proto.String(data.nodeInfo.tt)
		node.Ip = proto.String(data.nodeInfo.ip)
		node.Port = proto.Int32(data.nodeInfo.port)
		notify.Nodes = append(notify.Nodes,node)
		brocast(notify,nil)
	}
}

func onLogin(session kendynet.StreamSession,msg proto.Message) {
	data := session.GetUserData().(*sessionData)
	if nil == data.nodeInfo {
		_Login := msg.(*protocol.Login)
		for _,v := range(sessions) {
			str := makeStr(_Login.GetTt(),_Login.GetIp(),_Login.GetPort())
			d := v.GetUserData().(*sessionData)
			if nil != d.nodeInfo && d.nodeInfo.toStr() == str {
				//重复登录
				LoginFailed := &protocol.LoginFailed{}
				LoginFailed.Msg = proto.String("duplicate node:" + str)
				session.Send(LoginFailed)
				session.Close("duplicate node",1)
				delete(sessions,session)
				//记录日志
				return
			}
		}

		data.nodeInfo = &node{}
		data.nodeInfo.tt   = _Login.GetTt()
		data.nodeInfo.ip   = _Login.GetIp()
		data.nodeInfo.port = _Login.GetPort()

		sessions[session] = session

		kendynet.Infof("node Login %s\n",data.nodeInfo.toStr())

		//记录日志
		notify := &protocol.NotifyNodeInfo{}

		node := &protocol.NodeInfo{}
		node.Tt = proto.String(data.nodeInfo.tt)
		node.Ip = proto.String(data.nodeInfo.ip)
		node.Port = proto.Int32(data.nodeInfo.port)
		notify.Nodes = append(notify.Nodes,node)

		//将新节点的信息通告给除自己以外的其它节点
		exclude := make(map[kendynet.StreamSession]bool)
		exclude[session] = true
		brocast(notify,&exclude)
		notify.Reset()

		for _,v := range(sessions) {
			n := v.GetUserData().(*sessionData)
			if nil != n.nodeInfo {
				node := &protocol.NodeInfo{}
				node.Tt = proto.String(n.nodeInfo.tt)
				node.Ip = proto.String(n.nodeInfo.ip)
				node.Port = proto.Int32(n.nodeInfo.port)
				notify.Nodes = append(notify.Nodes,node)
			}
		}

		//将所有节点信息发给新到节点
		session.Send(notify)

	}
}

func onHeartBeat(session kendynet.StreamSession,msg proto.Message) {
	kendynet.Infof("onHeartBeat\n")
	heartbeat := msg.(*protocol.HeartbeatToCenter)
	resp := &protocol.HeartbeatToNode{}
	resp.TimestampBack = proto.Int64(heartbeat.GetTimestamp())
	resp.Timestamp = proto.Int64(time.Now().UnixNano())
	err := session.Send(resp)
	if nil != err {
		kendynet.Errorf("send error:%s\n",err.Error())
	}
}

func dispatchMsg(session kendynet.StreamSession,msg *ss.Message) {
	if nil != msg {
		name := msg.GetName()
		handler,ok := handlers[name]
		if ok {
			queue.Add(func () {
				handler(session,msg.GetData())
			})
		} else {
			//记录日志
			kendynet.Errorf("unkonw msg:%s\n",name)
		}
	}
}


func tick() {
	/*now := time.Now().Unix()
	var timeout []kendynet.StreamSession 
	for _,v := range(sessions) {
		data := v.GetUserData().(*sessionData)
		if data.heartbeat + 10 < now {
			//心跳超时，需要移除
			timeout = append(timeout,v)
			delete(sessions,v)
		}
	}

	for _,v := range(timeout) {
		v.Close("heartbeat timeout",0)
	}*/
}

func registerHandler(msg proto.Message,handler MsgHandler) {
	msgName := reflect.TypeOf(msg).String()	
	if nil == handler {
		//记录日志
		kendynet.Errorf("Register %s failed: handler is nil\n",msgName)
		return
	}
	_,ok := handlers[msgName]
	if ok {
		//记录日志
		kendynet.Errorf("Register %s failed: duplicate handler\n",msgName)
		return
	}
	handlers[msgName] = handler
}

func main() {

	outLogger := golog.NewOutputLogger("log","center",1024*1024*1000)
	kendynet.InitLogger(outLogger,"center")

	queue = util.NewBlockQueue()
	handlers = make(map[string]MsgHandler)
	sessions = make(map[kendynet.StreamSession]kendynet.StreamSession)

	registerHandler(&protocol.Login{},onLogin)
	registerHandler(&protocol.HeartbeatToCenter{},onHeartBeat)


	service := os.Args[1]

	//启动本地监听
	server,err := tcp.NewListener("tcp4",service)
	if server != nil {
		fmt.Printf("server running on:%s\n",service)

		//启动一个go程向主循环发送tick
		go func () {
			for {
				queue.Add(tick)
				time.Sleep(time.Millisecond * 500)
			}
		}()

		go func() {
			for {
				_,localList := queue.Get()
				for _ , task := range localList {
					task.(func())()
				}
			}
		}()

		err = server.Start(func(session kendynet.StreamSession) {
			session.SetRecvTimeout(common.HeartBeat_Timeout * time.Second)
			session.SetUserData(&sessionData{})
			session.SetReceiver(protocol.NewReceiver())
			session.SetEncoder(protocol.NewEncoder())
			session.SetCloseCallBack(func (sess kendynet.StreamSession, reason string) {
				queue.Add(func () {
					onSessionClose(sess,reason)	
				})
			})
			session.Start(func (event *kendynet.Event) {
				if event.EventType == kendynet.EventTypeError {
					event.Session.Close(event.Data.(error).Error(),0)
				} else {
					queue.Add(func () {
						dispatchMsg(session,event.Data.(*ss.Message))	
					})					
				}
			})
		})
		if nil != err {
			fmt.Printf("center start failed %s\n",err)			
		}
	} else {
		fmt.Printf("center failed %s\n",err)
	}

}

