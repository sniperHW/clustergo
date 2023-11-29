
package test

import (
	"github.com/sniperHW/clustergo"
	"github.com/sniperHW/clustergo/addr"
	"github.com/sniperHW/rpcgo"
	"context"
)

type Replyer struct {
	replyer *rpcgo.Replyer
}

func (r *Replyer) Reply(result *TestRsp) {
	r.replyer.Reply(result)
}

func (r *Replyer) Error(err error) {
	r.replyer.Error(err)
}

func (r *Replyer) Channel() rpcgo.Channel {
	return r.replyer.Channel()
}

type Test interface {
	ServeTest(context.Context, *Replyer,*TestReq)
}

func Register(o Test) {
	clustergo.RegisterRPC("test",func(ctx context.Context, r *rpcgo.Replyer,arg *TestReq) {
		o.ServeTest(ctx,&Replyer{replyer:r},arg)
	})
}

func Call(ctx context.Context, peer addr.LogicAddr,arg *TestReq) (*TestRsp,error) {
	var resp TestRsp
	err := clustergo.Call(ctx,peer,"test",arg,&resp)
	return &resp,err
}
