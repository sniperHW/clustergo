package center

import (
	//"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/rpc"
	"reflect"
	"time"
)

var rpcServer *rpc.RPCServer
var rpcClient *rpc.RPCClient

var timeout = time.Second * 8 // 超时

type RPCChannel struct {
	session kendynet.StreamSession
}

func (this *RPCChannel) SendRequest(message interface{}) error {
	return this.session.Send(message)
}

func (this *RPCChannel) SendResponse(message interface{}) error {
	return this.session.Send(message)
}

func (this *RPCChannel) Name() string {
	return "Session"
}

type encoder struct {
}

func (this *encoder) Encode(message rpc.RPCMessage) (interface{}, error) {
	return message, nil
}

type decoder struct {
}

func (this *decoder) Decode(o interface{}) (rpc.RPCMessage, error) {
	return o.(rpc.RPCMessage), nil
}

/*
 *  注册RPC服务,无锁保护，务必在初始化时完成
 */
func registerMethod(arg proto.Message, handler rpc.RPCMethodHandler) {
	rpcServer.RegisterMethod(reflect.TypeOf(arg).String(), handler)
}

func asynCall(ses kendynet.StreamSession, arg proto.Message, cb rpc.RPCResponseHandler) error {
	return rpcClient.AsynCall(&RPCChannel{session: ses}, reflect.TypeOf(arg).String(), arg, timeout, cb)
}

func onRPCRequest(ses kendynet.StreamSession, msg interface{}) {
	rpcServer.OnRPCMessage(&RPCChannel{session: ses}, msg)
}

func onRPCResponse(msg interface{}) {
	rpcClient.OnRPCMessage(msg)
}

func init() {
	rpcServer = rpc.NewRPCServer(&decoder{}, &encoder{})
	rpcClient = rpc.NewClient(&decoder{}, &encoder{})
}
