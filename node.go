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
	"sort"
	"sync"
	"time"

	"github.com/sniperHW/clustergo/addr"
	"github.com/sniperHW/clustergo/codec/ss"
	"github.com/sniperHW/clustergo/discovery"
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

func (cache *nodeCache) onNodeInfoUpdate(self *Node, nodeinfo []discovery.Node) {

	defer cache.initOnce.Do(func() {
		logger.Debug("init ok")
		close(cache.initC)
	})

	updates := []discovery.Node{}

	for _, v := range nodeinfo {
		if v.Export || v.Addr.LogicAddr().Cluster() == cache.localAddr.Cluster() {
			//Export节点与自身cluster相同的节点才需要处理
			updates = append(updates, v)
		} else if cache.localAddr.Type() == addr.HarbarType && v.Addr.LogicAddr().Type() == addr.HarbarType {
			//当前节点是harbor,则不同cluster的harbor节点也需要处理
			updates = append(updates, v)
		}
	}

	cache.Lock()
	defer cache.Unlock()

	locals := []*node{}
	for _, v := range cache.allnodes {
		locals = append(locals, v)
	}

	sort.Slice(locals, func(l int, r int) bool {
		return locals[l].addr.LogicAddr() < locals[r].addr.LogicAddr()
	})

	sort.Slice(updates, func(l int, r int) bool {
		return updates[l].Addr.LogicAddr() < updates[r].Addr.LogicAddr()
	})

	i := 0
	j := 0

	for i < len(updates) && j < len(locals) {
		nodej := locals[j]
		nodei := updates[i]

		if nodei.Addr.LogicAddr() == nodej.addr.LogicAddr() {
			if nodei.Addr.NetAddr().String() != nodej.addr.NetAddr().String() {
				//网络地址发生变更
				if nodej.addr.LogicAddr() == cache.localAddr {
					go self.Stop()
					return
				} else {
					nodej.closeSocket()
					nodej.addr.UpdateNetAddr(nodei.Addr.NetAddr())
				}
			}

			if nodei.Available {
				if !nodej.available {
					nodej.available = true
					if nodej.addr.LogicAddr().Type() != addr.HarbarType {
						cache.addNode(nodej)
					} else {
						cache.addHarbor(nodej)
					}
				}
			} else {
				if nodej.available {
					nodej.available = false
					if nodej.addr.LogicAddr().Type() != addr.HarbarType {
						cache.removeNode(nodej)
					} else {
						cache.removeHarbor(nodej)
					}
				}
			}
			i++
			j++
		} else if nodei.Addr.LogicAddr() > nodej.addr.LogicAddr() {
			//local  1 2 3 4 5 6
			//update 1 2 4 5 6
			//移除节点
			if nodej.addr.LogicAddr() == cache.localAddr {
				go self.Stop()
				return
			} else {
				delete(cache.allnodes, nodej.addr.LogicAddr())
				if nodej.available {
					if nodej.addr.LogicAddr().Type() != addr.HarbarType {
						cache.removeNode(nodej)
					} else {
						cache.removeHarbor(nodej)
					}
				}
				nodej.closeSocket()
			}
			j++
		} else {
			//local  1 2 4 5 6
			//update 1 2 3 4 5 6
			//添加节点
			n := &node{
				addr:       nodei.Addr,
				pendingMsg: list.New(),
				available:  nodei.Available,
			}
			cache.allnodes[n.addr.LogicAddr()] = n
			if n.available {
				if n.addr.LogicAddr().Type() != addr.HarbarType {
					cache.addNode(n)
				} else {
					cache.addHarbor(n)
				}
			}
			i++
		}
	}

	for _, v := range updates[i:] {
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

	for _, v := range locals[j:] {
		//移除节点
		if v.addr.LogicAddr() == cache.localAddr {
			go self.Stop()
			return
		} else {
			delete(cache.allnodes, v.addr.LogicAddr())
			if v.available {
				if v.addr.LogicAddr().Type() != addr.HarbarType {
					cache.removeNode(v)
				} else {
					cache.removeHarbor(v)
				}
			}
			v.closeSocket()
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
		nextNode.sendMessage(context.TODO(), self, message, time.Now().Add(time.Second))
	} else if rpcReq := message.GetRpcRequest(); rpcReq != nil && !rpcReq.Oneway {
		//对于无法路由的rpc请求，返回错误响应
		if nextNode = self.getNodeByLogicAddr(message.From()); nextNode != nil {
			logger.Debugf(fmt.Sprintf("route message to target:%s failed", message.To().String()))
			nextNode.sendMessage(context.TODO(), self, ss.NewMessage(message.From(), self.localAddr.LogicAddr(), &rpcgo.ResponseMsg{
				Seq: rpcReq.Seq,
				Err: &rpcgo.Error{
					Code: rpcgo.ErrOther,
					Err:  fmt.Sprintf("route message to target:%s failed", message.To().String()),
				},
			}), time.Now().Add(time.Second))
		}
	}
}

func (n *node) onMessage(self *Node, msg interface{}) {
	switch msg := msg.(type) {
	case *ss.Message:
		switch m := msg.Payload().(type) {
		case proto.Message:
			self.dispatchMessage(msg.From(), msg.Cmd(), m)
		case *rpcgo.RequestMsg:
			self.rpcSvr.OnMessage(context.TODO(), &rpcChannel{peer: msg.From(), node: n, self: self}, m)
		case *rpcgo.ResponseMsg:
			self.rpcCli.OnMessage(context.TODO(), m)
		}
	case *ss.RelayMessage:
		n.onRelayMessage(self, msg)
	}
}

func (n *node) onEstablish(self *Node, conn *net.TCPConn) {
	codec := ss.NewCodec(self.localAddr.LogicAddr())
	n.socket = netgo.NewAsynSocket(netgo.NewTcpSocket(conn, codec), netgo.AsynSocketOption{Codec: codec, AutoRecv: true})

	n.socket.SetPacketHandler(func(_ context.Context, as *netgo.AsynSocket, packet interface{}) error {
		if self.getNodeByLogicAddr(n.addr.LogicAddr()) != n {
			return ErrInvaildNode
		} else {
			n.onMessage(self, packet)
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
		if !msg.deadline.IsZero() {
			if now.Before(msg.deadline) {
				n.socket.Send(msg.message, msg.deadline)
			}
		} else if msg.ctx != nil {
			select {
			case <-msg.ctx.Done():
			default:
				n.socket.SendWithContext(msg.ctx, msg.message)
			}
		}
	}
}

func (n *node) dialError(self *Node) {
	n.Lock()
	defer n.Unlock()
	if nil == n.socket {
		now := time.Now()
		e := n.pendingMsg.Front()
		for e != nil {
			m := e.Value.(*pendingMessage)
			remove := false

			if !m.deadline.IsZero() {
				if now.After(m.deadline) {
					remove = true
				}
			} else {
				select {
				case <-m.ctx.Done():
					remove = true
				default:
				}
			}

			if remove {
				next := e.Next()
				n.pendingMsg.Remove(e)
				e = next
			} else {
				e = e.Next()
			}
		}

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
				return stream, err
			} else if err == smux.ErrGoAway {
				return nil, err
			} else {
				n.streamCli.session.Close()
				n.streamCli.session = nil
				return nil, err
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

func (n *node) sendMessage(ctx context.Context, self *Node, msg interface{}, deadline time.Time) (err error) {
	n.Lock()
	socket := n.socket
	if socket != nil {
		n.Unlock()
		if deadline.IsZero() {
			err = socket.SendWithContext(ctx, msg)
		} else {
			err = socket.Send(msg, deadline)
		}
	} else {
		n.pendingMsg.PushBack(&pendingMessage{
			message:  msg,
			ctx:      ctx,
			deadline: deadline,
		})
		//尝试与对端建立连接
		if n.pendingMsg.Len() == 1 {
			go n.dial(self)
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
