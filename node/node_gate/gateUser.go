package node_gate

import (
	"container/list"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/sanguo/cluster"
	"github.com/sniperHW/sanguo/cluster/addr"
	"github.com/sniperHW/sanguo/node/node_gate/codec"
	ss_message "github.com/sniperHW/sanguo/protocol/ss/ssmessage"
	"math/rand"
	"reflect"
	"sync"
	"sync/atomic"
)

const (
	maxSendQueueBytes      int64 = 10 * 1024 * 1024       //单玩家待发送大小10M
	maxTotalSendQueueBytes int64 = 1024 * 1024 * 1024 * 1 //整个进程1G
	status_login                 = 1
	status_ok                    = 2
	status_remove                = 3
	status_migration             = 4 //game对象正在执行迁移
)

var ErrSendQueueFull error = fmt.Errorf("sendQueue full")
var dispatcher map[string]func(kendynet.StreamSession, *codec.Message) = map[string]func(kendynet.StreamSession, *codec.Message){}
var gateMessage map[string]bool = map[string]bool{}
var totalSendQueueBytes int64
var idPool gateIDPool

type gateIDPool struct {
	pool *list.List
	mtx  sync.Mutex
}

func (this *gateIDPool) init() {
	this.pool = list.New()
	i := uint32(1)
	for ; i < uint32(0x0000FFFF); i++ {
		this.pool.PushBack(i)
	}
}

func (this *gateIDPool) get() uint32 {
	this.mtx.Lock()
	defer this.mtx.Unlock()
	if nil == this.pool {
		this.init()
	}
	if this.pool.Len() == 0 {
		return 0
	}
	e := this.pool.Front()
	id := e.Value
	this.pool.Remove(e)
	return id.(uint32)
}

func (this *gateIDPool) put(id uint32) {
	this.mtx.Lock()
	defer this.mtx.Unlock()
	this.pool.PushBack(id)
}

func getGateUserID() uint32 {
	return idPool.get() | (rand.Uint32() & 0xFFFF0000)
}

func putGateUserID(id uint32) {
	idPool.put(id & 0x0000FFFF)
}

func registerCliHandler(msg proto.Message, fn func(kendynet.StreamSession, *codec.Message)) {
	if nil == dispatcher {
		dispatcher = map[string]func(kendynet.StreamSession, *codec.Message){}
		gateMessage = map[string]bool{}
	}
	msgName := reflect.TypeOf(msg).String()
	dispatcher[msgName] = fn
	gateMessage[msgName] = true
}

func dispatchClientMsg(session kendynet.StreamSession, msg *codec.Message) {
	if nil != dispatcher {
		Debugln("dispatchClientMsg", msg.GetName())
		fn, ok := dispatcher[msg.GetName()]
		if ok {
			fn(session, msg)
		}
	}
}

type sendQueue struct {
	queue    [][]byte
	byteSize int64
}

func (this *sendQueue) append(bytes []byte) error {
	//Debugln(this.byteSize, len(bytes), totalSendQueueBytes)
	if this.byteSize+int64(len(bytes)) > maxSendQueueBytes ||
		atomic.LoadInt64(&totalSendQueueBytes)+int64(len(bytes)) > maxTotalSendQueueBytes {
		return ErrSendQueueFull
	} else {
		this.queue = append(this.queue, bytes)
		this.byteSize += int64(len(bytes))
		atomic.AddInt64(&totalSendQueueBytes, int64(len(bytes)))
		return nil
	}
}

type gateUser struct {
	id              uint32
	userID          string
	status          int
	game            addr.LogicAddr
	conn            kendynet.StreamSession
	downQueue       sendQueue //下行消息
	upQueue         sendQueue //上行消息
	mtx             sync.Mutex
	token           string
	t               *cluster.Timer
	holdGateSession bool
}

func (this *gateUser) onRemove() {
	if addr.LogicAddr(0) != this.game {
		//通告game服务gateUser被销毁
		cluster.PostMessage(this.game, &ss_message.GateUserDestroy{
			UserID:     proto.String(this.userID),
			GateUserID: proto.Uint32(this.id),
		})
	}

	if nil != this.t {
		cluster.UnregisterTimer(this.t)
		this.t = nil
	}
}

type gateUserMgr struct {
	mtx        sync.Mutex
	idUserMap  map[uint32]*gateUser
	uidUserMap map[string]*gateUser
}

var userMgr = gateUserMgr{
	idUserMap:  map[uint32]*gateUser{},
	uidUserMap: map[string]*gateUser{},
}

func GetUserByUserID(uid string) *gateUser {
	userMgr.mtx.Lock()
	defer userMgr.mtx.Unlock()
	return userMgr.uidUserMap[uid]
}

func GetUserByGateID(id uint32) *gateUser {
	userMgr.mtx.Lock()
	defer userMgr.mtx.Unlock()
	return userMgr.idUserMap[id]
}

func AddGateUser(u *gateUser) {
	userMgr.uidUserMap[u.userID] = u
	userMgr.idUserMap[u.id] = u
}

/*func remGateUserNoLock(u *gateUser) {
	delete(userMgr.idUserMap, u.id)
	delete(userMgr.uidUserMap, u.userID)
	u.mtx.Lock()
	u.onRemove()
	u.status = status_remove
	u.token = ""
	u.game = nil
	atomic.AddInt64(&totalSendQueueBytes, -u.downQueue.byteSize)
	atomic.AddInt64(&totalSendQueueBytes, -u.upQueue.byteSize)
	u.mtx.Unlock()
}*/

func remGateUser(u *gateUser) {
	userMgr.mtx.Lock()
	_, ok := userMgr.uidUserMap[u.userID]
	if !ok {
		userMgr.mtx.Unlock()
		return
	} else {
		delete(userMgr.idUserMap, u.id)
		delete(userMgr.uidUserMap, u.userID)
		userMgr.mtx.Unlock()
	}

	u.mtx.Lock()
	u.onRemove()
	u.status = status_remove
	u.token = ""
	u.game = addr.LogicAddr(0)
	atomic.AddInt64(&totalSendQueueBytes, -u.downQueue.byteSize)
	atomic.AddInt64(&totalSendQueueBytes, -u.upQueue.byteSize)
	u.mtx.Unlock()
}

func userCount() int32 {
	userMgr.mtx.Lock()
	defer userMgr.mtx.Unlock()
	return int32(len(userMgr.uidUserMap))
}
