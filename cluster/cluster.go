package cluster

import (
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/event"
	"github.com/sniperHW/kendynet/rpc"
	listener "github.com/sniperHW/kendynet/socket/listener/tcp"
	"github.com/sniperHW/sanguo/cluster/addr"
	"github.com/sniperHW/sanguo/codec/pb"
	"github.com/sniperHW/sanguo/codec/ss"
	_ "github.com/sniperHW/sanguo/protocol/ss" //触发pb注册
	"github.com/sniperHW/sanguo/util"
	"reflect"
	"sync/atomic"
	"time"
)

type clusterState struct {
	selfAddr addr.Addr
	started  int32
	stoped   int32
}

type rpcCall struct {
	arg      interface{}
	deadline time.Time
	to       addr.LogicAddr
	cb       rpc.RPCResponseHandler
}

/*
 *   如果服务要下线才将用Stop(true)调用
 *   如果只是重启更新不需要带参数
 */
func (this *Cluster) Stop(stopFunc func(), sendRemoveNode ...bool) {
	if atomic.CompareAndSwapInt32(&this.serverState.stoped, 0, 1) {
		if nil != stopFunc {
			stopFunc()
		}

		util.WaitCondition(nil, func() bool {
			return this.rpcMgr.server.PendingCount() == 0 && this.rpcMgr.client.PendingCount() == 0
		})
		this.queue.Close()

		this.l.Close()
		if len(sendRemoveNode) > 0 && sendRemoveNode[0] {
			this.centerClient.Close(true)
		} else {
			this.centerClient.Close(false)
		}
	}
}

func (this *Cluster) IsStoped() bool {
	return atomic.LoadInt32(&this.serverState.stoped) == 1
}

func (this *Cluster) postToEndPoint(end *endPoint, msg proto.Message) {
	end.Lock()
	defer end.Unlock()

	if nil != end.session {
		err := end.session.Send(msg)
		if nil != err {
			//记录日志
			logger.Errorln("Send error:", err.Error(), reflect.TypeOf(msg).String())
		} else {
			end.lastActive = time.Now()
		}
	} else {
		end.pendingMsg = append(end.pendingMsg, msg)
		//尝试与对端建立连接
		this.dial(end, 0)
	}
}

//向类型为tt的本cluster节点广播
func (this *Cluster) Brocast(tt uint32, msg proto.Message, exceptSelf ...bool) {
	this.serviceMgr.RLock()
	defer this.serviceMgr.RUnlock()
	if ttmap, ok := this.serviceMgr.ttEndPointMap[tt]; ok {
		for _, v := range ttmap.endPoints {
			if len(exceptSelf) == 0 || v.addr.Logic != this.serverState.selfAddr.Logic {
				this.postToEndPoint(v, msg)
			}
		}
	}
}

//向本cluster内所有节点广播
func (this *Cluster) BrocastToAll(msg proto.Message, exceptTT ...uint32) {
	this.serviceMgr.RLock()
	defer this.serviceMgr.RUnlock()
	exceptType := uint32(0)
	if len(exceptTT) > 0 {
		exceptType = exceptTT[0]
	}

	for tt, v := range this.serviceMgr.ttEndPointMap {
		if tt != exceptType {
			for _, vv := range v.endPoints {
				this.postToEndPoint(vv, msg)
			}
		}
	}
}

/*
*  异步投递
 */
func (this *Cluster) PostMessage(peer addr.LogicAddr, msg proto.Message) {

	if atomic.LoadInt32(&this.serverState.started) == 0 {
		panic("cluster not started")
	}

	if peer.Empty() {
		return
	}

	endPoint := this.serviceMgr.getEndPoint(peer)

	var msg_ interface{}
	if nil == endPoint {
		if peer.Group() == this.serverState.selfAddr.Logic.Group() {
			//记录日志
			logger.Errorf("PostMessage %s not found", peer.String())
			return
		} else {
			//不同服务器组，需要通告Harbor转发
			harbor := this.serviceMgr.getHarbor(peer)
			if nil == harbor {
				logger.Errorf("Post cross Group message failed,no harbor", peer.String())
				return
			} else {
				endPoint = harbor
				msg_ = ss.NewMessage(msg, peer, this.serverState.selfAddr.Logic)
			}
		}
	} else {
		msg_ = msg
	}

	endPoint.Lock()
	defer endPoint.Unlock()

	logger.Infoln("PostMessage", peer.String())

	if nil != endPoint.session {
		err := endPoint.session.Send(msg_)
		if nil != err {
			//记录日志
			logger.Errorln("Send error:", err.Error(), reflect.TypeOf(msg_).String())
		} else {
			endPoint.lastActive = time.Now()
		}
	} else {
		endPoint.pendingMsg = append(endPoint.pendingMsg, msg_)
		//尝试与对端建立连接
		this.dial(endPoint, 0)
	}
}

func (this *Cluster) SelfAddr() addr.Addr {
	return this.serverState.selfAddr
}

/*
*  启动服务
 */
func (this *Cluster) Start(center_addr []string, selfAddr addr.Addr, export ...bool) error {

	if !atomic.CompareAndSwapInt32(&this.serverState.started, 0, 1) {
		return ERR_STARTED
	}

	if selfAddr.Logic.Server() == uint32(0) {
		return ERR_SERVERADDR_ZERO
	}

	this.serverState.selfAddr = selfAddr

	server, err := listener.New("tcp", this.serverState.selfAddr.Net.String())
	if server != nil {

		this.l = server
		go func() {
			this.queue.Run()
		}()

		this.serviceMgr.init()
		this.centerInit(export...)
		this.connectCenter(center_addr)

		go func() {
			err := server.Serve(func(session kendynet.StreamSession) {
				logger.Infoln("on new client", session.LocalAddr(), session.RemoteAddr())
				go func() {
					e, err := this.auth(session)
					if nil != err {
						logger.Infoln("auth error", err.Error(), "self", this.serverState.selfAddr.Logic.String())
						session.Close(err.Error(), 0)
					} else {
						err = this.onEstablishServer(e, session)
						if nil != err {
							logger.Infoln("onEstablishServer error", err.Error())
							session.Close(err.Error(), 0)
						}
					}
				}()
			})
			if nil != err {
				logger.Errorf("server.Start() failed:%s\n", err.Error())
			}

		}()

		return nil
	} else {
		return err
	}
}

func (this *Cluster) GetEventQueue() *event.EventQueue {
	return this.queue
}

/*
*  将一个闭包投递到队列中执行，args为传递给闭包的参数
 */
func (this *Cluster) PostTask(function interface{}, args ...interface{}) {
	this.queue.PostNoWait(function, args...)
}

func (this *Cluster) Mod(tt uint32, num int) (addr.LogicAddr, error) {
	this.serviceMgr.RLock()
	defer this.serviceMgr.RUnlock()

	//优先从本集群查找
	if ttmap, ok := this.serviceMgr.ttEndPointMap[tt]; ok {
		addr_, err := ttmap.mod(num)
		if nil == err {
			return addr_, err
		}
	}

	//从forginService查找
	if smap, ok := this.serviceMgr.ttForignServiceMap[tt]; ok {
		return smap.mod(num)
	}

	return addr.LogicAddr(0), ERR_NO_AVAILABLE_SERVICE
}

//随机获取一个类型为tt的节点id
func (this *Cluster) Random(tt uint32) (addr.LogicAddr, error) {
	this.serviceMgr.RLock()
	defer this.serviceMgr.RUnlock()

	//优先从本集群查找
	if ttmap, ok := this.serviceMgr.ttEndPointMap[tt]; ok {
		addr_, err := ttmap.random()
		if nil == err {
			return addr_, err
		}
	}

	//从forginService查找
	if smap, ok := this.serviceMgr.ttForignServiceMap[tt]; ok {
		return smap.random()
	}

	return addr.LogicAddr(0), ERR_NO_AVAILABLE_SERVICE
}

func (this *Cluster) Select(tt uint32) ([]addr.LogicAddr, error) {
	this.serviceMgr.RLock()
	defer this.serviceMgr.RUnlock()

	ttmap := this.serviceMgr.ttEndPointMap[tt]
	if ttmap == nil {
		return nil, ERR_NO_AVAILABLE_SERVICE
	}

	if len(ttmap.endPoints) == 0 {
		return nil, ERR_NO_AVAILABLE_SERVICE
	}

	addrs := make([]addr.LogicAddr, 0, len(ttmap.endPoints))
	for _, ep := range ttmap.endPoints {
		addrs = append(addrs, ep.addr.Logic)
	}

	return addrs, nil
}

func init() {
	pb.Register("ss", &Heartbeat{}, 1)
}
