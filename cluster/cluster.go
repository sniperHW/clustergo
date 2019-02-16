package cluster

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/event"
	"github.com/sniperHW/kendynet/rpc"
	listener "github.com/sniperHW/kendynet/socket/listener/tcp"
	"reflect"
	"sanguo/cluster/addr"
	"sanguo/codec/pb"
	"sanguo/codec/ss"
	_ "sanguo/protocol/ss" //触发pb注册
	"sync"
	"sync/atomic"
	"time"
)

var selfAddr addr.Addr
var started int32
var mtx sync.Mutex
var queue *event.EventQueue = event.NewEventQueue()
var idEndPointMap map[addr.LogicAddr]*endPoint = map[addr.LogicAddr]*endPoint{}
var ttEndPointMap map[uint32]*typeEndPointMap = map[uint32]*typeEndPointMap{}

type rpcCall struct {
	arg      interface{}
	deadline time.Time
	to       addr.LogicAddr
	cb       rpc.RPCResponseHandler
}

func Brocast(tt uint32, msg proto.Message) {
	defer mtx.Unlock()
	mtx.Lock()
	if ttmap, ok := ttEndPointMap[tt]; ok {
		for _, v := range ttmap.endPoints {
			//Debugln("Brocast to", v.addr.Logic.String())
			postToEndPoint(v, msg)
		}
	}
}

func postToEndPoint(end *endPoint, msg proto.Message) {
	end.mtx.Lock()
	defer end.mtx.Unlock()

	if nil != end.conn {
		err := end.conn.session.Send(msg)
		if nil != err {
			//记录日志
			Errorln("Send error:", err.Error(), reflect.TypeOf(msg).String())
		}
	} else {
		end.pendingMsg = append(end.pendingMsg, msg)
		//尝试与对端建立连接
		dial(end)
	}
}

/*
*  异步投递
 */
func PostMessage(peer addr.LogicAddr, msg proto.Message) {

	if atomic.LoadInt32(&started) == 0 {
		panic("cluster not started")
	}

	endPoint := getEndPoint(peer)

	var msg_ interface{}
	if nil == endPoint {
		if peer.Group() == selfAddr.Logic.Group() {
			//记录日志
			Errorf("PostMessage %s not found", peer.String())
			return
		} else {
			//不同服务器组，需要通告Harbor转发
			harbor := getHarbor()
			if nil == harbor {
				Errorf("Post cross Group message failed,no harbor", peer.String())
				return
			} else {
				endPoint = harbor
				msg_ = ss.NewMessage("", msg, peer, selfAddr.Logic)
			}
		}
	} else {
		msg_ = msg
	}

	endPoint.mtx.Lock()
	defer endPoint.mtx.Unlock()

	//Infoln("PostMessage", peer.String(), endPoint.conn)

	if nil != endPoint.conn {
		err := endPoint.conn.session.Send(msg_)
		if nil != err {
			//记录日志
			Errorln("Send error:", err.Error(), reflect.TypeOf(msg_).String())
		}
	} else {
		endPoint.pendingMsg = append(endPoint.pendingMsg, msg_)
		//尝试与对端建立连接
		dial(endPoint)
	}
}

func SelfAddr() addr.Addr {
	return selfAddr
}

/*
*  启动服务
 */
func Start(center_addr []string, selfAddr_ addr.Addr) error {

	if !atomic.CompareAndSwapInt32(&started, 0, 1) {
		return fmt.Errorf("service already started")
	}

	selfAddr = selfAddr_

	server, err := listener.New("tcp4", selfAddr.Net.String())
	if server != nil {

		connectCenter(center_addr, selfAddr)

		go func() {
			err := server.Serve(func(session kendynet.StreamSession) {
				go func() {
					peerLogicAddr, err := auth(session)
					//Debugln("auth", peerLogicAddr, err)
					if nil != err {
						session.Close(err.Error(), 0)
					} else {
						err = onEstablishServer(peerLogicAddr, session)
						if nil != err {
							Debugln("onEstablishServer", err)
							session.Close(err.Error(), 0)
						}
					}
				}()
			})
			if nil != err {
				Errorf("server.Start() failed:%s\n", err.Error())
			}

		}()

		return nil
	} else {
		return err
	}
}

func GetEventQueue() *event.EventQueue {
	return queue
}

/*
*  将一个闭包投递到队列中执行，args为传递给闭包的参数
 */
func PostTask(function interface{}, args ...interface{}) {
	queue.PostNoWait(function, args...)
}

func init() {

	pb.Register("ss", &Heartbeat{}, 1)

	rpcServer = rpc.NewRPCServer(&decoder{}, &encoder{})

	rpcClient = rpc.NewClient(&decoder{}, &encoder{})

	centerInit()

	go func() {
		for {
			queue.PostNoWait(TickTimer)
			time.Sleep(time.Millisecond * 10)
		}
	}()

	go func() {
		queue.Run()
	}()

}
