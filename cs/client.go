package cs


import(
	codecs "sanguo/codec/cs"
	_ "sanguo/protocol/cs" //触发pb注册
	cs_proto "sanguo/protocol/cs/message"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/util"
	"github.com/sniperHW/kendynet/socket/stream_socket/tcp"
	"time"
	"sanguo/common"
	"fmt"
)

var (
	sessions   map[kendynet.StreamSession]int64
	queue     *util.BlockQueue
)

func DialTcp(peerAddr string,timeout time.Duration,dispatcher ClientDispatcher) {
	connector,_ := tcp.NewConnector("tcp",peerAddr)
	go func() {
		session,err := connector.Dial(timeout)
		if nil != err {
			dispatcher.OnConnectFailed(peerAddr,err)
		} else{
			queue.Add(func() {
				sessions[session] = time.Now().Unix() + (common.HeartBeat_Timeout/2)
				session.SetRecvTimeout(common.HeartBeat_Timeout * time.Second)
				session.SetReceiver(codecs.NewReceiver("sc"))
				session.SetEncoder(codecs.NewEncoder("cs"))
				session.SetCloseCallBack(func (sess kendynet.StreamSession, reason string) {
					queue.Add(func() {
						delete(sessions,sess)
					})
					dispatcher.OnClose(sess,reason)
				})
				dispatcher.OnEstablish(session)
				session.Start(func (event *kendynet.Event) {
					if event.EventType == kendynet.EventTypeError {
						event.Session.Close(event.Data.(error).Error(),0)
					} else {
						msg := event.Data.(*codecs.Message)
						switch msg.GetData().(type) {
		        			case *cs_proto.HeartbeatToC:
		        				fmt.Printf("on HeartbeatToC\n")
		        				break	
		        			default:
								dispatcher.Dispatch(session,msg)	
								break
						}				
					}
				})
			})
		}
	}()	
}


func init() {
	sessions   = make(map[kendynet.StreamSession]int64)
	queue      = util.NewBlockQueue()

	go func() {
		for {
			_,localList := queue.Get()
			for _ , task := range localList {
				task.(func())()
			}
		}		
	}()

	go func () {
		for {
			queue.Add(func(){
				now := time.Now().Unix()
				for k,v := range(sessions) {
					if now >= v {
						sessions[k] = now + common.HeartBeat_Timeout/2
						//发送心跳
						Heartbeat := &cs_proto.HeartbeatToS{}
						k.Send(codecs.NewMessage(0,Heartbeat))
					}
				} 
			})
			time.Sleep(time.Millisecond * 1000)
		}
	}()
}

