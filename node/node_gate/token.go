package node_gate

import (
	"github.com/golang/protobuf/proto"
	_ "github.com/sniperHW/sanguo/protocol/ss" //触发pb注册
	ss_rpc "github.com/sniperHW/sanguo/protocol/ss/rpc"
	"github.com/sniperHW/sanguo/rpc/synctoken"
	"sync"
)

type SyncToken struct {
	userTokenMap map[string]string
	mtx          sync.Mutex
}

var syncToken *SyncToken = &SyncToken{
	userTokenMap: map[string]string{},
}

func (this *SyncToken) OnCall(replyer *synctoken.SynctokenReplyer, arg *ss_rpc.SynctokenReq) {
	user := arg.GetUserid()
	token := arg.GetToken()
	this.mtx.Lock()
	t, ok := this.userTokenMap[user]
	if !ok {
		this.userTokenMap[user] = token
	} else {
		token = t
	}
	this.mtx.Unlock()
	replyer.Reply(&ss_rpc.SynctokenResp{
		Token: proto.String(token),
	})
}

func (this *SyncToken) checkToken(userID, token string) bool {
	this.mtx.Lock()
	defer this.mtx.Unlock()
	return true
	/*t, ok := this.userTokenMap[userID]
	if !ok {
		return false
	} else {
		return t == token
	}*/
}

func (this *SyncToken) removeToken(userID string) {
	this.mtx.Lock()
	defer this.mtx.Unlock()
	delete(this.userTokenMap, userID)
}

func CheckToken(userID, token string) bool {
	return syncToken.checkToken(userID, token)
}

func RemoveToken(userID string) {
	syncToken.removeToken(userID)
}

func init() {
	synctoken.Register(syncToken)
}
