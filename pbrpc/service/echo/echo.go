
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

func (r *Replyer) Reply(result *Response) {
	r.replyer.Reply(result)
}

func (r *Replyer) Error(err error) {
	r.replyer.Error(err)
}

func (r *Replyer) Channel() rpcgo.Channel {
	return r.replyer.Channel()
}

type EchoService interface {
	OnCall(context.Context, *Replyer,*Request)
}

func Register(o EchoService) {
	clustergo.RegisterRPC("echo",func(ctx context.Context, r *rpcgo.Replyer,arg *Request) {
		o.OnCall(ctx,&Replyer{replyer:r},arg)
	})
}

func Call(ctx context.Context, peer addr.LogicAddr,arg *Request) (*Response,error) {
	var resp Response
	err := clustergo.Call(ctx,peer,"echo",arg,&resp)
	return &resp,err
}
