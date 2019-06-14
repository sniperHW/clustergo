package node_gate

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/event"
	listener "github.com/sniperHW/kendynet/socket/listener/tcp"
	"github.com/sniperHW/kendynet/timer"
	"github.com/sniperHW/sanguo/cluster"
	"github.com/sniperHW/sanguo/cluster/addr"
	"github.com/sniperHW/sanguo/common"
	"github.com/sniperHW/sanguo/node/node_gate/codec"
	cs_message "github.com/sniperHW/sanguo/protocol/cs/message"
	ss_message "github.com/sniperHW/sanguo/protocol/ss/ssmessage"
	"math/rand"
	"strconv"
	"strings"
	"time"
)

var (
	started        int32
	l              *listener.Listener
	heartbeatBytes []byte
)

func onCliHeartBeat(session kendynet.StreamSession, msg *codec.Message) {
	Heartbeat := &cs_message.HeartbeatToC{}
	session.Send(codec.NewMessage(0, Heartbeat))

	ud := session.GetUserData()
	if nil != ud {
		u := ud.(*gateUser)
		if u.holdGateSession {
			u.holdGateSession = false
			u.forwardSS(heartbeatBytes)
		} else {
			//尝试发送被缓存的上行消息
			u.forwardSS(nil)
		}
	}
}

func onGateMessage(session kendynet.StreamSession, msg *codec.Message) {
	dispatchClientMsg(session, msg)
}

func onCliMessage(session kendynet.StreamSession, msg interface{}) {

	Debugln("onCliMessage")

	fmt.Println("onCliMessage")

	switch msg.(type) {
	//来自客户端透传到内部服务器的消息
	case *codec.RelayMessage:
		Debugln("RelayMessage")
		u := session.GetUserData()
		if u != nil {
			u.(*gateUser).forwardSS(msg.(*codec.RelayMessage).GetData())
		}
		break
	//来自客户端需要由gate处理的消息
	case *codec.Message:
		Debugln("gateMessage")
		onGateMessage(session, msg.(*codec.Message))
		break
	default:
		break
	}
}

func onCliClose(session kendynet.StreamSession, reason string) {
	ud := session.GetUserData()
	if nil != ud {
		u := ud.(*gateUser)
		u.mtx.Lock()
		defer u.mtx.Unlock()
		u.conn = nil
		Debugln("onClient disconnect", reason)
		if u.status == status_ok || u.status == status_migration {
			//启动定时器等待重连
			u.t = cluster.RegisterTimerOnce(time.Now().Add(time.Second*60), func(_ *timer.Timer) {
				Debugln("timeout remove gateUser", u.userID)
				remGateUser(u)
			})
		}
	}
}

func Start(externalAddr string) error {
	//初始化随机种子
	rand.Seed(time.Now().Unix())

	t := strings.Split(externalAddr, ":")
	port, _ := strconv.Atoi(t[1])

	l, err := listener.New("tcp", fmt.Sprintf("0.0.0.0:%d", port))
	if l != nil {
		go func() {
			fmt.Printf("server running on:%s\n", fmt.Sprintf("0.0.0.0:%d", port))
			err = l.Serve(func(session kendynet.StreamSession) {
				session.SetRecvTimeout(common.HeartBeat_Timeout_Client * time.Second)
				session.SetReceiver(codec.NewReceiver("cs", gateMessage))
				session.SetEncoder(codec.NewEncoder("sc"))
				session.SetCloseCallBack(onCliClose)
				session.Start(func(event *kendynet.Event) {
					if event.EventType == kendynet.EventTypeError {
						event.Session.Close(event.Data.(error).Error(), 0)
					} else {
						onCliMessage(session, event.Data)
					}
				})
			})
			if nil != err {
				fmt.Printf("TcpServer start failed %s\n", err)
			}
		}()

	} else {
		fmt.Printf("NewListener failed %s\n", err)
	}
	return err
}

func onKickGateUser(_ addr.LogicAddr, msg proto.Message) {
	req := msg.(*ss_message.KickGateUser)
	Debugln("onKickGateUser", req.GetUserID())
	u := GetUserByUserID(req.GetUserID())
	if nil != u {
		remGateUser(u)
	}
}

func onPeerDisconnected(peer addr.LogicAddr, err error) {
	Debugln("onPeerDisconnected", peer.String(), err)
	if peer.Type() == 3 {
		userMgr.mtx.Lock()
		defer userMgr.mtx.Unlock()
		for _, u := range userMgr.uidUserMap {
			if u.game == peer {
				/*  连接断开的原因可能是网络闪断,如果是这种原因，game会为玩家的gate会话维持60秒钟
				 *  如果60秒内收到gate的上行消息则会话继续保持，否则game会将玩家的gate会话信息销毁。
				 *  为了防止60秒内客户端无主动的上行消息，需要设置holdGateSession标记，如果标记被打开
				 *  接收到玩家心跳消息时会向game上行一条消息以保持gate会话
				 */
				u.holdGateSession = true
			}
		}
	}
}

func Init() {

	encoder := codec.NewEncoder("cs")
	msg, _ := encoder.EnCode(codec.NewMessage(0, &cs_message.HeartbeatToS{}))
	heartbeatBytes = msg.Bytes()

	relayRoutineCount := uint32(10)

	pool := make([]*event.EventQueue, relayRoutineCount)
	for i := uint32(0); i < relayRoutineCount; i++ {
		q := event.NewEventQueue(1000)
		pool[i] = q
		go q.Run()
	}

	registerCliHandler(&cs_message.ReconnectToS{}, onReconnect)

	registerCliHandler(&cs_message.GameLoginToS{}, onLogin)

	registerCliHandler(&cs_message.HeartbeatToS{}, onCliHeartBeat)

	cluster.Register(&ss_message.KickGateUser{}, onKickGateUser)

	cluster.RegisterPeerDisconnected(onPeerDisconnected)

	cluster.Register(&ss_message.SsToGate{}, func(from addr.LogicAddr, msg proto.Message) {
		ssMsg := msg.(*ss_message.SsToGate)
		Debugln("onSSToGate", *ssMsg)
		for _, v := range ssMsg.GetGateUsers() {
			Debugln("relay to ", v)
			u := GetUserByGateID(v)
			if nil != u {
				pool[v%relayRoutineCount].Post(func() {
					err := u.RelaySCMessage(ssMsg.GetMessage())
					if nil != err {
						remGateUser(u)
					}
				})
			} else {
				if from.Type() == 3 {
					cluster.PostMessage(from, &ss_message.SsToGateError{
						Guid: proto.Uint32(v),
					})
				}
			}
		}
	})

}
