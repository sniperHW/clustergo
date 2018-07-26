package cluster

import(
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/rpc"
	"github.com/sniperHW/kendynet/socket/stream_socket/tcp"
	"github.com/sniperHW/kendynet/util"
	"github.com/golang/protobuf/proto"
	_ "sanguo/protocol/ss" //触发pb注册
	ss_proto "sanguo/protocol/ss/message"
	center_proto "sanguo/center/protocol"
	"sanguo/codec/ss"
	"sanguo/common"
	"time"
	"fmt"
	"sync/atomic"
	"net"
	"io"
	"encoding/binary"
)

//外部不能依赖PeerID的类型
type PeerID string

var selfService Service


var (
	queue        *util.BlockQueue
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


func postTask(task func()) {
	queue.Add(task)
}

func dialError(end *endPoint,session kendynet.StreamSession,err error) {
	//记录日志
	Errorf("%s dial error:%s\n",end.toPeerID().ToString(),err.Error())

	if nil != session {
		session.Close(err.Error(),0)
	}

	end.pendingMsg = end.pendingMsg[0:0]
	go func() {
		pendingCall := end.pendingCall
	  	end.pendingCall = end.pendingCall[0:0]
	  	for _,r := range pendingCall {  
	  		r.cb(nil,err)
	    } 		
	}()
	end.dialing = false
}

func dialOK(end *endPoint,session kendynet.StreamSession) {

	//再次确保endPoint是有效的
	if _,ok := idEndPointMap[end.toPeerID()];!ok {
		//对端此时已经失去跟center的联系
		Infof("dialOK %s but endPoint lose center\n",end.toPeerID().ToString())
		session.Close("endPoint lose center",0)
		return
	}

	if nil != end.conn {
		Infof("endPoint:%s already have conn\n",end.toPeerID().ToString())
		session.Close("invaild session",0)
		return
	}

	Infof("dial %s local:%s ok\n",end.toPeerID().ToString(),session.LocalAddr().String())	
	conn := &connection{session:session}
	rpcchan := newRPCChannel(session,end.toPeerID().ToString())
	conn.rpcCli , _ = rpc.NewClient(rpcchan,&decoder{},&encoder{})
	session.SetRecvTimeout(common.HeartBeat_Timeout * time.Second)
	session.SetReceiver(ss.NewReceiver("ss","rpc_req","rpc_resp"))
	session.SetEncoder(ss.NewEncoder("ss","rpc_req","rpc_resp"))
	session.SetCloseCallBack(func (sess kendynet.StreamSession, reason string) {
		err := fmt.Errorf(reason)
		conn.rpcCli.OnChannelClose(err)
		end.conn = nil
		remSessionPeerID(session)
		Infof("%s disconnected error:%s\n",end.toPeerID().ToString(),reason)
	})
	end.conn = conn
	session.Start(func (event *kendynet.Event) {
		if event.EventType == kendynet.EventTypeError {
			event.Session.Close(event.Data.(error).Error(),0)
		} else {
			postTask(func(){
				switch event.Data.(type) {
					case *ss.Message:
						//处理普通消息
						dispatchClient(session,event.Data.(*ss.Message))
						break
					case *rpc.RPCResponse:
						conn.rpcCli.OnRPCMessage(event.Data.(*rpc.RPCResponse))
						break
					default:
						Errorf("invaild message type\n")
						break
				}
			})
		}
	})

	addSessionPeerID(session,end.toPeerID())
	pendingMsg := end.pendingMsg
	end.pendingMsg = end.pendingMsg[0:0]
	Infof("Flush pending message:%d\n",len(pendingMsg))
	for _ , v := range pendingMsg {
		session.Send(v)
	}	
	pendingCall := end.pendingCall
	end.pendingCall = end.pendingCall[0:0]	 
	for _ , v := range pendingCall {
		err := conn.rpcCli.AsynCall("rpc",v.arg,v.timeout,v.cb)//methord参数在这里没有被使用，随便填一个值
		if nil != err {
			v.cb(nil,err)
		}
	}	

	end.dialing = false
}


func login(end *endPoint,session kendynet.StreamSession) {
	go func() {
		conn := session.GetUnderConn().(*net.TCPConn)
		name := selfService.ToPeerID().ToString()
		buffer := kendynet.NewByteBuffer(len(name)+2)
		buffer.PutUint16(0,uint16(len(name)))
		buffer.PutString(2,name)
		conn.SetWriteDeadline(time.Now().Add(time.Second * common.HeartBeat_Timeout))
		_,err := conn.Write(buffer.Bytes())
		conn.SetWriteDeadline(time.Time{})		
		Infof("login send ok\n")
		if nil != err {
			postTask(func () {
				dialError(end,session,err)
			})
		} else {
			
			buffer := make([]byte,512)
			var err  error	
			var ret  string		

			for {
				conn.SetReadDeadline(time.Now().Add(time.Second * common.HeartBeat_Timeout))
				_,err = io.ReadFull(conn,buffer[:2])
				if nil != err {
					break
				}

				s := binary.BigEndian.Uint16(buffer[:2])
				if s > 510 {
					err = fmt.Errorf("too large")
					break
				}	

				_,err = io.ReadFull(conn,buffer[2:2+s])
				if nil != err {
					break
				}

				ret = string(buffer[2:2+s])

				conn.SetReadDeadline(time.Time{})
				break
			}

			if nil != err {
				postTask(func () {
					dialError(end,session,err)
				})
			} else {
				if ret == "ok" {
					postTask(func () {
						dialOK(end,session)
					})
				} else {
					postTask(func () {
						dialError(end,session,fmt.Errorf(ret))
					})
				}
			}
		}
	}()
	/*
	postTask(func () {
		dialOK(end,session)
	})*/	
}

func dialRet(end *endPoint,session kendynet.StreamSession,err error) {
	if nil != session {
		//连接成功
		login(end,session)
	} else {
		//连接失败
		dialError(end,session,err)		
	}
}

func dial(end *endPoint) {
	//发起异步Dial连接

	if !end.dialing {

		end.dialing = true

		Infof("dial %s\n",end.toPeerID().ToString())

		go func(){
			client,err := tcp.NewConnector("tcp4",fmt.Sprintf("%s:%d",end.ip,end.port))
			if err != nil {
				postTask(func () {
					dialRet(end,nil,err)
				})
			} else {
		    	session,err := client.Dial(time.Second * 3)
				postTask(func () {
					dialRet(end,session,err)
				})
		    }
		}()
	}
}

func Brocast(tt string,msg proto.Message) {
	postTask(func () {
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

	postTask(func () {
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


func auth(session kendynet.StreamSession) (string,error) {
	buffer := make([]byte,512)
	var name string
	var err  error

	conn := session.GetUnderConn().(*net.TCPConn)

	for {
		conn.SetReadDeadline(time.Now().Add(time.Second * common.HeartBeat_Timeout))
		_,err = io.ReadFull(conn,buffer[:2])
		if nil != err {
			break
		}

		s := binary.BigEndian.Uint16(buffer[:2])
		if s > 510 {
			err = fmt.Errorf("too large")
			break
		}	

		_,err = io.ReadFull(conn,buffer[2:2+s])
		if nil != err {
			break
		}

		name = string(buffer[2:2+s])

		conn.SetReadDeadline(time.Time{})

		resp := kendynet.NewByteBuffer(64)
		resp.PutUint16(0,uint16(len("ok")))
		resp.PutString(2,"ok")
		conn.SetWriteDeadline(time.Now().Add(time.Second * common.HeartBeat_Timeout))
		_,err = conn.Write(resp.Bytes())
		conn.SetWriteDeadline(time.Time{})
		break
	}
	return name,err

//	return session.RemoteAddr().String(),nil
}

func tick() {
	now := time.Now().Unix()	
	for _,v := range(idEndPointMap) {
		if nil != v.conn && now >= v.conn.nextHeartbeat {
			v.conn.nextHeartbeat = now + (common.HeartBeat_Timeout/2)
			heartbeat := &ss_proto.Heartbeat{}
			heartbeat.Timestamp1 = proto.Int64(time.Now().UnixNano())
			v.conn.session.Send(heartbeat)
		}
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
							postTask(func() {
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

func init() {
	handlers        = make(map[string]MsgHandler)
	queue           = util.NewBlockQueue()
	idEndPointMap   = make(map[PeerID]*endPoint)
	ttEndPointMap   = make(map[string]ttMap)	
	sessionPeerIDMap = make(map[kendynet.StreamSession]PeerID)	
	center_handlers = make(map[string]MsgHandler)

	rpcServer,_     = rpc.NewRPCServer(&decoder{},&encoder{})


	RegisterCenterMsgHandler(&center_proto.HeartbeatToNode{},func (session kendynet.StreamSession, msg proto.Message) {
		//心跳响应暂时不处理
		//kendynet.Infof("HeartbeatToNode\n")
	})

	RegisterCenterMsgHandler(&center_proto.NotifyNodeInfo{},func (session kendynet.StreamSession, msg proto.Message) {
		postTask(func () {
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
		postTask(func () {
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
		postTask(func () {
			LoginFailed := msg.(*center_proto.LoginFailed)
			Errorf("login center failed:%s",LoginFailed.GetMsg())	
		})		
	})

	go func () {
		for {
			postTask(tick)
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

}



