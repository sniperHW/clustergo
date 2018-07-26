package cluster

import (
	"github.com/sniperHW/kendynet/rpc"
	"github.com/sniperHW/kendynet"
	"github.com/golang/protobuf/proto"
	"reflect"
	"fmt"
)




var rpcServer *rpc.RPCServer

type rpcChannel struct {
	session    kendynet.StreamSession
	name       string
}

func newRPCChannel(sess kendynet.StreamSession,name string) *rpcChannel {
	return &rpcChannel{session:sess,name:name}
}

func(this *rpcChannel) SendRequest(message interface {}) error {
	return this.session.Send(message)
}

func(this *rpcChannel) SendResponse(message interface {}) error {
	return this.session.Send(message)
}

func(this *rpcChannel) Name() string {
	return this.name
}

func(this *rpcChannel)	GetSession() kendynet.StreamSession {
	return this.session
}

type encoder struct {

}

func (this *encoder) Encode(message rpc.RPCMessage) (interface{},error) {
	return message,nil	
}

type decoder struct {

}

func (this *decoder) Decode(o interface{}) (rpc.RPCMessage,error) {	
	return o.(rpc.RPCMessage),nil	
}

/*
*  注册RPC服务,无锁保护，务必在初始化时完成
*/
func RegisterMethod(arg proto.Message,handler rpc.RPCMethodHandler) {
	rpcServer.RegisterMethod(reflect.TypeOf(arg).String(),handler)
}


/*
*  同步RPC调用
*/

func SyncCall(peer PeerID,arg proto.Message,timeout uint32) (ret interface{},err error) {
	respChan := make(chan struct{})
	AsynCall(peer,arg,timeout,func (ret_ interface{},err_ error) {
		ret = ret_
		err = err_
		respChan <- struct{}{}
	})
	_ = <- respChan
	return
}

/*
*  异步RPC调用
*/
func AsynCall(peer PeerID,arg proto.Message,timeout uint32,cb rpc.RPCResponseHandler) {

	if started == 0 {
		PostTask(func() {
			cb(nil,fmt.Errorf("cluster not started"))
		})
		return
	} 

	PostTask(func () {
		endPoint := getEndPointByID(peer)
		if nil != endPoint {
			if nil != endPoint.conn {
				err := endPoint.conn.rpcCli.AsynCall("call",arg,timeout,cb) //methord参数在这里没有被使用，随便填一个值
				if nil != err {
					//记录日志
					Errorf("Call %s:%s error:%s\n",peer.ToString(),reflect.TypeOf(arg).String(),err.Error())
					cb(nil,err)
				}
			} else {

				call := &rpcCall{
					arg     : arg,
					timeout : timeout,
					cb      : cb,
				}		

				endPoint.pendingCall = append(endPoint.pendingCall,call)
				//尝试与对端建立连接
				dial(endPoint)
				
			}
		} else {
			//记录日志
			Errorf("Call %s not found",peer.ToString())
			cb(nil,fmt.Errorf("%s not found",peer.ToString()))
		}
	}) 	
}


