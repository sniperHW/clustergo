package cs

import (
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/util"
	//"net"
	codecs "github.com/sniperHW/sanguo/codec/cs"
	"github.com/sniperHW/sanguo/common"
	"github.com/sniperHW/sanguo/network"
	_ "github.com/sniperHW/sanguo/protocol/cs" //触发pb注册
	cs_proto "github.com/sniperHW/sanguo/protocol/cs/message"
	"time"
)

var (
	sessions map[kendynet.StreamSession]time.Time
	queue    *util.BlockQueue
)

func DialTcp(peerAddr string, timeout time.Duration, dispatcher ClientDispatcher) {
	go func() {
		conn, err := network.Dial("tcp", peerAddr, timeout)
		if err != nil {
			dispatcher.OnConnectFailed(peerAddr, err)
		} else {
			session := network.CreateSession(conn)
			queue.Add(func() {
				sessions[session] = time.Now().Add(common.HeartBeat_Timeout_Client / 2)
				session.SetRecvTimeout(common.HeartBeat_Timeout_Client)
				session.SetInBoundProcessor(codecs.NewReceiver("sc"))
				session.SetEncoder(codecs.NewEncoder("cs"))
				session.SetCloseCallBack(func(sess kendynet.StreamSession, reason error) {
					queue.Add(func() {
						delete(sessions, sess)
					})
					dispatcher.OnClose(sess, reason)
				})
				dispatcher.OnEstablish(session)
				session.BeginRecv(func(s kendynet.StreamSession, m interface{}) {
					msg := m.(*codecs.Message)
					switch msg.GetData().(type) {
					case *cs_proto.HeartbeatToC:
						//fmt.Printf("on HeartbeatToC\n")
						break
					default:
						dispatcher.Dispatch(session, msg)
						break
					}
				})
			})
		}
	}()
}

func init() {
	sessions = make(map[kendynet.StreamSession]time.Time)
	queue = util.NewBlockQueue()

	go func() {
		for {
			_, localList := queue.Get()
			for _, task := range localList {
				task.(func())()
			}
		}
	}()

	go func() {
		for {
			queue.Add(func() {
				now := time.Now()
				for k, v := range sessions {
					if !now.Before(v) {
						sessions[k] = now.Add(common.HeartBeat_Timeout_Client / 2)
						//发送心跳
						Heartbeat := &cs_proto.HeartbeatToS{}
						k.Send(codecs.NewMessage(0, Heartbeat))
					}
				}
			})
			time.Sleep(time.Millisecond * 1000)
		}
	}()
}
