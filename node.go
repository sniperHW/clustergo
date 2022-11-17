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
	localAddr        addr.LogicAddr
	nodes            map[addr.LogicAddr]*node
	nodeByType       map[uint32][]*node
	harborsByCluster map[uint32][]*node
	initOnce         sync.Once
	initC            chan struct{}
}

func (cache *nodeCache) close() {
	cache.RLock()
	for _, v := range cache.nodes {
		v.closeSocket()
	}
	cache.RUnlock()
}

func (cache *nodeCache) waitInit() {
	<-cache.initC
}

func (cache *nodeCache) addNodeByType(n *node) {
	if nodeByType, ok := cache.nodeByType[n.addr.LogicAddr().Type()]; !ok {
		cache.nodeByType[n.addr.LogicAddr().Type()] = []*node{n}
	} else {
		cache.nodeByType[n.addr.LogicAddr().Type()] = append(nodeByType, n)
	}
}

func (cache *nodeCache) removeNodeByType(n *node) {
	if nodeByType, ok := cache.nodeByType[n.addr.LogicAddr().Type()]; ok {
		for k, v := range nodeByType {
			if v == n {
				nodeByType[k] = nodeByType[len(nodeByType)-1]
				cache.nodeByType[n.addr.LogicAddr().Type()] = nodeByType[:len(nodeByType)-1]
				break
			}
		}
	}
}

func (cache *nodeCache) addHarborsByCluster(harbor *node) {
	if harborsByCluster, ok := cache.harborsByCluster[harbor.addr.LogicAddr().Cluster()]; !ok {
		cache.harborsByCluster[harbor.addr.LogicAddr().Cluster()] = []*node{harbor}
	} else {
		cache.harborsByCluster[harbor.addr.LogicAddr().Cluster()] = append(harborsByCluster, harbor)
	}
}

func (cache *nodeCache) removeHarborsByCluster(harbor *node) {
	if harborsByCluster, ok := cache.harborsByCluster[harbor.addr.LogicAddr().Cluster()]; ok {
		for k, v := range harborsByCluster {
			if v == harbor {
				harborsByCluster[k] = harborsByCluster[len(harborsByCluster)-1]
				cache.harborsByCluster[harbor.addr.LogicAddr().Cluster()] = harborsByCluster[:len(harborsByCluster)-1]
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

	nodes := []discovery.Node{}

	for _, v := range nodeinfo {
		if v.Export || v.Addr.LogicAddr().Cluster() == cache.localAddr.Cluster() {
			//Export节点与自身cluster相同的节点才需要处理
			nodes = append(nodes, v)
		} else if cache.localAddr.Type() == addr.HarbarType && v.Addr.LogicAddr().Type() == addr.HarbarType {
			//当前节点是harbor,则不同cluster的harbor节点也需要处理
			nodes = append(nodes, v)
		}
	}

	cache.Lock()
	defer cache.Unlock()

	localNodes := []*node{}
	for _, v := range cache.nodes {
		localNodes = append(localNodes, v)
	}

	sort.Slice(localNodes, func(l int, r int) bool {
		return localNodes[l].addr.LogicAddr() < localNodes[r].addr.LogicAddr()
	})

	sort.Slice(nodes, func(l int, r int) bool {
		return nodes[l].Addr.LogicAddr() < nodes[r].Addr.LogicAddr()
	})

	i := 0
	j := 0

	for i < len(nodes) && j < len(localNodes) {
		localNode := localNodes[j]
		updateNode := nodes[i]

		if updateNode.Addr.LogicAddr() == localNode.addr.LogicAddr() {
			if updateNode.Addr.NetAddr().String() != localNode.addr.NetAddr().String() {
				//网络地址发生变更
				if localNode.addr.LogicAddr() == cache.localAddr {
					go self.Stop()
					return
				} else {
					localNode.closeSocket()
					localNode.addr.UpdateNetAddr(updateNode.Addr.NetAddr())
				}
			}

			if updateNode.Available {
				if !localNode.available {
					localNode.available = true
					if localNode.addr.LogicAddr().Type() != addr.HarbarType {
						cache.addNodeByType(localNode)
					} else {
						cache.addHarborsByCluster(localNode)
					}
				}
			} else {
				if localNode.available {
					localNode.available = false
					if localNode.addr.LogicAddr().Type() != addr.HarbarType {
						cache.removeNodeByType(localNode)
					} else {
						cache.removeHarborsByCluster(localNode)
					}
				}
			}
			i++
			j++
		} else if updateNode.Addr.LogicAddr() > localNode.addr.LogicAddr() {
			//local  1 2 3 4 5 6
			//update 1 2 4 5 6
			//移除节点
			if localNode.addr.LogicAddr() == cache.localAddr {
				go self.Stop()
				return
			} else {
				delete(cache.nodes, localNode.addr.LogicAddr())
				if localNode.available {
					if localNode.addr.LogicAddr().Type() != addr.HarbarType {
						cache.removeNodeByType(localNode)
					} else {
						cache.removeHarborsByCluster(localNode)
					}
				}
				localNode.closeSocket()
			}
			j++
		} else {
			//local  1 2 4 5 6
			//update 1 2 3 4 5 6
			//添加节点
			n := &node{
				addr:       updateNode.Addr,
				pendingMsg: list.New(),
				available:  updateNode.Available,
			}
			cache.nodes[n.addr.LogicAddr()] = n
			if n.available {
				if n.addr.LogicAddr().Type() != addr.HarbarType {
					cache.addNodeByType(n)
				} else {
					cache.addHarborsByCluster(n)
				}
			}
			i++
		}
	}

	for ; i < len(nodes); i++ {
		n := &node{
			addr:       nodes[i].Addr,
			pendingMsg: list.New(),
			available:  nodes[i].Available,
		}
		cache.nodes[n.addr.LogicAddr()] = n
		if n.available {
			if n.addr.LogicAddr().Type() != addr.HarbarType {
				cache.addNodeByType(n)
			} else {
				cache.addHarborsByCluster(n)
			}
		}
	}

	for ; j < len(localNodes); j++ {
		localNode := localNodes[j]
		//移除节点
		if localNode.addr.LogicAddr() == cache.localAddr {
			go self.Stop()
			return
		} else {
			delete(cache.nodes, localNode.addr.LogicAddr())
			if localNode.available {
				if localNode.addr.LogicAddr().Type() != addr.HarbarType {
					cache.removeNodeByType(localNode)
				} else {
					cache.removeHarborsByCluster(localNode)
				}
			}
			localNode.closeSocket()
		}
	}
}

func (cache *nodeCache) getHarborByCluster(cluster uint32, m addr.LogicAddr) *node {
	cache.RLock()
	defer cache.RUnlock()
	if harbors, ok := cache.harborsByCluster[cluster]; !ok || len(harbors) == 0 {
		return nil
	} else {
		return harbors[int(m)%len(harbors)]
	}
}

func (cache *nodeCache) getNodeByType(tt uint32, n int) *node {
	cache.RLock()
	defer cache.RUnlock()
	if nodes, ok := cache.nodeByType[tt]; !ok || len(nodes) == 0 {
		return nil
	} else {
		if n == 0 {
			return nodes[int(rand.Int31())%len(nodes)]
		} else {
			return nodes[n%len(nodes)]
		}
	}
}

func (cache *nodeCache) getNodeByLogicAddr(logicAddr addr.LogicAddr) *node {
	cache.RLock()
	defer cache.RUnlock()
	return cache.nodes[logicAddr]
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

func (n *node) login(self *Node, conn net.Conn, isStream bool) error {

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

func (n *node) onEstablish(self *Node, conn net.Conn) {
	codec := ss.NewCodec(self.localAddr.LogicAddr())
	n.socket = netgo.NewAsynSocket(netgo.NewTcpSocket(conn.(*net.TCPConn), codec),
		netgo.AsynSocketOption{
			Codec:    codec,
			AutoRecv: true,
		}).SetPacketHandler(func(_ context.Context, as *netgo.AsynSocket, packet interface{}) error {
		if self.getNodeByLogicAddr(n.addr.LogicAddr()) != n {
			return ErrInvaildNode
		} else {
			n.onMessage(self, packet)
			return nil
		}
	}).SetCloseCallback(func(as *netgo.AsynSocket, err error) {
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

func (n *node) dialOK(self *Node, conn net.Conn) {
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
		if err = n.login(self, conn, false); err != nil {
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
			n.dialOK(self, conn)
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
			} else if err = n.login(self, conn, true); err != nil {
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
