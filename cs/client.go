package cs

import (
	codecs "github.com/sniperHW/sanguo/codec/cs"
	"github.com/sniperHW/sanguo/common"
	_ "github.com/sniperHW/sanguo/protocol/cs" //触发pb注册
	//cs_proto "github.com/sniperHW/sanguo/protocol/cs/message"
	"github.com/sniperHW/kendynet"
	connector "github.com/sniperHW/kendynet/socket/connector/tcp"
	"github.com/sniperHW/kendynet/util"
	"time"
)

var (
	sessions map[kendynet.StreamSession]int64
	queue    *util.BlockQueue
)

func DialTcp(peerAddr string, timeout time.Duration, dispatcher ClientDispatcher) {
	c, _ := connector.New("tcp", peerAddr)
	go func() {
		session, err := c.Dial(timeout)
		if nil != err {
			dispatcher.OnConnectFailed(peerAddr, err)
		} else {
			queue.Add(func() {
				sessions[session] = time.Now().Unix() + (common.HeartBeat_Timeout_Client / 2)
				session.SetRecvTimeout(common.HeartBeat_Timeout_Client * time.Second)
				session.SetReceiver(codecs.NewReceiver("sc"))
				session.SetEncoder(codecs.NewEncoder("cs"))
				session.SetCloseCallBack(func(sess kendynet.StreamSession, reason string) {
					queue.Add(func() {
						delete(sessions, sess)
					})
					dispatcher.OnClose(sess, reason)
				})
				dispatcher.OnEstablish(session)
				session.Start(func(event *kendynet.Event) {
					if event.EventType == kendynet.EventTypeError {
						event.Session.Close(event.Data.(error).Error(), 0)
					} else {
						dispatcher.Dispatch(session, event.Data.(*codecs.Message))
					}
				})
			})
		}
	}()
}

func init() {
	sessions = make(map[kendynet.StreamSession]int64)
	queue = util.NewBlockQueue()

	go func() {
		for {
			_, localList := queue.Get()
			for _, task := range localList {
				task.(func())()
			}
		}
	}()

	/*go func() {
		for {
			queue.Add(func() {
				now := time.Now().Unix()
				for k, v := range sessions {
					if now >= v {
						sessions[k] = now + common.HeartBeat_Timeout_Client/2
						//发送心跳
						Heartbeat := &cs_proto.HeartbeatToS{}
						k.Send(codecs.NewMessage(0, Heartbeat))
					}
				}
			})
			time.Sleep(time.Millisecond * 1000)
		}
	}()*/
}
