
package test

import (
	"github.com/sniperHW/sanguo"
	"github.com/sniperHW/sanguo/addr"
	"github.com/sniperHW/rpcgo"
	"context"
	"time"
)

type Replyer struct {
	replyer *rpcgo.Replyer
}

func (r *Replyer) Reply(result *Response,err error) {
	r.replyer.Reply(result,err)
}

func (r *Replyer) Channel() rpcgo.Channel {
	return r.replyer.Channel()
}

type TestService interface {
	OnCall(context.Context, *Replyer,*Request)
}

func Register(o TestService) {
	sanguo.RegisterRPC("test",func(ctx context.Context, r *rpcgo.Replyer,arg *Request) {
		o.OnCall(ctx,&Replyer{replyer:r},arg)
	})
}

func Call(ctx context.Context, peer addr.LogicAddr,arg *Request) (*Response,error) {
	var resp Response
	err := sanguo.Call(ctx,peer,"test",arg,&resp)
	return &resp,err
}

func CallWithCallback(peer addr.LogicAddr,deadline time.Time,arg *Request,cb func(*Response,error)) func() bool {
	var resp Response
	var fn func(interface{}, error)
	if cb != nil {
		fn = func (ret interface{}, err error){
			if ret != nil {
				cb(ret.(*Response),err)
			} else {
				cb(nil,err)
			}
		}
	}


	return sanguo.CallWithCallback(peer,deadline,"test",arg,&resp,fn) 		
}
