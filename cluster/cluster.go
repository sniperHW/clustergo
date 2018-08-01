package cluster

import(
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/rpc"
	"github.com/sniperHW/kendynet/socket/stream_socket/tcp"
	"github.com/sniperHW/kendynet/event"
	"github.com/golang/protobuf/proto"
	_ "sanguo/protocol/ss" //触发pb注册
	"sanguo/codec/ss"
	"sanguo/common"
	"time"
	"fmt"
	"sync/atomic"
	"sanguo/codec/pb"
)

//外部不能依赖PeerID的类型
type PeerID string

var selfService Service


var (
	queue        *event.EventQueue
	started       int32
)

func (this PeerID) ToString() string {
	return string(this)
}

func MakePeerID(tt,ip string,port int32) PeerID {
	return MakeService(tt,ip,port).ToPeerID()
}

func makePeerID(str string) PeerID {
	return PeerID(str)
}

//类型@ip:port唯一标识一个端点
type connection struct {
	session    		 kendynet.StreamSession
	rpcCli    		*rpc.RPCClient
	nextHeartbeat    int64
}

type Service struct {
	tt         	  string
	ip         	  string
	port       	  int32
}

func (this Service) ToPeerID() PeerID {
	return PeerID(fmt.Sprintf("%s@%s:%d",this.tt,this.ip,this.port))
}

func MakeService(tt,ip string,port int32) Service {
	return Service{tt:tt,ip:ip,port:port}
}


type rpcCall struct {
	arg 	 interface{}
	timeout  uint32
	cb       rpc.RPCResponseHandler
}

func Brocast(tt string,msg proto.Message) {
	queue.Post(func () {
		if ttmap,ok := ttEndPointMap[tt];ok {
			for _,v := range(ttmap) {
				PostMessage(v.toPeerID(),msg)
			}
		} 
	})
}

/*
*  异步投递
*/
func PostMessage(peer PeerID,msg proto.Message) error {

	if started == 0 {
		return fmt.Errorf("cluster not started")
	} 

	queue.Post(func () {
		endPoint := getEndPointByID(peer)
		if nil != endPoint {
			if nil != endPoint.conn {
				err := endPoint.conn.session.Send(msg)
				if nil != err {
					//记录日志
					Errorf("Send error:%s\n",err.Error())
				}
			} else {
				endPoint.pendingMsg = append(endPoint.pendingMsg,msg)
				//尝试与对端建立连接
				dial(endPoint)
				
			}
		} else {
			//记录日志
			Errorf("PostMessage %s not found",peer.ToString())
		}
	}) 

	return nil
}


func tick() {
	now := time.Now().Unix()	
	for _,v := range(idEndPointMap) {
		if nil != v.conn && now >= v.conn.nextHeartbeat {
			v.conn.nextHeartbeat = now + (common.HeartBeat_Timeout/2)
			heartbeat := &Heartbeat{}
			heartbeat.Timestamp1 = proto.Int64(time.Now().UnixNano())
			v.conn.session.Send(heartbeat)
		}
	}
}

/*
*  启动服务
*/
func Start(center_addr string,def Service) error {

	if !atomic.CompareAndSwapInt32(&started,0,1) {
		return fmt.Errorf("service already started")
	}

	selfService = def

	server,err := tcp.NewListener("tcp4",fmt.Sprintf("%s:%d",def.ip,def.port))
	if server != nil {
		
		connectCenter(center_addr,def)

		go func(){
			err := server.Start(func(session kendynet.StreamSession) {
				go func() {
					peerName,err := auth(session)
					if nil != err {
						session.Close(err.Error(),0)
						return
					}


					addSessionPeerID(session,makePeerID(peerName))

					Infof("new client:%s\n",peerName)

					rpcchan := newRPCChannel(session,peerName)
					session.SetRecvTimeout(common.HeartBeat_Timeout * time.Second)
		    		session.SetReceiver(ss.NewReceiver("ss","rpc_req","rpc_resp"))
		    		session.SetEncoder(ss.NewEncoder("ss","rpc_req","rpc_resp"))
					session.SetCloseCallBack(func (sess kendynet.StreamSession, reason string) {
						remSessionPeerID(session)
					})

					session.Start(func (event *kendynet.Event) {
						if event.EventType == kendynet.EventTypeError {
							event.Session.Close(event.Data.(error).Error(),0)
						} else {
							queue.Post(func() {
								switch event.Data.(type) {
									case *ss.Message:
										//处理普通消息
										dispatchServer(session,event.Data.(*ss.Message))
										break
									case *rpc.RPCRequest:
										rpcServer.OnRPCMessage(rpcchan,event.Data.(*rpc.RPCRequest))
										break
									default:
										Errorf("recv unkonw message\n")
										break
								}
							})					
						}
					})
				}()
			})

			if nil != err {
				Errorf("server.Start() failed:%s\n",err.Error())
			}

		}()

		return nil
	} else {
		return err
	}
}

func GetEventQueue() *event.EventQueue {
	return queue
}

/*
*  将一个闭包投递到队列中执行，args为传递给闭包的参数
*/
func PostTask(function interface{},args ...interface{}) {
	queue.Post(function,args...)
}

func init() {
	handlers        = make(map[string]MsgHandler)
	queue           = event.NewEventQueue()
	idEndPointMap   = make(map[PeerID]*endPoint)
	ttEndPointMap   = make(map[string]ttMap)	
	sessionPeerIDMap = make(map[kendynet.StreamSession]PeerID)	
	center_handlers = make(map[string]MsgHandler)

	pb.Register("ss",&Heartbeat{},1)

	rpcServer,_     = rpc.NewRPCServer(&decoder{},&encoder{})

	centerInit()

	go func () {
		for {
			queue.Post(tick)
			time.Sleep(time.Millisecond * 500)
		}
	}()	

	go func() {
		queue.Run()
	}()

}



