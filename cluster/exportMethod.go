package cluster

import (
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet/event"
	"github.com/sniperHW/kendynet/rpc"
	"github.com/sniperHW/kendynet/timer"
	"github.com/sniperHW/sanguo/cluster/addr"
	"time"
)

func Stop(stopFunc func(), sendRemoveNode ...bool) {
	defaultCluster.Stop(stopFunc, sendRemoveNode...)
}

func IsStoped() bool {
	return defaultCluster.IsStoped()
}

func Brocast(tt uint32, msg proto.Message, exceptSelf ...bool) {
	defaultCluster.Brocast(tt, msg, exceptSelf...)
}

func BrocastToAll(msg proto.Message, exceptTT ...uint32) {
	defaultCluster.BrocastToAll(msg, exceptTT...)
}

func PostMessage(peer addr.LogicAddr, msg proto.Message) {
	defaultCluster.PostMessage(peer, msg)
}

func SelfAddr() addr.Addr {
	return defaultCluster.SelfAddr()
}

/*
*  启动服务
 */
func Start(center_addr []string, selfAddr addr.Addr, export ...bool) error {
	return defaultCluster.Start(center_addr, selfAddr, export...)
}

func GetEventQueue() *event.EventQueue {
	return defaultCluster.GetEventQueue()
}

/*
*  将一个闭包投递到队列中执行，args为传递给闭包的参数
 */
func PostTask(function interface{}, args ...interface{}) {
	defaultCluster.PostTask(function, args...)
}

func Mod(tt uint32, num int) (addr.LogicAddr, error) {
	return defaultCluster.Mod(tt, num)
}

func Random(tt uint32) (addr.LogicAddr, error) {
	return defaultCluster.Random(tt)
}

func Select(tt uint32) ([]addr.LogicAddr, error) {
	return defaultCluster.Select(tt)
}

func SetPeerDisconnected(cb func(addr.LogicAddr, error)) {
	defaultCluster.SetPeerDisconnected(cb)
}

func Register(cmd uint16, handler MsgHandler) {
	defaultCluster.Register(cmd, handler)
}

func RegisterMethod(arg proto.Message, handler rpc.RPCMethodHandler) {
	defaultCluster.RegisterMethod(arg, handler)
}

func AsynCall(peer addr.LogicAddr, arg proto.Message, timeout uint32, cb rpc.RPCResponseHandler) {
	defaultCluster.AsynCall(peer, arg, timeout, cb)
}

func RegisterTimerOnce(expired time.Time, callback func(*timer.Timer, interface{}), ctx interface{}) *timer.Timer {
	return defaultCluster.RegisterTimerOnce(expired, callback, ctx)
}

func RegisterTimer(timeout time.Duration, callback func(*timer.Timer, interface{}), ctx interface{}) *timer.Timer {
	return defaultCluster.RegisterTimer(timeout, callback, ctx)
}
