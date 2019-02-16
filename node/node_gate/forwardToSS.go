package node_gate

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet/rpc"
	"github.com/sniperHW/sanguo/cluster"
	"github.com/sniperHW/sanguo/cluster/addr"
	ss_rpc "github.com/sniperHW/sanguo/protocol/ss/rpc"
	"github.com/sniperHW/sanguo/rpc/forwardUserMsg"
	"sync/atomic"
)

var errInvaildGateUser = fmt.Errorf("invaild gateuser")

//将来自客户端的消息发往内部服务

func isTemporary(err error) bool {
	return err == cluster.ErrDial || err == rpc.ErrCallTimeout
}

func (this *gateUser) forwardSS(msg []byte) {
	Debugln("forwardSS")
	var resp *ss_rpc.ForwardUserMsgResp
	var err error

	this.mtx.Lock()
	defer func() {
		this.mtx.Unlock()
		if nil != err {
			if nil != this.conn {
				this.conn.Close("", 0)
			}
			remGateUser(this)
		}
	}()

	if this.status == status_remove {
		return
	} else {

		if nil != msg {
			err = this.upQueue.append(msg)
		}

		if len(this.upQueue.queue) == 0 {
			return
		}

		if nil != err {
			Debugln("forwardSS", err)
			return
		} else {
			if this.status == status_migration || addr.LogicAddr(0) == this.game {
				return
			} else {
				req := &ss_rpc.ForwardUserMsgReq{
					UserID:     proto.String(this.userID),
					GateUserID: proto.Uint32(this.id),
					Messages:   this.upQueue.queue,
				}
				this.mtx.Unlock()
				resp, err = forwardUserMsg.SyncCall(this.game, req, 1000)
				this.mtx.Lock()
				if nil != err {
					if isTemporary(err) {
						//忽略错误，稍后重试
						err = nil
						Debugln("call failed")
					} else {
						Debugln("forwardSS", err)
					}
					return
				} else {
					code := resp.GetCode()
					if code == ss_rpc.ForwordEnumType_Forword_OK {
						this.upQueue.queue = [][]byte{}
						atomic.AddInt64(&totalSendQueueBytes, -this.upQueue.byteSize)
						this.upQueue.byteSize = 0
					} else if code == ss_rpc.ForwordEnumType_Forword_ERROR_UID || code == ss_rpc.ForwordEnumType_Forword_ERROR_GUID {
						err = errInvaildGateUser
						Debugln("invaild gateuser")
						return
					}
				}
			}
		}
	}
}
