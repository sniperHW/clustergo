
package echo

import (
	"github.com/sniperHW/clustergo"
	"github.com/sniperHW/clustergo/addr"
	"github.com/sniperHW/rpcgo"
	"context"
	"time"
)

type Replyer struct {
	replyer *rpcgo.Replyer
}

func (r *Replyer) Reply(result *EchoRsp) {
	r.replyer.Reply(result)
}

func (r *Replyer) Error(err error) {
	r.replyer.Error(err)
}

func (r *Replyer) Channel() rpcgo.Channel {
	return r.replyer.Channel()
}

type Echo interface {
	ServeEcho(context.Context, *Replyer,*EchoReq)
}

func Register(o Echo) {
	clustergo.RegisterService("echo",func(ctx context.Context, r *rpcgo.Replyer,arg *EchoReq) {
		o.ServeEcho(ctx,&Replyer{replyer:r},arg)
	})
}


func Call(ctx context.Context, peer addr.LogicAddr,arg *EchoReq) (*EchoRsp,error) {
	return clustergo.Call[*EchoReq,EchoRsp](ctx,peer,"echo",arg)
}

func CallWithTimeout(peer addr.LogicAddr,arg *EchoReq,d time.Duration) (*EchoRsp,error) {
	return clustergo.CallWithTimeout[*EchoReq,EchoRsp](peer,"echo",arg,d)
}

func AsyncCall(peer addr.LogicAddr,arg *EchoReq,deadline time.Time,callback func(*EchoRsp,error)) error {
	return clustergo.AsyncCall[*EchoReq,EchoRsp](peer,"echo",arg,deadline,callback)
}

