
package synctoken

import (
	"github.com/sniperHW/sanguo/cluster"
	"github.com/sniperHW/sanguo/cluster/addr"
	"github.com/sniperHW/kendynet/rpc"
	ss_rpc "github.com/sniperHW/sanguo/protocol/ss/rpc"
)

type SynctokenReplyer struct {
	replyer_ *rpc.RPCReplyer
}

func (this *SynctokenReplyer) Reply(result *ss_rpc.SynctokenResp) {
	this.replyer_.Reply(result,nil)
}

func (this *SynctokenReplyer) Error(err error) {
	this.replyer_.Reply(nil,err)
}


func (this *SynctokenReplyer) GetChannel() rpc.RPCChannel {
	return this.replyer_.GetChannel()
}


type Synctoken interface {
	OnCall(*SynctokenReplyer,*ss_rpc.SynctokenReq)
}

func Register(methodObj Synctoken) {
	f := func(r *rpc.RPCReplyer, arg interface{}) {
		replyer_ := &SynctokenReplyer{replyer_:r}
		methodObj.OnCall(replyer_,arg.(*ss_rpc.SynctokenReq))
	}

	cluster.RegisterMethod(&ss_rpc.SynctokenReq{},f)
}

func AsynCall(peer addr.LogicAddr,arg *ss_rpc.SynctokenReq,timeout uint32,cb func(*ss_rpc.SynctokenResp,error)) {
	callback := func(r interface{},e error) {
		if nil != r {
			cb(r.(*ss_rpc.SynctokenResp),e)
		} else {
			cb(nil,e)
		}
	}
	cluster.AsynCall(peer,arg,timeout,callback)
}

func SyncCall(peer addr.LogicAddr,arg *ss_rpc.SynctokenReq,timeout uint32) (ret *ss_rpc.SynctokenResp, err error) {
	respChan := make(chan struct{})
	f := func(ret_ *ss_rpc.SynctokenResp, err_ error) {
		ret = ret_
		err = err_
		respChan <- struct{}{}
	}
	AsynCall(peer,arg,timeout,f)
	_ = <-respChan
	return
}

