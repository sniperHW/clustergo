package clustergo

import (
	"container/list"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/sniperHW/clustergo/addr"
	"github.com/sniperHW/clustergo/codec/ss"
	"github.com/sniperHW/clustergo/membership"
	"github.com/sniperHW/clustergo/pkg/crypto"
	"github.com/sniperHW/netgo"
	"github.com/sniperHW/rpcgo"
	"github.com/xtaci/smux"
	"google.golang.org/protobuf/proto"
)

// 从配置系统获得的node视图缓存
type nodeCache struct {
	sync.RWMutex
	localAddr addr.LogicAddr
	allnodes  map[addr.LogicAddr]*node //所有的节点
	nodes     map[uint32][]*node       //普通avalabale节点，按type分组
	harbors   map[uint32][]*node       //harbor avalabale节点，按cluster分组
	initOnce  sync.Once
	initC     chan struct{}
}

func (cache *nodeCache) close() {
	cache.RLock()
	for _, v := range cache.allnodes {
		v.closeSocket()
	}
	cache.RUnlock()
}

func (cache *nodeCache) waitInit() {
	<-cache.initC
}

func (cache *nodeCache) addNode(n *node) {
	if nodeArray, ok := cache.nodes[n.addr.LogicAddr().Type()]; !ok {
		cache.nodes[n.addr.LogicAddr().Type()] = []*node{n}
	} else {
		cache.nodes[n.addr.LogicAddr().Type()] = append(nodeArray, n)
	}
}

func (cache *nodeCache) removeNode(n *node) {
	if nodeArray, ok := cache.nodes[n.addr.LogicAddr().Type()]; ok {
		for k, v := range nodeArray {
			if v == n {
				nodeArray[k] = nodeArray[len(nodeArray)-1]
				cache.nodes[n.addr.LogicAddr().Type()] = nodeArray[:len(nodeArray)-1]
				break
			}
		}
	}
}

func (cache *nodeCache) addHarbor(harbor *node) {
	if harborArray, ok := cache.harbors[harbor.addr.LogicAddr().Cluster()]; !ok {
		cache.harbors[harbor.addr.LogicAddr().Cluster()] = []*node{harbor}
	} else {
		cache.harbors[harbor.addr.LogicAddr().Cluster()] = append(harborArray, harbor)
	}
}

func (cache *nodeCache) removeHarbor(harbor *node) {
	if harborArray, ok := cache.harbors[harbor.addr.LogicAddr().Cluster()]; ok {
		for k, v := range harborArray {
			if v == harbor {
				harborArray[k] = harborArray[len(harborArray)-1]
				cache.harbors[harbor.addr.LogicAddr().Cluster()] = harborArray[:len(harborArray)-1]
				break
			}
		}
	}
}

func (cache *nodeCache) onNodeInfoUpdate(self *Node, nodeinfo membership.MemberInfo) {

	defer cache.initOnce.Do(func() {
		logger.Debug("init ok")
		close(cache.initC)
	})

	cache.Lock()
	defer cache.Unlock()

	for _, v := range nodeinfo.Add {
		if v.Export || v.Addr.LogicAddr().Cluster() == cache.localAddr.Cluster() ||
			(cache.localAddr.Type() == addr.HarbarType && v.Addr.LogicAddr().Type() == addr.HarbarType) {
			n := &node{
				addr:       v.Addr,
				pendingMsg: list.New(),
				available:  v.Available,
			}
			cache.allnodes[n.addr.LogicAddr()] = n
			if n.available {
				if n.addr.LogicAddr().Type() != addr.HarbarType {
					cache.addNode(n)
				} else {
					cache.addHarbor(n)
				}
			}
		}
	}

	for _, v := range nodeinfo.Remove {
		if v.Export || v.Addr.LogicAddr().Cluster() == cache.localAddr.Cluster() ||
			(cache.localAddr.Type() == addr.HarbarType && v.Addr.LogicAddr().Type() == addr.HarbarType) {
			if v.Addr.LogicAddr() == cache.localAddr {
				go self.Stop()
				return
			} else {
				n := cache.allnodes[v.Addr.LogicAddr()]
				delete(cache.allnodes, n.addr.LogicAddr())
				if n.available {
					if n.addr.LogicAddr().Type() != addr.HarbarType {
						cache.removeNode(n)
					} else {
						cache.removeHarbor(n)
					}
				}
				n.closeSocket()
			}
		}
	}

	for _, v := range nodeinfo.Update {
		if v.Export || v.Addr.LogicAddr().Cluster() == cache.localAddr.Cluster() ||
			(cache.localAddr.Type() == addr.HarbarType && v.Addr.LogicAddr().Type() == addr.HarbarType) {
			n := cache.allnodes[v.Addr.LogicAddr()]

			if v.Addr.NetAddr().String() != n.addr.NetAddr().String() {
				//网络地址发生变更
				if n.addr.LogicAddr() == cache.localAddr {
					go self.Stop()
					return
				} else {
					n.closeSocket()
					n.addr.UpdateNetAddr(v.Addr.NetAddr())
				}
			}

			if v.Available {
				if !n.available {
					n.available = true
					if n.addr.LogicAddr().Type() != addr.HarbarType {
						cache.addNode(n)
					} else {
						cache.addHarbor(n)
					}
				}
			} else {
				if n.available {
					n.available = false
					if n.addr.LogicAddr().Type() != addr.HarbarType {
						cache.removeNode(n)
					} else {
						cache.removeHarbor(n)
					}
				}
			}
		}
	}
}

func (cache *nodeCache) getHarbor(cluster uint32, m addr.LogicAddr) *node {
	cache.RLock()
	defer cache.RUnlock()
	if harbors, ok := cache.harbors[cluster]; !ok || len(harbors) == 0 {
		return nil
	} else {
		return harbors[int(m)%len(harbors)]
	}
}

func (cache *nodeCache) getNormalNode(tt uint32, n int) *node {
	cache.RLock()
	defer cache.RUnlock()
	if nodeArray, ok := cache.nodes[tt]; !ok || len(nodeArray) == 0 {
		return nil
	} else {
		if n == 0 {
			return nodeArray[int(rand.Int31())%len(nodeArray)]
		} else {
			return nodeArray[n%len(nodeArray)]
		}
	}
}

func (cache *nodeCache) getNodeByLogicAddr(logicAddr addr.LogicAddr) *node {
	cache.RLock()
	defer cache.RUnlock()
	return cache.allnodes[logicAddr]
}

type pendingMessage struct {
	message  interface{}
	deadline time.Time
	ctx      context.Context
}

type streamClient struct {
	sync.Mutex
	session *smux.Session
}

type node struct {
	sync.Mutex
	addr       addr.Addr
	socket     *netgo.AsynSocket
	pendingMsg *list.List
	available  bool
	streamCli  streamClient
}

func (n *node) login(self *Node, conn *net.TCPConn, isStream bool) error {

	j, err := json.Marshal(&loginReq{
		LogicAddr: uint32(self.localAddr.LogicAddr()),
		NetAddr:   self.localAddr.NetAddr().String(),
		IsStream:  isStream,
	})

	if nil != err {
		return err
	}

	if j, err = crypto.AESCBCEncrypt(cecret_key, j); nil != err {
		return err
	}

	b := make([]byte, 4+len(j))
	binary.BigEndian.PutUint32(b, uint32(len(j)))
	copy(b[4:], j)

	conn.SetWriteDeadline(time.Now().Add(time.Second))
	_, err = conn.Write(b)
	conn.SetWriteDeadline(time.Time{})

	if nil != err {
		return err
	} else {
		buffer := make([]byte, 4)
		conn.SetReadDeadline(time.Now().Add(time.Second))
		_, err = io.ReadFull(conn, buffer)
		conn.SetReadDeadline(time.Time{})
		if nil != err {
			return err
		}
	}
	return nil
}

func (n *node) onRelayMessage(self *Node, message *ss.RelayMessage) {
	logger.Debugf("onRelayMessage self:%s %s->%s", self.localAddr.LogicAddr().String(), message.From().String(), message.To().String())
	if nextNode := self.getNodeByLogicAddr(message.To()); nextNode != nil {
		logger.Debugf("nextNode %s", nextNode.addr.LogicAddr())
		nextNode.sendMessage(self, message, time.Now().Add(time.Second))
	} else if rpcReq := message.GetRpcRequest(); rpcReq != nil && !rpcReq.Oneway {
		//对于无法路由的rpc请求，返回错误响应
		if nextNode = self.getNodeByLogicAddr(message.From()); nextNode != nil {
			logger.Debugf(fmt.Sprintf("route message to target:%s failed", message.To().String()))
			nextNode.sendMessage(self, ss.NewMessage(message.From(), self.localAddr.LogicAddr(), &rpcgo.ResponseMsg{
				Seq: rpcReq.Seq,
				Err: rpcgo.NewError(rpcgo.ErrOther, fmt.Sprintf("route message to target:%s failed", message.To().String())),
			}), time.Now().Add(time.Second))
		}
	}
}

func (n *node) onMessage(ctx context.Context, self *Node, msg interface{}) {
	switch msg := msg.(type) {
	case *ss.Message:
		switch m := msg.Payload().(type) {
		case proto.Message:
			self.msgManager.dispatchProto(ctx, msg.From(), msg.Cmd(), m)
		case []byte:
			self.msgManager.dispatchBinary(ctx, msg.From(), msg.Cmd(), m)
		case *rpcgo.RequestMsg:
			self.rpcSvr.OnMessage(ctx, &rpcChannel{peer: msg.From(), node: n, self: self}, m)
		case *rpcgo.ResponseMsg:
			self.rpcCli.OnMessage(nil, m)
		}
	case *ss.RelayMessage:
		n.onRelayMessage(self, msg)
	}
}

func (n *node) onEstablish(self *Node, conn *net.TCPConn) {
	codec := ss.NewCodec(self.localAddr.LogicAddr())
	n.socket = netgo.NewAsynSocket(netgo.NewTcpSocket(conn, codec), netgo.AsynSocketOption{
		SendChanSize: SendChanSize,
		Codec:        codec,
		AutoRecv:     true,
		Context:      context.TODO(),
	})

	n.socket.SetPacketHandler(func(ctx context.Context, as *netgo.AsynSocket, packet interface{}) error {
		if self.getNodeByLogicAddr(n.addr.LogicAddr()) != n {
			return ErrInvaildNode
		} else {
			n.onMessage(ctx, self, packet)
			return nil
		}
	})

	n.socket.SetCloseCallback(func(as *netgo.AsynSocket, err error) {
		n.Lock()
		n.socket = nil
		n.Unlock()
	}).Recv()

	now := time.Now()

	for e := n.pendingMsg.Front(); e != nil; e = n.pendingMsg.Front() {
		msg := n.pendingMsg.Remove(e).(*pendingMessage)
		if msg.ctx != nil {
			select {
			case <-msg.ctx.Done():
			default:
				n.socket.SendWithContext(msg.ctx, msg.message)
			}
		} else if msg.deadline.IsZero() || now.Before(msg.deadline) {
			n.socket.Send(msg.message, msg.deadline)
		}
	}
}

// 移除ctx.Done或到达deadline的消息
func (n *node) removeFailedMsg(removeZero bool) {
	now := time.Now()
	e := n.pendingMsg.Front()
	for e != nil {
		m := e.Value.(*pendingMessage)
		remove := false

		if m.ctx != nil {
			select {
			case <-m.ctx.Done():
				remove = true
			default:
			}
		} else if (m.deadline.IsZero() && removeZero) || now.After(m.deadline) {
			remove = true
		}

		if remove {
			next := e.Next()
			n.pendingMsg.Remove(e)
			e = next
		} else {
			e = e.Next()
		}
	}
}

func (n *node) dialError(self *Node) {
	n.Lock()
	defer n.Unlock()
	if nil == n.socket {
		n.removeFailedMsg(false)
		if n.pendingMsg.Len() > 0 {
			time.AfterFunc(time.Second, func() {
				n.dial(self)
			})
		}
	}
}

func (n *node) dialOK(self *Node, conn *net.TCPConn) {
	n.Lock()
	defer n.Unlock()
	if n.socket != nil {
		//两段同时建立连接
		conn.Close()
	} else {
		n.onEstablish(self, conn)
	}
}

func (n *node) dial(self *Node) {
	dialer := &net.Dialer{}
	logger.Debugf("%s dial %s", self.localAddr.LogicAddr().String(), n.addr.LogicAddr().String())
	ok := false
	conn, err := dialer.Dial("tcp", n.addr.NetAddr().String())
	if err == nil {
		if err = n.login(self, conn.(*net.TCPConn), false); err != nil {
			conn.Close()
			conn = nil
		} else {
			ok = true
		}
	}

	select {
	case <-self.die:
		if conn != nil {
			conn.Close()
		}
	default:
		if ok {
			n.dialOK(self, conn.(*net.TCPConn))
		} else {
			n.dialError(self)
		}
	}
}

func (n *node) openStream(self *Node) (*smux.Stream, error) {
	n.streamCli.Lock()
	defer n.streamCli.Unlock()
	for {
		if n.streamCli.session != nil {
			if stream, err := n.streamCli.session.OpenStream(); err == nil {
				return stream, nil
			} else if err == smux.ErrGoAway {
				//stream id overflows, should start a new connection
				return nil, err
			} else {
				n.streamCli.session.Close()
				n.streamCli.session = nil
			}
		} else {
			dialer := &net.Dialer{}
			if conn, err := dialer.Dial("tcp", n.addr.NetAddr().String()); err != nil {
				return nil, err
			} else if err = n.login(self, conn.(*net.TCPConn), true); err != nil {
				conn.Close()
				return nil, err
			} else if session, err := smux.Client(conn, nil); err != nil {
				conn.Close()
				return nil, err
			} else {
				select {
				case <-self.die:
					session.Close()
					conn.Close()
					return nil, errors.New("server die")
				default:
				}
				n.streamCli.session = session
			}
		}
	}
}

func (n *node) sendMessage(self *Node, msg interface{}, deadline time.Time) (err error) {
	n.Lock()
	socket := n.socket
	if socket != nil {
		n.Unlock()
		err = socket.Send(msg, deadline)
	} else {
		if n.pendingMsg.Len() >= MaxPendingMsgSize {
			if deadline.IsZero() {
				//deadline.IsZero的包优先级最低，直接丢弃
				return ErrPendingQueueFull
			} else {
				//尝试移除已经失效或deadline.IsZero的msg
				n.removeFailedMsg(true)
				if n.pendingMsg.Len() >= MaxPendingMsgSize {
					return ErrPendingQueueFull
				}
			}
		}

		n.pendingMsg.PushBack(&pendingMessage{
			message:  msg,
			deadline: deadline,
		})
		//尝试与对端建立连接
		if n.pendingMsg.Len() == 1 {
			self.Go(func() {
				n.dial(self)
			})
		}
		n.Unlock()
	}
	return err
}

func (n *node) sendMessageWithContext(ctx context.Context, self *Node, msg interface{}) (err error) {
	n.Lock()
	socket := n.socket
	if socket != nil {
		n.Unlock()
		err = socket.SendWithContext(ctx, msg)
	} else {
		if n.pendingMsg.Len() >= MaxPendingMsgSize {
			//尝试移除已经失效或deadline.IsZero的msg
			n.removeFailedMsg(true)
			if n.pendingMsg.Len() >= MaxPendingMsgSize {
				return ErrPendingQueueFull
			}
		}
		n.pendingMsg.PushBack(&pendingMessage{
			message: msg,
			ctx:     ctx,
		})
		//尝试与对端建立连接
		if n.pendingMsg.Len() == 1 {
			self.Go(func() {
				n.dial(self)
			})
		}
		n.Unlock()
	}
	return err
}

func (n *node) closeSocket() {
	n.Lock()
	if n.socket != nil {
		n.socket.Close(nil)
		n.socket = nil
	}
	n.Unlock()

	n.streamCli.Lock()
	if n.streamCli.session != nil {
		n.streamCli.session.Close()
		n.streamCli.session = nil
	}
	n.streamCli.Unlock()
}
