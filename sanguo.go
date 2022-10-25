package sanguo

import (
	"context"
	"errors"
	"fmt"
	"net"
	"runtime"
	"sync"
	"time"

	"github.com/sniperHW/netgo"
	"github.com/sniperHW/rpcgo"
	"github.com/sniperHW/sanguo/addr"
	"github.com/sniperHW/sanguo/codec/pb"
	"github.com/sniperHW/sanguo/codec/ss"
	"github.com/sniperHW/sanguo/discovery"
	"github.com/sniperHW/sanguo/log"
	"google.golang.org/protobuf/proto"
)

var (
	ErrInvaildNode   = errors.New("invaild node")
	ErrDuplicateConn = errors.New("duplicate node connection")
	ErrDial          = errors.New("dial failed")
)

var logger log.Logger

func InitLogger(l log.Logger) {
	logger = l
}

type MsgHandler func(addr.LogicAddr, proto.Message)

type msgManager struct {
	sync.RWMutex
	msgHandlers map[uint16]MsgHandler
}

func (m *msgManager) register(cmd uint16, handler MsgHandler) {
	m.Lock()
	defer m.Unlock()

	if nil == handler {
		logger.Errorf("Register %d failed: handler is nil\n", cmd)
		return
	}
	_, ok := m.msgHandlers[cmd]
	if ok {
		logger.Errorf("Register %d failed: duplicate handler\n", cmd)
		return
	}

	m.msgHandlers[cmd] = handler
}

func pcall(handler MsgHandler, from addr.LogicAddr, cmd uint16, msg proto.Message) {
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 65535)
			l := runtime.Stack(buf, false)
			logger.Errorf("error on Dispatch:%d\nstack:%v,%s\n", cmd, r, buf[:l])
		}
	}()
	handler(from, msg)
}

func (m *msgManager) dispatch(from addr.LogicAddr, cmd uint16, msg proto.Message) {
	m.RLock()
	handler, ok := m.msgHandlers[cmd]
	m.RUnlock()
	if ok {
		pcall(handler, from, cmd, msg)
	} else {
		logger.Errorf("unkonw cmd:%d\n", cmd)
	}
}

type Sanguo struct {
	localAddr  addr.Addr
	listener   net.Listener
	nodeCache  nodeCache
	rpcSvr     *rpcgo.Server
	rpcCli     *rpcgo.Client
	msgManager msgManager
	startOnce  sync.Once
	stopOnce   sync.Once
	die        chan struct{}
}

// 根据目标逻辑地址返回一个node用于发送消息
func (s *Sanguo) getNodeByLogicAddr(to addr.LogicAddr) (n *node) {
	if to.Cluster() == s.localAddr.LogicAddr().Cluster() {
		//同cluster内发送消息
		n = s.nodeCache.getNodeByLogicAddr(to)
	} else {
		//不同cluster需要通过harbor转发
		var harbor *node
		if s.localAddr.LogicAddr().Type() == addr.HarbarType {
			//当前为harbor节点，选择harbor集群中与to在同一个cluster的harbor节点负责转发
			harbor = s.nodeCache.getHarborByCluster(to.Cluster(), to)
		} else {
			//当前节点非harbor节点，从cluster内选择一个harbor节点负责转发
			harbor = s.nodeCache.getHarborByCluster(s.localAddr.LogicAddr().Cluster(), to)
		}
		n = harbor
	}
	return n
}

func (s *Sanguo) GetAddrByType(tt uint32, n ...int) (addr addr.LogicAddr, err error) {
	var num int
	if len(n) > 0 {
		num = n[0]
	}

	if node := s.nodeCache.getNodeByType(tt, num); node != nil {
		addr = node.addr.LogicAddr()
	} else {
		err = errors.New("no available node")
	}
	return addr, err
}

func (s *Sanguo) RegisterMessageHandler(msg proto.Message, handler MsgHandler) {
	if cmd := pb.GetCmd(ss.Namespace, msg); cmd != 0 {
		s.msgManager.register(uint16(cmd), handler)
	}
}

func (s *Sanguo) RegisterRPC(name string, method interface{}) error {
	return s.rpcSvr.Register(name, method)
}

func (s *Sanguo) SendMessage(to addr.LogicAddr, msg proto.Message) {
	if to == s.localAddr.LogicAddr() {
		s.dispatchMessage(to, uint16(pb.GetCmd(ss.Namespace, msg)), msg)
	} else {
		if n := s.getNodeByLogicAddr(to); n != nil {
			n.sendMessage(context.TODO(), ss.NewMessage(to, s.localAddr.LogicAddr(), msg), time.Now().Add(time.Second))
		} else {
			logger.Debugf("target: not found", to.String())
		}
	}
}

func (s *Sanguo) Call(ctx context.Context, to addr.LogicAddr, method string, arg interface{}, ret interface{}) error {
	if to == s.localAddr.LogicAddr() {
		return s.rpcCli.Call(ctx, &selfChannel{sanguo: s}, method, arg, ret)
	} else {
		if n := s.getNodeByLogicAddr(to); n != nil {
			return s.rpcCli.Call(ctx, &rpcChannel{peer: to, node: n}, method, arg, ret)
		} else {
			return errors.New("call failed")
		}
	}
}

func (s *Sanguo) CallWithCallback(to addr.LogicAddr, deadline time.Time, method string, arg interface{}, ret interface{}, cb func(interface{}, error)) func() bool {
	if to == s.localAddr.LogicAddr() {
		return s.rpcCli.CallWithCallback(&selfChannel{sanguo: s}, deadline, method, arg, ret, cb)
	} else {
		if n := s.getNodeByLogicAddr(to); n != nil {
			return s.rpcCli.CallWithCallback(&rpcChannel{peer: to, node: n}, deadline, method, arg, ret, cb)
		} else {
			go cb(nil, errors.New("call failed"))
			return nil
		}
	}
}

func (s *Sanguo) dispatchMessage(from addr.LogicAddr, cmd uint16, msg proto.Message) {
	s.msgManager.dispatch(from, cmd, msg)
}

func (s *Sanguo) Stop() {
	s.stopOnce.Do(func() {
		s.listener.Close()
		close(s.die)
	})
}

func (s *Sanguo) Start(discoveryService discovery.Discovery, localAddr addr.LogicAddr) (err error) {
	s.startOnce.Do(func() {
		s.nodeCache.sanguo = s
		s.nodeCache.localAddr = localAddr
		s.nodeCache.onSelfRemove = s.Stop //当自己从配置中移除调用Stop

		var nodeInfo []discovery.Node
		if nodeInfo, err = discoveryService.LoadNodeInfo(); err == nil {
			s.nodeCache.onNodeInfoUpdate(nodeInfo)
		}
		if n := s.nodeCache.getNodeByLogicAddr(localAddr); n == nil {
			//当前节点在配置中找不到
			err = fmt.Errorf("%s not in config", localAddr.String())
		} else {
			s.localAddr = n.addr
			var serve func()
			s.listener, serve, err = netgo.ListenTCP("tcp", s.localAddr.NetAddr().String(), func(conn *net.TCPConn) {
				logger.Debugf("%s %s new connection", s.localAddr.LogicAddr().String(), s.localAddr.NetAddr().String())
				go func() {
					if err := s.auth(conn); nil != err {
						logger.Infof("auth error %s self %s", err.Error(), localAddr.String())
						conn.Close()
					}
				}()
			})
			if err == nil {
				//订阅更新
				discoveryService.Subscribe(s.nodeCache.onNodeInfoUpdate)
				logger.Debugf("%s serve on:%s", localAddr.String(), s.localAddr.NetAddr().String())
				go serve()
			}
		}
	})
	return err
}

func (s *Sanguo) Wait() {
	<-s.die
}

func NewSanguo() *Sanguo {
	codec := &JsonCodec{}
	return &Sanguo{
		nodeCache: nodeCache{
			nodes:            map[addr.LogicAddr]*node{},
			nodeByType:       map[uint32][]*node{},
			harborsByCluster: map[uint32][]*node{},
		},
		rpcSvr: rpcgo.NewServer(codec),
		rpcCli: rpcgo.NewClient(codec),
		msgManager: msgManager{
			msgHandlers: map[uint16]MsgHandler{},
		},
		die: make(chan struct{}),
	}
}

var defaultSanguo *Sanguo
var defaultOnce sync.Once

func getDefault() *Sanguo {
	defaultOnce.Do(func() {
		defaultSanguo = NewSanguo()
	})
	return defaultSanguo
}

func Start(discovery discovery.Discovery, localAddr addr.LogicAddr) (err error) {
	return getDefault().Start(discovery, localAddr)
}

func Stop() {
	getDefault().Stop()
}

func RegisterMessageHandler(msg proto.Message, handler MsgHandler) {
	getDefault().RegisterMessageHandler(msg, handler)
}

func RegisterRPC(name string, method interface{}) error {
	return getDefault().RegisterRPC(name, method)
}

func SendMessage(to addr.LogicAddr, msg proto.Message) {
	getDefault().SendMessage(to, msg)
}

func Call(ctx context.Context, to addr.LogicAddr, method string, arg interface{}, ret interface{}) error {
	return getDefault().Call(ctx, to, method, arg, ret)
}

func CallWithCallback(to addr.LogicAddr, deadline time.Time, method string, arg interface{}, ret interface{}, cb func(interface{}, error)) func() bool {
	return getDefault().CallWithCallback(to, deadline, method, arg, ret, cb)
}
