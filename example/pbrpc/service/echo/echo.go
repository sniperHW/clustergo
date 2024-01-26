
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
	clustergo.GetRPCServer().RegisterService("echo",func(ctx context.Context, r *rpcgo.Replyer,arg *EchoReq) {
		o.ServeEcho(ctx,&Replyer{replyer:r},arg)
	})
}

var client *clustergo.RPCClient = clustergo.GetRPCClient()

func Call(ctx context.Context, peer addr.LogicAddr,arg *EchoReq) (*EchoRsp,error) {
	var resp EchoRsp
	err := client.Call(ctx,peer,"echo",arg,&resp)
	return &resp,err
}

func CallWithTimeout(peer addr.LogicAddr,arg *EchoReq,d time.Duration) (*EchoRsp,error) {
	var resp EchoRsp
	err := client.CallWithTimeout(peer,"echo",arg,&resp,d)
	return &resp,err
}

func AsyncCall(peer addr.LogicAddr,arg *EchoReq,deadline time.Time,callback func(*EchoRsp,error)) error {
	var resp EchoRsp
	err := client.AsyncCall(peer,"echo",arg,&resp,deadline,func(res interface{},err error){
		callback(res.(*EchoRsp),err)
	})
	return err
}

