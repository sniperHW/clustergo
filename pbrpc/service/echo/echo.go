
package echo

import (
	"github.com/sniperHW/clustergo"
	"github.com/sniperHW/clustergo/addr"
	"github.com/sniperHW/rpcgo"
	"context"
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
	clustergo.RegisterRPC("echo",func(ctx context.Context, r *rpcgo.Replyer,arg *EchoReq) {
		o.ServeEcho(ctx,&Replyer{replyer:r},arg)
	})
}

func Call(ctx context.Context, peer addr.LogicAddr,arg *EchoReq) (*EchoRsp,error) {
	var resp EchoRsp
	err := clustergo.Call(ctx,peer,"echo",arg,&resp)
	return &resp,err
}
