package sanguo

import (
	"container/list"
	"context"
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
				sanguo:     cache.sanguo,
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
			sanguo:     cache.sanguo,
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

type node struct {
	sync.Mutex
	addr       addr.Addr
	dialing    bool
	socket     *netgo.AsynSocket
	pendingMsg *list.List
	sanguo     *Sanguo
	available  bool
}

func (n *node) dialError(err error) {
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
			time.AfterFunc(time.Second, n.dial)
		}
	}
}

func (n *node) onRelayMessage(message *ss.RelayMessage) {
	//if message.From() == n.sanguo.localAddr.LogicAddr() {
	//	panic("here")
	//}

	logger.Debugf("onRelayMessage self:%s %s->%s", n.sanguo.localAddr.LogicAddr().String(), message.From().String(), message.To().String())

	var nextNode *node //下一跳节点
	if message.To().Cluster() == n.sanguo.localAddr.LogicAddr().Cluster() {
		//当前节点与目标节点处于同一个cluster
		nextNode = n.sanguo.nodeCache.getNodeByLogicAddr(message.To())
	} else {
		//获取目标cluster的harbor
		nextNode = n.sanguo.nodeCache.getHarborByCluster(message.To().Cluster(), message.To())
		if nil != nextNode && nextNode.addr.LogicAddr() == n.addr.LogicAddr() {
			return
		}
	}

	if nextNode != nil {

		logger.Debugf("nextNode %s", nextNode.addr.LogicAddr())

		nextNode.sendMessage(context.TODO(), message, time.Now().Add(time.Second))
	} else if rpcReq := message.GetRpcRequest(); rpcReq != nil {
		//对于无法路由的rpc请求，返回错误响应
		respMsg := ss.NewMessage(message.From(), message.To(), &rpcgo.ResponseMsg{
			Seq: rpcReq.Seq,
			Err: &rpcgo.Error{
				Code: rpcgo.ErrOther,
				Err:  fmt.Sprintf("can't send message to target:%s", message.To().String()),
			},
		})

		if message.From().Cluster() == n.sanguo.localAddr.LogicAddr().Cluster() {
			nextNode = n.sanguo.nodeCache.getNodeByLogicAddr(message.From())
		} else {
			nextNode = n.sanguo.nodeCache.getHarborByCluster(message.From().Cluster(), message.From())
		}

		if nextNode != nil {
			nextNode.sendMessage(context.TODO(), respMsg, time.Now().Add(time.Second))
		}
	}
}

func (n *node) onMessage(msg interface{}) {
	switch msg := msg.(type) {
	case *ss.Message:
		switch m := msg.Data().(type) {
		case proto.Message:
			n.sanguo.dispatchMessage(msg.From(), msg.Cmd(), m)
		case *rpcgo.RequestMsg:
			n.sanguo.rpcSvr.OnMessage(context.TODO(), &rpcChannel{peer: msg.From(), node: n}, m)
		case *rpcgo.ResponseMsg:
			n.sanguo.rpcCli.OnMessage(context.TODO(), m)
		}
	case *ss.RelayMessage:
		n.onRelayMessage(msg)
	}
}

func (n *node) onEstablish(conn net.Conn) {
	codec := ss.NewCodec(n.sanguo.localAddr.LogicAddr())
	n.socket = netgo.NewAsynSocket(netgo.NewTcpSocket(conn.(*net.TCPConn), codec),
		netgo.AsynSocketOption{
			Codec:    codec,
			AutoRecv: true,
		}).SetPacketHandler(func(_ context.Context, as *netgo.AsynSocket, packet interface{}) error {
		n.onMessage(packet)
		return nil
	}).Recv()

	now := time.Now()

	//logger.Debugf("onEstablish %s %s %d", n.addr.LogicAddr().String(), n.addr.NetAddr().String(), n.pendingMsg.Len())

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

func (n *node) dialOK(conn net.Conn) {
	if n == n.sanguo.nodeCache.getNodeByLogicAddr(n.addr.LogicAddr()) {
		n.Lock()
		defer n.Unlock()
		n.dialing = false
		if n.socket != nil {
			//两段同时建立连接
			conn.Close()
		} else {
			n.onEstablish(conn)
		}
	} else {
		//不再是合法的node
		conn.Close()
		n.dialError(ErrInvaildNode)
	}
}

func (n *node) dial() {
	dialer := &net.Dialer{}
	logger.Debugf("%s dial %s", n.sanguo.localAddr.LogicAddr().String(), n.addr.LogicAddr().String())
	if conn, err := dialer.Dial("tcp", n.addr.NetAddr().String()); err != nil {
		n.dialError(ErrDial)
	} else {
		if err := n.login(conn); err != nil {
			conn.Close()
			n.dialError(err)
		} else {
			n.dialOK(conn)
		}
	}
}

func (n *node) sendMessage(ctx context.Context, msg interface{}, deadline time.Time) (err error) {
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
			go n.dial()
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
