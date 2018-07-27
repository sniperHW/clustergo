package cluster

import(
	"github.com/golang/protobuf/proto"
	"sanguo/codec/ss"
	"github.com/sniperHW/kendynet"
	"runtime"
	"reflect"
	"time"
	"sync"
)

type MsgHandler func (kendynet.StreamSession,proto.Message)

var mtxHandler sync.Mutex
var handlers map[string]MsgHandler

func Register(msg proto.Message,handler MsgHandler) {
	defer mtxHandler.Unlock()
	mtxHandler.Lock()

	msgName := reflect.TypeOf(msg).String()	
	if nil == handler {
		//记录日志
		Errorf("Register %s failed: handler is nil\n",msgName)
		return
	}
	_,ok := handlers[msgName]
	if ok {
		//记录日志
		Errorf("Register %s failed: duplicate handler\n",msgName)
		return
	}

	handlers[msgName] = handler
}


func pcall(handler MsgHandler,name string,session kendynet.StreamSession,msg proto.Message) {
	defer func(){
		if r := recover(); r != nil {
			buf := make([]byte, 65535)
			l := runtime.Stack(buf, false)
			Errorf("error on Dispatch:%s\nstack:%v,%s\n",name,r,buf[:l])
		}		 
	}()
	handler(session,msg)
}

func dispatch(session kendynet.StreamSession,msg *ss.Message) {
	if nil != msg {
		name := msg.GetName()
		mtxHandler.Lock()
		handler,ok := handlers[name]
		mtxHandler.Unlock()
		if ok {
			pcall(handler,name,session,msg.GetData())
		} else {
			//记录日志
			Errorf("unkonw msg:%s\n",name)
		}
	}
}


func dispatchServer(session kendynet.StreamSession,msg *ss.Message) {
	if nil != msg {
		switch msg.GetData().(type) {
			case *Heartbeat:
				heartbeat := msg.GetData().(*Heartbeat)
				heartbeat_resp := &Heartbeat{}
				heartbeat_resp.Timestamp1 = proto.Int64(time.Now().UnixNano())
				heartbeat_resp.Timestamp2 = proto.Int64(heartbeat.GetTimestamp1())
				session.Send(heartbeat_resp)				
				break
			default:
				dispatch(session,msg)
				break
		}
	}
}


func dispatchClient(session kendynet.StreamSession,msg *ss.Message) {
	if nil != msg {
		switch msg.GetData().(type) {
			case *Heartbeat:			
				break
			default:
				dispatch(session,msg)
				break
		}		
	}	
}
