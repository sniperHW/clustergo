package sanguo

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

	"github.com/sniperHW/netgo"
	"github.com/sniperHW/rpcgo"
	"github.com/sniperHW/sanguo/addr"
	"github.com/sniperHW/sanguo/codec/pb"
	"github.com/sniperHW/sanguo/codec/ss"
	"github.com/sniperHW/sanguo/discovery"
	"github.com/sniperHW/sanguo/pkg/crypto"
	"github.com/xtaci/smux"
	"google.golang.org/protobuf/proto"
)

var (
	ErrInvaildNode     = errors.New("invaild node")
	ErrDuplicateConn   = errors.New("duplicate node connection")
	ErrNetAddrMismatch = errors.New("net addr mismatch")
)

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

type Sanguo struct {
	localAddr   addr.Addr
	listener    net.Listener
	nodeCache   nodeCache
	rpcSvr      *rpcgo.Server
	rpcCli      *rpcgo.Client
	msgManager  msgManager
	startOnce   sync.Once
	stopOnce    sync.Once
	die         chan struct{}
	onNewStream atomic.Value //func(*smux.Stream)
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

func (s *Sanguo) OnNewStream(onNewStream func(*smux.Stream)) {
	s.onNewStream.Store(onNewStream)
}

func (s *Sanguo) OpenStream(peer addr.LogicAddr) (*smux.Stream, error) {
	if peer == s.localAddr.LogicAddr() {
		return nil, errors.New("cant't open stream to self")
	} else if n := s.getNodeByLogicAddr(peer); n != nil {
		return n.openStream(s)
	} else {
		return nil, errors.New("invaild peer")
	}
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
			n.sendMessage(context.TODO(), s, ss.NewMessage(to, s.localAddr.LogicAddr(), msg), time.Now().Add(time.Second))
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
			return s.rpcCli.Call(ctx, &rpcChannel{peer: to, node: n, sanguo: s}, method, arg, ret)
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
			return s.rpcCli.CallWithCallback(&rpcChannel{peer: to, node: n, sanguo: s}, deadline, method, arg, ret, cb)
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
	once := false
	s.stopOnce.Do(func() {
		once = true
	})
	if once {
		s.listener.Close()
		s.nodeCache.RLock()
		for _, v := range s.nodeCache.nodes {
			v.Lock()
			if v.socket != nil {
				v.socket.Close(nil)
			}
			v.Unlock()

			v.streamCli.Lock()
			if v.streamCli.session != nil {
				v.streamCli.session.Close()
				v.streamCli.session = nil
			}
			v.streamCli.Unlock()

		}
		s.nodeCache.RUnlock()
		close(s.die)
	}
}

func (s *Sanguo) Start(discoveryService discovery.Discovery, localAddr addr.LogicAddr) (err error) {
	once := false
	s.startOnce.Do(func() {
		once = true
	})
	if once {
		s.nodeCache.localAddr = localAddr

		if err = discoveryService.Subscribe(func(nodeinfo []discovery.Node) {
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
			}
		}
	}
	return err
}

func (s *Sanguo) Wait() {
	<-s.die
}

func (s *Sanguo) listenStream(session *smux.Session, onNewStream func(*smux.Stream)) {
	defer session.Close()
	for {
		session.SetDeadline(time.Now().Add(time.Second))
		if stream, err := session.AcceptStream(); err == nil {
			onNewStream(stream)
		} else if err == smux.ErrTimeout {
			select {
			case <-s.die:
				return
			default:
			}
		} else {
			session.Close()
			return
		}
	}
}

func (s *Sanguo) onNewConnection(conn net.Conn) (err error) {
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
		if node.dialing {
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
			node.onEstablish(s, conn)
			return nil
		}
	}
}

type SanguoOption struct {
	RPCCodec rpcgo.Codec
}

func newSanguo(o SanguoOption) *Sanguo {
	return &Sanguo{
		nodeCache: nodeCache{
			nodes:            map[addr.LogicAddr]*node{},
			nodeByType:       map[uint32][]*node{},
			harborsByCluster: map[uint32][]*node{},
			initC:            make(chan struct{}),
		},
		rpcSvr: rpcgo.NewServer(o.RPCCodec),
		rpcCli: rpcgo.NewClient(o.RPCCodec),
		msgManager: msgManager{
			msgHandlers: map[uint16]MsgHandler{},
		},
		die: make(chan struct{}),
	}
}

var defaultSanguo *Sanguo
var defaultOnce sync.Once
var defaultRPCCodec rpcgo.Codec = &PbCodec{}

func getDefault() *Sanguo {
	defaultOnce.Do(func() {
		defaultSanguo = newSanguo(SanguoOption{
			RPCCodec: defaultRPCCodec,
		})
	})
	return defaultSanguo
}

func SetRpcCodec(rpcCodec rpcgo.Codec) {
	defaultRPCCodec = rpcCodec
}

func Start(discovery discovery.Discovery, localAddr addr.LogicAddr) (err error) {
	return getDefault().Start(discovery, localAddr)
}

func GetAddrByType(tt uint32, n ...int) (addr addr.LogicAddr, err error) {
	return getDefault().GetAddrByType(tt, n...)
}

func Stop() {
	getDefault().Stop()
}

func Wait() {
	getDefault().Wait()
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

func OnNewStream(onNewStream func(*smux.Stream)) {
	getDefault().OnNewStream(onNewStream)
}

func OpenStream(peer addr.LogicAddr) (*smux.Stream, error) {
	return getDefault().OpenStream(peer)
}
