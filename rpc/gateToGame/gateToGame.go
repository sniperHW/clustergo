package gateToGame

import (
	"github.com/sniperHW/kendynet/rpc"
	"github.com/sniperHW/sanguo/cluster"
	"github.com/sniperHW/sanguo/cluster/addr"
	ss_rpc "github.com/sniperHW/sanguo/protocol/ss/rpc"
)

type GateToGameReplyer struct {
	replyer_ *rpc.RPCReplyer
}

func (this *GateToGameReplyer) Reply(result *ss_rpc.GateToGameResp) {
	this.replyer_.Reply(result, nil)
}

func (this *GateToGameReplyer) Error(err error) {
	this.replyer_.Reply(nil, err)
}

func (this *GateToGameReplyer) GetChannel() rpc.RPCChannel {
	return this.replyer_.GetChannel()
}

type GateToGame interface {
	OnCall(*GateToGameReplyer, *ss_rpc.GateToGameReq)
}

func Register(methodObj GateToGame) {
	f := func(r *rpc.RPCReplyer, arg interface{}) {
		replyer_ := &GateToGameReplyer{replyer_: r}
		methodObj.OnCall(replyer_, arg.(*ss_rpc.GateToGameReq))
	}

	cluster.RegisterMethod(&ss_rpc.GateToGameReq{}, f)
}

func AsynCall(peer addr.LogicAddr, arg *ss_rpc.GateToGameReq, timeout uint32, cb func(*ss_rpc.GateToGameResp, error)) {
	callback := func(r interface{}, e error) {
		if nil != r {
			cb(r.(*ss_rpc.GateToGameResp), e)
		} else {
			cb(nil, e)
		}
	}
	cluster.AsynCall(peer, arg, timeout, callback)
}

func SyncCall(peer addr.LogicAddr, arg *ss_rpc.GateToGameReq, timeout uint32) (ret *ss_rpc.GateToGameResp, err error) {
	respChan := make(chan struct{})
	f := func(ret_ *ss_rpc.GateToGameResp, err_ error) {
		ret = ret_
		err = err_
		respChan <- struct{}{}
	}
	AsynCall(peer, arg, timeout, f)
	_ = <-respChan
	return
}
