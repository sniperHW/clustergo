package test

import (
	"context"
	"github.com/sniperHW/clustergo"
	"github.com/sniperHW/clustergo/addr"
	"github.com/sniperHW/clustergo/rpc"
	"time"
)

type Replyer struct {
	replyer *rpc.Replyer
}

func (r *Replyer) Reply(result *TestRsp) {
	r.replyer.Reply(result)
}

func (r *Replyer) Error(err error) {
	r.replyer.Error(err)
}

func (r *Replyer) Channel() rpc.Channel {
	return r.replyer.Channel()
}

type Test interface {
	ServeTest(context.Context, *Replyer, *TestReq)
}

func Register(o Test) {
	clustergo.RegisterService("test", func(ctx context.Context, r *rpc.Replyer, arg *TestReq) {
		o.ServeTest(ctx, &Replyer{replyer: r}, arg)
	})
}

func Call(ctx context.Context, peer addr.LogicAddr, arg *TestReq) (*TestRsp, error) {
	return clustergo.Call[*TestReq, TestRsp](ctx, peer, "test", arg)
}

func CallWithTimeout(peer addr.LogicAddr, arg *TestReq, d time.Duration) (*TestRsp, error) {
	return clustergo.CallWithTimeout[*TestReq, TestRsp](peer, "test", arg, d)
}

func AsyncCall(peer addr.LogicAddr, arg *TestReq, deadline time.Time, callback func(*TestRsp, error)) error {
	return clustergo.AsyncCall[*TestReq, TestRsp](peer, "test", arg, deadline, callback)
}
