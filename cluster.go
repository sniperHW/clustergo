package clustergo

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sniperHW/clustergo/addr"
	"github.com/sniperHW/clustergo/codec/pb"
	"github.com/sniperHW/clustergo/codec/ss"
	"github.com/sniperHW/clustergo/discovery"
	"github.com/sniperHW/clustergo/pkg/crypto"
	"github.com/sniperHW/netgo"
	"github.com/sniperHW/rpcgo"
	"github.com/xtaci/smux"
	"google.golang.org/protobuf/proto"
)

var (
	ErrInvaildNode     = errors.New("invaild node")
	ErrDuplicateConn   = errors.New("duplicate node connection")
	ErrNetAddrMismatch = errors.New("net addr mismatch")
)

var RPCCodec rpcgo.Codec = PbCodec{}

var cecret_key []byte = []byte("sanguo_2022")

type loginReq struct {
	LogicAddr uint32 `json:"LogicAddr,omitempty"`
	NetAddr   string `json:"NetAddr,omitempty"`
	IsStream  bool   `json:"IsStream,omitempty"`
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

type Node struct {
	localAddr    addr.Addr
	listener     net.Listener
	nodeCache    nodeCache
	rpcSvr       *rpcgo.Server
	rpcCli       *rpcgo.Client
	msgManager   msgManager
	startOnce    sync.Once
	stopOnce     sync.Once
	die          chan struct{}
	started      chan struct{}
	smuxSessions sync.Map
	onNewStream  atomic.Value
}

// 根据目标逻辑地址返回一个node用于发送消息
func (s *Node) getNodeByLogicAddr(to addr.LogicAddr) (n *node) {
	if to.Cluster() == s.localAddr.LogicAddr().Cluster() {
		//同cluster内发送消息
		n = s.nodeCache.getNodeByLogicAddr(to)
	} else {
		//不同cluster需要通过harbor转发
		var harbor *node
		if s.localAddr.LogicAddr().Type() == addr.HarbarType {
			//当前为harbor节点，选择harbor集群中与to在同一个cluster的harbor节点负责转发
			harbor = s.nodeCache.getHarbor(to.Cluster(), to)
		} else {
			//当前节点非harbor节点，从cluster内选择一个harbor节点负责转发
			harbor = s.nodeCache.getHarbor(s.localAddr.LogicAddr().Cluster(), to)
		}
		n = harbor
	}
	return n
}

func (s *Node) StartSmuxServer(onNewStream func(*smux.Stream)) error {
	if s.onNewStream.CompareAndSwap(nil, onNewStream) {
		return nil
	} else {
		return errors.New("started")
	}
}

func (s *Node) OpenStream(peer addr.LogicAddr) (*smux.Stream, error) {
	select {
	case <-s.die:
		return nil, errors.New("server die")
	case <-s.started:
	default:
		return nil, errors.New("server not start")
	}

	if peer == s.localAddr.LogicAddr() {
		return nil, errors.New("cant't open stream to self")
	} else if n := s.getNodeByLogicAddr(peer); n != nil {
		return n.openStream(s)
	} else {
		return nil, errors.New("invaild peer")
	}
}

func (s *Node) GetAddrByType(tt uint32, n ...int) (addr addr.LogicAddr, err error) {
	select {
	case <-s.die:
		err = errors.New("server die")
		return
	case <-s.started:
	default:
		err = errors.New("server not start")
		return
	}

	var num int
	if len(n) > 0 {
		num = n[0]
	}

	if node := s.nodeCache.getNormalNode(tt, num); node != nil {
		addr = node.addr.LogicAddr()
	} else {
		err = errors.New("no available node")
	}
	return addr, err
}

func (s *Node) RegisterMessageHandler(msg proto.Message, handler MsgHandler) {
	if cmd := pb.GetCmd(ss.Namespace, msg); cmd != 0 {
		s.msgManager.register(uint16(cmd), handler)
	}
}

func (s *Node) RegisterRPC(name string, method interface{}) error {
	return s.rpcSvr.Register(name, method)
}

func (s *Node) SendMessage(to addr.LogicAddr, msg proto.Message) error {
	select {
	case <-s.die:
		return errors.New("server die")
	case <-s.started:
	default:
		return errors.New("server not start")
	}
	if to == s.localAddr.LogicAddr() {
		s.dispatchMessage(to, uint16(pb.GetCmd(ss.Namespace, msg)), msg)
	} else {
		if n := s.getNodeByLogicAddr(to); n != nil {
			n.sendMessage(context.TODO(), s, ss.NewMessage(to, s.localAddr.LogicAddr(), msg), time.Now().Add(time.Second))
		} else {
			return fmt.Errorf("target:%s not found", to.String())
		}
	}
	return nil
}

func (s *Node) Call(ctx context.Context, to addr.LogicAddr, method string, arg interface{}, ret interface{}) error {
	select {
	case <-s.die:
		return rpcgo.NewError(rpcgo.ErrOther, "server die")
	case <-s.started:
	default:
		return rpcgo.NewError(rpcgo.ErrOther, "server not start")
	}
	if to == s.localAddr.LogicAddr() {
		return s.rpcCli.Call(ctx, &selfChannel{self: s}, method, arg, ret)
	} else {
		if n := s.getNodeByLogicAddr(to); n != nil {
			return s.rpcCli.Call(ctx, &rpcChannel{peer: to, node: n, self: s}, method, arg, ret)
		} else {
			return rpcgo.NewError(rpcgo.ErrSend, fmt.Sprintf("target:%s not found", to.String()))
		}
	}
}

func (s *Node) CallWithCallback(to addr.LogicAddr, deadline time.Time, method string, arg interface{}, ret interface{}, cb func(interface{}, error)) (func() bool, error) {
	select {
	case <-s.die:
		return nil, rpcgo.NewError(rpcgo.ErrSend, "server die")
	case <-s.started:
	default:
		return nil, rpcgo.NewError(rpcgo.ErrSend, "server not start")
	}

	if to == s.localAddr.LogicAddr() {
		return s.rpcCli.CallWithCallback(&selfChannel{self: s}, deadline, method, arg, ret, cb)
	} else {
		if n := s.getNodeByLogicAddr(to); n != nil {
			return s.rpcCli.CallWithCallback(&rpcChannel{peer: to, node: n, self: s}, deadline, method, arg, ret, cb)
		} else {
			go cb(nil, rpcgo.NewError(rpcgo.ErrSend, fmt.Sprintf("target:%s not found", to.String())))
			return nil, nil
		}
	}
}

func (s *Node) dispatchMessage(from addr.LogicAddr, cmd uint16, msg proto.Message) {
	s.msgManager.dispatch(from, cmd, msg)
}

func (s *Node) Stop() error {
	select {
	case <-s.started:
	default:
		return errors.New("server not start")
	}

	once := false
	s.stopOnce.Do(func() {
		once = true
	})
	if once {
		s.listener.Close()
		s.nodeCache.close()
		s.smuxSessions.Range(func(key, _ interface{}) bool {
			key.(*smux.Session).Close()
			return true
		})
		close(s.die)
		return nil
	} else {
		return errors.New("stoped")
	}
}

func (s *Node) Start(discoveryService discovery.Discovery, localAddr addr.LogicAddr) (err error) {
	once := false
	s.startOnce.Do(func() {
		once = true
	})
	if once {
		s.nodeCache.localAddr = localAddr

		if err = discoveryService.Subscribe(func(nodeinfo discovery.DiscoveryInfo) {
			s.nodeCache.onNodeInfoUpdate(s, nodeinfo)
		}); err != nil {
			return err
		}

		s.nodeCache.waitInit()

		if n := s.nodeCache.getNodeByLogicAddr(localAddr); n == nil {
			//当前节点在配置中找不到
			err = fmt.Errorf("%s not in config", localAddr.String())
		} else {
			s.localAddr = n.addr
			var serve func()
			s.listener, serve, err = netgo.ListenTCP("tcp", s.localAddr.NetAddr().String(), func(conn *net.TCPConn) {
				go func() {
					if err := s.onNewConnection(conn); nil != err {
						logger.Infof("auth error %s self %s", err.Error(), localAddr.String())
						conn.Close()
					}
				}()
			})
			if err == nil {
				logger.Debugf("%s serve on:%s", localAddr.String(), s.localAddr.NetAddr().String())
				go serve()
				close(s.started)
			}
		}
	} else {
		err = errors.New("started")
	}
	return err
}

func (s *Node) Wait() error {
	select {
	case <-s.started:
	default:
		return errors.New("server not start")
	}
	<-s.die
	return nil
}

func (s *Node) listenStream(session *smux.Session, onNewStream func(*smux.Stream)) {
	defer func() {
		session.Close()
		s.smuxSessions.Delete(session)
	}()
	s.smuxSessions.Store(session, struct{}{})
	for {
		if stream, err := session.AcceptStream(); err == nil {
			onNewStream(stream)
		} else {
			return
		}
	}
}

func (s *Node) onNewConnection(conn net.Conn) (err error) {
	buff := make([]byte, 4)
	conn.SetReadDeadline(time.Now().Add(time.Second))
	defer conn.SetReadDeadline(time.Time{})

	_, err = io.ReadFull(conn, buff)
	if nil != err {
		return err
	}

	datasize := int(binary.BigEndian.Uint32(buff))

	buff = make([]byte, datasize)

	_, err = io.ReadFull(conn, buff)
	if nil != err {
		return err
	}

	if buff, err = crypto.AESCBCDecrypter(cecret_key, buff); nil != err {
		return err
	}

	var req loginReq

	if err = json.Unmarshal(buff, &req); nil != err {
		return err
	}

	node := s.nodeCache.getNodeByLogicAddr(addr.LogicAddr(req.LogicAddr))
	if node == nil {
		return ErrInvaildNode
	} else if node.addr.NetAddr().String() != req.NetAddr {
		return ErrNetAddrMismatch
	}

	resp := []byte{0, 0, 0, 0}
	binary.BigEndian.PutUint32(resp, 0)

	if req.IsStream {
		if onNewStream, ok := s.onNewStream.Load().(func(*smux.Stream)); !ok {
			return errors.New("onNewStream not set")
		} else {
			conn.SetWriteDeadline(time.Now().Add(time.Millisecond * 5))
			defer conn.SetWriteDeadline(time.Time{})

			if _, err = conn.Write(resp); err != nil {
				return err
			} else {
				if streamSvr, err := smux.Server(conn, nil); err != nil {
					return err
				} else {
					go s.listenStream(streamSvr, onNewStream)
					return nil
				}
			}
		}
	} else {
		node.Lock()
		defer node.Unlock()
		if node.pendingMsg.Len() != 0 {
			//当前节点同时正在向对端dialing,逻辑地址小的一方放弃接受连接
			if s.localAddr.LogicAddr() < node.addr.LogicAddr() {
				logger.Errorf("(self:%v) (other:%v) connectting simultaneously", s.localAddr.LogicAddr(), node.addr.LogicAddr())
				return errors.New("connectting simultaneously")
			}
		} else if nil != node.socket {
			return ErrDuplicateConn
		}

		conn.SetWriteDeadline(time.Now().Add(time.Millisecond * 5))
		defer conn.SetWriteDeadline(time.Time{})

		if _, err = conn.Write(resp); err != nil {
			return err
		} else {
			node.onEstablish(s, conn.(*net.TCPConn))
			return nil
		}
	}
}

func newNode(rpccodec rpcgo.Codec) *Node {
	return &Node{
		nodeCache: nodeCache{
			allnodes: map[addr.LogicAddr]*node{},
			nodes:    map[uint32][]*node{},
			harbors:  map[uint32][]*node{},
			initC:    make(chan struct{}),
		},
		rpcSvr: rpcgo.NewServer(rpccodec),
		rpcCli: rpcgo.NewClient(rpccodec),
		msgManager: msgManager{
			msgHandlers: map[uint16]MsgHandler{},
		},
		die:     make(chan struct{}),
		started: make(chan struct{}),
	}
}

var defaultInstance *Node
var defaultOnce sync.Once

func getDefault() *Node {
	defaultOnce.Do(func() {
		defaultInstance = newNode(RPCCodec)
	})
	return defaultInstance
}

func Start(discovery discovery.Discovery, localAddr addr.LogicAddr) (err error) {
	return getDefault().Start(discovery, localAddr)
}

func GetAddrByType(tt uint32, n ...int) (addr addr.LogicAddr, err error) {
	return getDefault().GetAddrByType(tt, n...)
}

func Stop() error {
	return getDefault().Stop()
}

func Wait() error {
	return getDefault().Wait()
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

func CallWithCallback(to addr.LogicAddr, deadline time.Time, method string, arg interface{}, ret interface{}, cb func(interface{}, error)) (func() bool, error) {
	return getDefault().CallWithCallback(to, deadline, method, arg, ret, cb)
}

func StartSmuxServer(onNewStream func(*smux.Stream)) {
	getDefault().StartSmuxServer(onNewStream)
}

func OpenStream(peer addr.LogicAddr) (*smux.Stream, error) {
	return getDefault().OpenStream(peer)
}
