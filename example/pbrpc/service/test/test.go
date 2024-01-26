
package test

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
	clustergo.GetRPCServer().RegisterService("test",func(ctx context.Context, r *rpcgo.Replyer,arg *TestReq) {
		o.ServeTest(ctx,&Replyer{replyer:r},arg)
	})
}

var client *clustergo.RPCClient = clustergo.GetRPCClient()

func Call(ctx context.Context, peer addr.LogicAddr,arg *TestReq) (*TestRsp,error) {
	var resp TestRsp
	err := client.Call(ctx,peer,"test",arg,&resp)
	return &resp,err
}

func CallWithTimeout(peer addr.LogicAddr,arg *TestReq,d time.Duration) (*TestRsp,error) {
	var resp TestRsp
	err := client.CallWithTimeout(peer,"test",arg,&resp,d)
	return &resp,err
}

func AsyncCall(peer addr.LogicAddr,arg *TestReq,deadline time.Time,callback func(*TestRsp,error)) error {
	var resp TestRsp
	err := client.AsyncCall(peer,"test",arg,&resp,deadline,func(res interface{},err error){
		callback(res.(*TestRsp),err)
	})
	return err
}

