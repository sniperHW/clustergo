
package forwardUserMsg

import (
	"github.com/sniperHW/sanguo/cluster"
	"github.com/sniperHW/sanguo/cluster/addr"
	"github.com/sniperHW/kendynet/rpc"
	ss_rpc "github.com/sniperHW/sanguo/protocol/ss/rpc"
)

type ForwardUserMsgReplyer struct {
	replyer_ *rpc.RPCReplyer
}

func (this *ForwardUserMsgReplyer) Reply(result *ss_rpc.ForwardUserMsgResp) {
	this.replyer_.Reply(result,nil)
}

func (this *ForwardUserMsgReplyer) Error(err error) {
	this.replyer_.Reply(nil,err)
}


func (this *ForwardUserMsgReplyer) GetChannel() rpc.RPCChannel {
	return this.replyer_.GetChannel()
}


type ForwardUserMsg interface {
	OnCall(*ForwardUserMsgReplyer,*ss_rpc.ForwardUserMsgReq)
}

func Register(methodObj ForwardUserMsg) {
	f := func(r *rpc.RPCReplyer, arg interface{}) {
		replyer_ := &ForwardUserMsgReplyer{replyer_:r}
		methodObj.OnCall(replyer_,arg.(*ss_rpc.ForwardUserMsgReq))
	}

	cluster.RegisterMethod(&ss_rpc.ForwardUserMsgReq{},f)
}

func AsynCall(peer addr.LogicAddr,arg *ss_rpc.ForwardUserMsgReq,timeout uint32,cb func(*ss_rpc.ForwardUserMsgResp,error)) {
	callback := func(r interface{},e error) {
		if nil != r {
			cb(r.(*ss_rpc.ForwardUserMsgResp),e)
		} else {
			cb(nil,e)
		}
	}
	cluster.AsynCall(peer,arg,timeout,callback)
}

func SyncCall(peer addr.LogicAddr,arg *ss_rpc.ForwardUserMsgReq,timeout uint32) (ret *ss_rpc.ForwardUserMsgResp, err error) {
	respChan := make(chan struct{})
	f := func(ret_ *ss_rpc.ForwardUserMsgResp, err_ error) {
		ret = ret_
		err = err_
		respChan <- struct{}{}
	}
	AsynCall(peer,arg,timeout,f)
	_ = <-respChan
	return
}

