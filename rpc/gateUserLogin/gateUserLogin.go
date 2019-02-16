
package gateUserLogin

import (
	"github.com/sniperHW/sanguo/cluster"
	"github.com/sniperHW/sanguo/cluster/addr"
	"github.com/sniperHW/kendynet/rpc"
	ss_rpc "github.com/sniperHW/sanguo/protocol/ss/rpc"
)

type GateUserLoginReplyer struct {
	replyer_ *rpc.RPCReplyer
}

func (this *GateUserLoginReplyer) Reply(result *ss_rpc.GateUserLoginResp) {
	this.replyer_.Reply(result,nil)
}

func (this *GateUserLoginReplyer) Error(err error) {
	this.replyer_.Reply(nil,err)
}


func (this *GateUserLoginReplyer) GetChannel() rpc.RPCChannel {
	return this.replyer_.GetChannel()
}


type GateUserLogin interface {
	OnCall(*GateUserLoginReplyer,*ss_rpc.GateUserLoginReq)
}

func Register(methodObj GateUserLogin) {
	f := func(r *rpc.RPCReplyer, arg interface{}) {
		replyer_ := &GateUserLoginReplyer{replyer_:r}
		methodObj.OnCall(replyer_,arg.(*ss_rpc.GateUserLoginReq))
	}

	cluster.RegisterMethod(&ss_rpc.GateUserLoginReq{},f)
}

func AsynCall(peer addr.LogicAddr,arg *ss_rpc.GateUserLoginReq,timeout uint32,cb func(*ss_rpc.GateUserLoginResp,error)) {
	callback := func(r interface{},e error) {
		if nil != r {
			cb(r.(*ss_rpc.GateUserLoginResp),e)
		} else {
			cb(nil,e)
		}
	}
	cluster.AsynCall(peer,arg,timeout,callback)
}

func SyncCall(peer addr.LogicAddr,arg *ss_rpc.GateUserLoginReq,timeout uint32) (ret *ss_rpc.GateUserLoginResp, err error) {
	respChan := make(chan struct{})
	f := func(ret_ *ss_rpc.GateUserLoginResp, err_ error) {
		ret = ret_
		err = err_
		respChan <- struct{}{}
	}
	AsynCall(peer,arg,timeout,f)
	_ = <-respChan
	return
}

