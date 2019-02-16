package main

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/sanguo/cluster"
	"github.com/sniperHW/sanguo/cluster/addr"
	"github.com/sniperHW/sanguo/codec/cs"
	cs_message "github.com/sniperHW/sanguo/protocol/cs/message"
	ss_rpc "github.com/sniperHW/sanguo/protocol/ss/rpc"
	ss_message "github.com/sniperHW/sanguo/protocol/ss/ssmessage"
	"github.com/sniperHW/sanguo/rpc/forwardUserMsg"
	"github.com/sniperHW/sanguo/rpc/gateUserLogin"
)

var receiver *cs.Receiver
var encoder *cs.Encoder

var users map[string]*gameUser

type gate struct {
	gate addr.LogicAddr
	guid uint32
}

type gameUser struct {
	userID   string
	gateInfo *gate
}

func (this *gameUser) Send(msg *cs.Message) {
	if nil != this.gateInfo && addr.LogicAddr(0) != this.gateInfo.gate {
		buffer, err := encoder.EnCode(msg)
		if nil == err {
			ssMsg := &ss_message.SsToGate{
				GateUsers: []uint32{this.gateInfo.guid},
				Message:   [][]byte{buffer.Bytes()},
			}
			cluster.PostMessage(this.gateInfo.gate, ssMsg)
		} else {
			fmt.Println(err)
		}
	}
}

func (this *gameUser) onCliMsg(msg *cs.Message) {

	switch msg.GetData().(type) {
	case *cs_message.EchoToS:
		this.Send(cs.NewMessage(0, &cs_message.EchoToC{Msg: proto.String("world")}))
		break
	default:
		break
	}

}

func (this *gameUser) processCliMsg(msgs [][]byte) {
	//收到消息直接回射
	for _, v := range msgs {
		msg, err := receiver.DirectUnpack(v)
		if nil == err {
			this.onCliMsg(msg.(*cs.Message))
		}
	}
}

type GateUserLogin struct {
}

func (this *GateUserLogin) OnCall(replyer *gateUserLogin.GateUserLoginReplyer, arg *ss_rpc.GateUserLoginReq) {

	fmt.Println("GateUserLogin", arg.GetUserID())

	channel := replyer.GetChannel().(*cluster.RPCChannel)

	gatePeerID := channel.PeerAddr() //cluster.GetPeerIDBySession(channel.GetSession())

	u := users[arg.GetUserID()]
	if nil == u {
		u := &gameUser{
			userID: arg.GetUserID(),
			gateInfo: &gate{
				gate: gatePeerID,
				guid: arg.GetGateUserID(),
			},
		}

		users[u.userID] = u

		//通告成功
		replyer.Reply(&ss_rpc.GateUserLoginResp{
			Code: ss_rpc.GateLoginEnumType(ss_rpc.GateLoginEnumType_OK).Enum(),
		})
	} else {
		if nil == u.gateInfo {
			u.gateInfo = &gate{
				gate: gatePeerID,
				guid: arg.GetGateUserID(),
			}
			//通告成功
			replyer.Reply(&ss_rpc.GateUserLoginResp{
				Code: ss_rpc.GateLoginEnumType(ss_rpc.GateLoginEnumType_OK).Enum(),
			})
		} else {
			//向原gate发送踢人消息

			cluster.PostMessage(u.gateInfo.gate, &ss_message.KickGateUser{
				UserID: proto.String(u.userID),
			})

			replyer.Reply(&ss_rpc.GateUserLoginResp{
				Code: ss_rpc.GateLoginEnumType(ss_rpc.GateLoginEnumType_RETRY).Enum(),
			})

		}
	}
}

type ForwardUserMsg struct {
}

func (this *ForwardUserMsg) OnCall(replyer *forwardUserMsg.ForwardUserMsgReplyer, arg *ss_rpc.ForwardUserMsgReq) {
	channel := replyer.GetChannel().(*cluster.RPCChannel)
	/*gatePeerID, err := cluster.GetPeerIDBySession(channel.GetSession())

	if nil != err {
		panic("invaild gate session")
	}*/

	gatePeerID := channel.PeerAddr()

	u := users[arg.GetUserID()]
	if nil == u {
		replyer.Reply(&ss_rpc.ForwardUserMsgResp{
			Code: ss_rpc.ForwordEnumType(ss_rpc.ForwordEnumType_Forword_ERROR_UID).Enum(),
		})
		return
	}

	if u.gateInfo == nil || u.gateInfo.gate == addr.LogicAddr(0) {
		replyer.Reply(&ss_rpc.ForwardUserMsgResp{
			Code: ss_rpc.ForwordEnumType(ss_rpc.ForwordEnumType_Forword_ERROR_UID).Enum(),
		})
		return
	}

	if u.gateInfo.gate != gatePeerID || u.gateInfo.guid != arg.GetGateUserID() {
		replyer.Reply(&ss_rpc.ForwardUserMsgResp{
			Code: ss_rpc.ForwordEnumType(ss_rpc.ForwordEnumType_Forword_ERROR_UID).Enum(),
		})
		return
	}

	u.processCliMsg(arg.GetMessages())
	replyer.Reply(&ss_rpc.ForwardUserMsgResp{
		Code: ss_rpc.ForwordEnumType(ss_rpc.ForwordEnumType_Forword_OK).Enum(),
	})
}

func onKickGateUser(_ addr.LogicAddr, msg proto.Message) {
	req := msg.(*ss_message.KickGateUser)
	u := users[req.GetUserID()]
	if u != nil && nil != u.gateInfo && addr.LogicAddr(0) != u.gateInfo.gate {
		cluster.PostMessage(u.gateInfo.gate, msg)
	}
}

func onGateUserDestroy(from addr.LogicAddr, msg proto.Message) {
	req := msg.(*ss_message.GateUserDestroy)
	uid := req.GetUserID()
	guid := req.GetGateUserID()

	u := users[uid]
	if u == nil || nil == u.gateInfo || addr.LogicAddr(0) == u.gateInfo.gate {
		return
	}

	if u.gateInfo.guid == guid && u.gateInfo.gate == from {
		u.gateInfo = nil
	}
}

func main() {

	receiver = cs.NewReceiver("cs")
	encoder = cs.NewEncoder("sc")

	users = map[string]*gameUser{}

	gateUserLogin.Register(&GateUserLogin{})

	forwardUserMsg.Register(&ForwardUserMsg{})

	cluster.Register(&ss_message.KickGateUser{}, onKickGateUser)
	cluster.Register(&ss_message.GateUserDestroy{}, onGateUserDestroy)

	centerAddrs := []string{"localhost:8010"}

	selfAddr, _ := addr.MakeAddr("1.3.1", "localhost:8011")

	err := cluster.Start(centerAddrs, selfAddr) //cluster.MakeService("game", "localhost", int32(8011)))

	if nil != err {
		fmt.Println("testGame start failed1:%s\n", err.Error())
		return
	} else {
		fmt.Println("testGame start ok")
		sigStop := make(chan bool)
		_, _ = <-sigStop
	}

}
