package node_gate

import (
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/sanguo/cluster"
	"github.com/sniperHW/sanguo/cluster/addr"
	"github.com/sniperHW/sanguo/node/node_gate/codec"
	cs_message "github.com/sniperHW/sanguo/protocol/cs/message"
	ss_rpc "github.com/sniperHW/sanguo/protocol/ss/rpc"
	ss_message "github.com/sniperHW/sanguo/protocol/ss/ssmessage"
	"github.com/sniperHW/sanguo/rpc/gateUserLogin"
	"time"
)

func onLogin(session kendynet.StreamSession, msg *codec.Message) {

	ud := session.GetUserData()
	if nil != ud {
		return
	} else {

		onError := func(u *gateUser, closeReason string, code cs_message.EnumType) {

			Infoln("onError", closeReason)

			session.Send(codec.NewMessage(0, &cs_message.GameLoginToC{
				Code: cs_message.EnumType(code).Enum(),
			}))
			session.Close(closeReason, time.Second)
			if nil != u {
				remGateUser(u)
			}
		}

		req := msg.GetData().(*cs_message.GameLoginToS)
		//首先验证令牌
		if !CheckToken(req.GetUserID(), req.GetToken()) {
			session.Close("invaild token", 0)
			return
		}

		game, err := cluster.Random(3)

		if nil != err {
			Debugln(err)
			onError(nil, "busy", cs_message.EnumType_RETRY)
			return
		}

		userMgr.mtx.Lock()

		u := userMgr.uidUserMap[req.GetUserID()]

		if u != nil {
			userMgr.mtx.Unlock()

			status := u.status
			game := u.game

			if status == status_ok && addr.LogicAddr(0) != game {
				//向game发出踢gateUser请求
				cluster.PostMessage(game, &ss_message.KickGateUser{
					UserID: proto.String(req.GetUserID()),
				})
			}
			Debugln("kick")
			onError(nil, "retry", cs_message.EnumType_RETRY)
			return
		}

		id := getGateUserID()
		if 0 == id {
			userMgr.mtx.Unlock()
			Debugln("no free id")
			onError(nil, "busy", cs_message.EnumType_RETRY)
			return
		}

		u = &gateUser{
			id:     id,
			userID: req.GetUserID(),
			conn:   session,
			token:  req.GetToken(),
			upQueue: sendQueue{
				queue: [][]byte{},
			},
			downQueue: sendQueue{
				queue: [][]byte{},
			},
			status: status_login,
		}

		session.SetUserData(u)

		AddGateUser(u)

		userMgr.mtx.Unlock()

		for {
			//向game发起登录请求
			ret, err := gateUserLogin.SyncCall(game, &ss_rpc.GateUserLoginReq{
				UserID:     proto.String(req.GetUserID()),
				GateUserID: proto.Uint32(u.id),
				ServerID:   proto.Int32(req.GetServerID()),
			}, 5000)

			if err != nil {
				Debugln(err)
				onError(u, "busy", cs_message.EnumType_RETRY)
				return
			} else {
				if ret.GetCode() == ss_rpc.GateLoginEnumType_REDIRECT {
					//重定向到另外一台Game
					game, _ = addr.MakeLogicAddr(ret.GetRedirectInfo())
				} else {
					if ret.GetCode() == ss_rpc.GateLoginEnumType_OK {
						session.Send(codec.NewMessage(0, &cs_message.GameLoginToC{
							Code: cs_message.EnumType(cs_message.EnumType_OK).Enum(),
						}))
						u.mtx.Lock()
						u.status = status_ok
						u.game = game
						u.FlushSCMessage()
						u.mtx.Unlock()
						Debugln(req.GetUserID(), "login ok")
					} else {
						Debugln(ret.GetCode())
						onError(u, "busy", cs_message.EnumType_RETRY)
					}
				}
				return
			}
		}
	}
}
