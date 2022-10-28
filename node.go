package sanguo

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/sniperHW/netgo"
	"github.com/sniperHW/rpcgo"
	"github.com/sniperHW/sanguo/addr"
	"github.com/sniperHW/sanguo/codec/ss"
	"github.com/sniperHW/sanguo/discovery"
	"github.com/xtaci/smux"
	"google.golang.org/protobuf/proto"
)

// 从配置系统获得的node视图缓存
type nodeCache struct {
	sync.RWMutex
	localAddr        addr.LogicAddr
	sanguo           *Sanguo
	nodes            map[addr.LogicAddr]*node
	nodeByType       map[uint32][]*node
	harborsByCluster map[uint32][]*node
	onSelfRemove     func()
	initOnce         sync.Once
	initC            chan struct{}
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
	nodeByType := cache.nodeByType[n.addr.LogicAddr().Type()]
	for k, v := range nodeByType {
		if v == n {
			nodeByType[k] = nodeByType[len(nodeByType)-1]
			cache.nodeByType[n.addr.LogicAddr().Type()] = nodeByType[:len(nodeByType)-1]
			break
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
	harborsByCluster := cache.harborsByCluster[harbor.addr.LogicAddr().Cluster()]
	for k, v := range harborsByCluster {
		if v == harbor {
			harborsByCluster[k] = harborsByCluster[len(harborsByCluster)-1]
			cache.harborsByCluster[harbor.addr.LogicAddr().Cluster()] = harborsByCluster[:len(harborsByCluster)-1]
			break
		}
	}
}

func (cache *nodeCache) onNodeInfoUpdate(nodeinfo []discovery.Node) {

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
					go cache.onSelfRemove()
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
				go cache.onSelfRemove()
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
			go cache.onSelfRemove()
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
	dialing    bool
	socket     *netgo.AsynSocket
	pendingMsg *list.List
	available  bool
	streamCli  streamClient
}

func (n *node) dialError(sanguo *Sanguo, err error) {
	select {
	case <-sanguo.die:
		return
	default:
	}

	n.Lock()
	defer n.Unlock()
	n.dialing = false
	if err == ErrInvaildNode {
		n.pendingMsg = list.New()
	} else if nil == n.socket {
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
			n.dialing = true
			time.AfterFunc(time.Second, func() {
				n.dial(sanguo)
			})
		}
	}
}

func (n *node) onRelayMessage(sanguo *Sanguo, message *ss.RelayMessage) {
	logger.Debugf("onRelayMessage self:%s %s->%s", sanguo.localAddr.LogicAddr().String(), message.From().String(), message.To().String())
	if nextNode := sanguo.getNodeByLogicAddr(message.To()); nextNode != nil {
		logger.Debugf("nextNode %s", nextNode.addr.LogicAddr())
		nextNode.sendMessage(context.TODO(), sanguo, message, time.Now().Add(time.Second))
	} else if rpcReq := message.GetRpcRequest(); rpcReq != nil && !rpcReq.Oneway {
		//对于无法路由的rpc请求，返回错误响应
		if nextNode = sanguo.getNodeByLogicAddr(message.From()); nextNode != nil {
			nextNode.sendMessage(context.TODO(), sanguo, ss.NewMessage(message.From(), message.To(), &rpcgo.ResponseMsg{
				Seq: rpcReq.Seq,
				Err: &rpcgo.Error{
					Code: rpcgo.ErrOther,
					Err:  fmt.Sprintf("route message to target:%s failed", message.To().String()),
				},
			}), time.Now().Add(time.Second))
		}
	}
}

func (n *node) onMessage(sanguo *Sanguo, msg interface{}) {
	switch msg := msg.(type) {
	case *ss.Message:
		switch m := msg.Payload().(type) {
		case proto.Message:
			sanguo.dispatchMessage(msg.From(), msg.Cmd(), m)
		case *rpcgo.RequestMsg:
			sanguo.rpcSvr.OnMessage(context.TODO(), &rpcChannel{peer: msg.From(), node: n, sanguo: sanguo}, m)
		case *rpcgo.ResponseMsg:
			sanguo.rpcCli.OnMessage(context.TODO(), m)
		}
	case *ss.RelayMessage:
		n.onRelayMessage(sanguo, msg)
	}
}

func (n *node) onEstablish(sanguo *Sanguo, conn net.Conn) {
	codec := ss.NewCodec(sanguo.localAddr.LogicAddr())
	n.socket = netgo.NewAsynSocket(netgo.NewTcpSocket(conn.(*net.TCPConn), codec),
		netgo.AsynSocketOption{
			Codec:    codec,
			AutoRecv: true,
		}).SetPacketHandler(func(_ context.Context, as *netgo.AsynSocket, packet interface{}) error {
		n.onMessage(sanguo, packet)
		return nil
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

func (n *node) dialOK(sanguo *Sanguo, conn net.Conn) {
	select {
	case <-sanguo.die:
		conn.Close()
		return
	default:
	}

	if n == sanguo.nodeCache.getNodeByLogicAddr(n.addr.LogicAddr()) {
		n.Lock()
		defer n.Unlock()
		n.dialing = false
		if n.socket != nil {
			//两段同时建立连接
			conn.Close()
		} else {
			n.onEstablish(sanguo, conn)
		}
	} else {
		//不再是合法的node
		conn.Close()
		n.dialError(sanguo, ErrInvaildNode)
	}
}

func (n *node) dial(sanguo *Sanguo) {
	dialer := &net.Dialer{}
	logger.Debugf("%s dial %s", sanguo.localAddr.LogicAddr().String(), n.addr.LogicAddr().String())
	if conn, err := dialer.Dial("tcp", n.addr.NetAddr().String()); err != nil {
		n.dialError(sanguo, ErrDial)
	} else {
		if err := n.login(sanguo, conn, false); err != nil {
			conn.Close()
			n.dialError(sanguo, err)
		} else {
			n.dialOK(sanguo, conn)
		}
	}
}

func (n *node) openStream(sanguo *Sanguo) (*smux.Stream, error) {
	n.streamCli.Lock()
	defer n.streamCli.Unlock()
	for {
		if n.streamCli.session != nil {
			if s, err := n.streamCli.session.OpenStream(); err == nil {
				return s, err
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
			} else {
				if err = n.login(sanguo, conn, true); err != nil {
					conn.Close()
					return nil, err
				}

				if session, err := smux.Client(conn, nil); err != nil {
					conn.Close()
					return nil, err
				} else {
					select {
					case <-sanguo.die:
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
}

func (n *node) sendMessage(ctx context.Context, sanguo *Sanguo, msg interface{}, deadline time.Time) (err error) {
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
		if !n.dialing {
			n.dialing = true
			go n.dial(sanguo)
		}
		n.Unlock()
	}
	return err
}

func (n *node) closeSocket() {
	n.Lock()
	defer n.Unlock()
	if n.socket != nil {
		n.socket.Close(nil)
	}
}
