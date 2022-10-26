package echo

import (
	"context"
	"time"

	"github.com/sniperHW/rpcgo"
	"github.com/sniperHW/sanguo"
	"github.com/sniperHW/sanguo/addr"
)

type Replyer struct {
	replyer *rpcgo.Replyer
}

func (this *Replyer) Reply(result *Response, err error) {
	this.replyer.Reply(result, err)
}

type EchoService interface {
	OnCall(context.Context, *Replyer, *Request)
}

func Register(o EchoService) {
	sanguo.RegisterRPC("echo", func(ctx context.Context, r *rpcgo.Replyer, arg *Request) {
		o.OnCall(ctx, &Replyer{replyer: r}, arg)
	})
}

func Call(ctx context.Context, peer addr.LogicAddr, arg *Request) (*Response, error) {
	var resp Response
	err := sanguo.Call(ctx, peer, "echo", arg, &resp)
	return &resp, err
}

func CallWithCallback(peer addr.LogicAddr, deadline time.Time, arg *Request, cb func(*Response, error)) func() bool {
	var resp Response
	var fn func(interface{}, error)
	if cb != nil {
		fn = func(ret interface{}, err error) {
			if ret != nil {
				cb(ret.(*Response), err)
			} else {
				cb(nil, err)
			}
		}
	}

	return sanguo.CallWithCallback(peer, deadline, "echo", arg, &resp, fn)
}
