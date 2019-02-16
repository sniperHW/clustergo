package node_gate

import (
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/sanguo/cluster"
	"github.com/sniperHW/sanguo/node/node_gate/codec"
	cs_message "github.com/sniperHW/sanguo/protocol/cs/message"
)

func onReconnect(session kendynet.StreamSession, msg *codec.Message) {
	ud := session.GetUserData()
	if nil != ud {
		session.Close("reconnect failed", 0)
		return
	}

	req := msg.GetData().(*cs_message.ReconnectToS)

	u := GetUserByUserID(req.GetUserID())
	if nil == u {
		session.Close("reconnect failed", 0)
		return
	}

	if u.conn != nil {
		session.Close("reconnect failed", 0)
		return
	}

	if u.token != req.GetToken() {
		session.Close("reconnect failed", 0)
		return
	}

	session.SetUserData(u)
	u.mtx.Lock()
	u.conn = session
	if nil != u.t {
		cluster.UnregisterTimer(u.t)
		u.t = nil
	}
	session.Send(codec.NewMessage(msg.GetSeriNo(), &cs_message.ReconnectToC{
		Code: cs_message.EnumType(cs_message.EnumType_OK).Enum(),
	}))

	//将断线期间缓存的消息发送给客户端
	u.FlushSCMessage()
	u.mtx.Unlock()

}
