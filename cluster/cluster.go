package cluster

import (
	"fmt"
	"github.com/sniperHW/sanguo/cluster/addr"
	"github.com/sniperHW/sanguo/codec/pb"
	"github.com/sniperHW/sanguo/codec/ss"
	_ "github.com/sniperHW/sanguo/protocol/ss" //触发pb注册
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/event"
	"github.com/sniperHW/kendynet/rpc"
	listener "github.com/sniperHW/kendynet/socket/listener/tcp"
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

//向类型为tt的本cluster节点广播
func Brocast(tt uint32, msg proto.Message, exceptSelf ...bool) {
	defer mtx.Unlock()
	mtx.Lock()
	if ttmap, ok := ttEndPointMap[tt]; ok {
		for _, v := range ttmap.endPoints {
			if len(exceptSelf) == 0 || v.addr.Logic != selfAddr.Logic {
				postToEndPoint(v, msg)
			}
		}
	}
}

//向本cluster内所有节点广播
func BrocastToAll(msg proto.Message, exceptTT ...uint32) {
	defer mtx.Unlock()
	mtx.Lock()
	exceptType := uint32(0)
	if len(exceptTT) > 0 {
		exceptType = exceptTT[0]
	}

	for tt, v := range ttEndPointMap {
		if tt != exceptType {
			for _, vv := range v.endPoints {
				postToEndPoint(vv, msg)
			}
		}
	}
}

func postToEndPoint(end *endPoint, msg proto.Message) {
	end.mtx.Lock()
	defer end.mtx.Unlock()

	//Infoln("postToEndPoint", end.addr.Logic.String())

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
func Start(center_addr []string, selfAddr_ addr.Addr, export ...uint32) error {

	if !atomic.CompareAndSwapInt32(&started, 0, 1) {
		return fmt.Errorf("service already started")
	}

	selfAddr = selfAddr_

	server, err := listener.New("tcp4", selfAddr.Net.String())
	if server != nil {

		connectCenter(center_addr, selfAddr, export...)

		go func() {
			err := server.Serve(func(session kendynet.StreamSession) {
				go func() {
					nodeInfo, err := auth(session)
					if nil != err {
						session.Close(err.Error(), 0)
					} else {
						err = onEstablishServer(nodeInfo, session)
						if nil != err {
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

//随机获取一个类型为tt的节点id
func Random(tt uint32) (addr.LogicAddr, error) {
	defer mtx.Unlock()
	mtx.Lock()

	//优先从本集群查找
	if ttmap, ok := ttEndPointMap[tt]; ok {
		addr_, err := ttmap.random()
		if nil == err {
			return addr_, err
		}
	}

	//从forginService查找
	if smap, ok := ttForignServiceMap[tt]; ok {
		return smap.random()
	}

	return addr.LogicAddr(0), fmt.Errorf("invaild tt")
}

func init() {

	pb.Register("ss", &Heartbeat{}, 1)

	rpcServer = rpc.NewRPCServer(&decoder{}, &encoder{})

	rpcClient = rpc.NewClient(&decoder{}, &encoder{})

	centerInit()

	go func() {
		queue.Run()
	}()

}
