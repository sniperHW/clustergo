package cluster

import(
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/rpc"
	"github.com/sniperHW/kendynet/socket/stream_socket/tcp"
	"sanguo/codec/ss"
	"sanguo/common"
	"time"
	"fmt"
)


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
			queue.Post(func(){
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
				queue.Post(func () {
					dialRet(end,nil,err)
				})
			} else {
		    	session,err := client.Dial(time.Second * 3)
				queue.Post(func () {
					dialRet(end,session,err)
				})
		    }
		}()
	}
}