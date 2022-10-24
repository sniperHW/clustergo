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
	"google.golang.org/protobuf/proto"
)

const maxDialCount int = 3

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

func (cache *nodeCache) onNodeInfoUpdate(nodeinfo []addr.Addr) {

	nodes := []addr.Addr{}

	for _, v := range nodeinfo {
		if v.LogicAddr().Cluster() == cache.localAddr.Cluster() {
			//只有与自身cluster相同的节点才需要处理
			nodes = append(nodes, v)
		} else if cache.localAddr.Type() == addr.HarbarType && v.LogicAddr().Type() == addr.HarbarType {
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
		return nodes[l].LogicAddr() < nodes[r].LogicAddr()
	})

	i := 0
	j := 0

	for i < len(nodes) && j < len(localNodes) {
		if nodes[i].LogicAddr() == localNodes[j].addr.LogicAddr() {
			if nodes[i].NetAddr().String() != localNodes[j].addr.NetAddr().String() {
				//网络地址发生变更
				if localNodes[j].addr.LogicAddr() == cache.localAddr {
					go cache.onSelfRemove()
					return
				} else {
					localNodes[j].closeSocket()
					localNodes[j].addr.UpdateNetAddr(nodes[i].NetAddr())
				}
			}
			i++
			j++
		} else if nodes[i].LogicAddr() > localNodes[j].addr.LogicAddr() {
			n := &node{
				addr:       nodes[i],
				pendingMsg: list.New(),
				sanguo:     cache.sanguo,
			}
			cache.nodes[n.addr.LogicAddr()] = n
			if n.addr.LogicAddr().Type() != addr.HarbarType {
				if nodeByType, ok := cache.nodeByType[n.addr.LogicAddr().Type()]; !ok {
					cache.nodeByType[n.addr.LogicAddr().Type()] = []*node{n}
				} else {
					cache.nodeByType[n.addr.LogicAddr().Type()] = append(nodeByType, n)
				}
			} else {
				if harborsByCluster, ok := cache.harborsByCluster[n.addr.LogicAddr().Cluster()]; !ok {
					cache.harborsByCluster[n.addr.LogicAddr().Cluster()] = []*node{n}
				} else {
					cache.harborsByCluster[n.addr.LogicAddr().Cluster()] = append(harborsByCluster, n)
				}
			}
		} else {
			if localNodes[j].addr.LogicAddr() == cache.localAddr {
				go cache.onSelfRemove()
				return
			} else {
				n := localNodes[j]
				delete(cache.nodes, n.addr.LogicAddr())
				if n.addr.LogicAddr().Type() != addr.HarbarType {
					nodeByType := cache.nodeByType[n.addr.LogicAddr().Type()]
					for k, v := range nodeByType {
						if v == n {
							nodeByType[k] = nodeByType[len(nodeByType)-1]
							cache.nodeByType[n.addr.LogicAddr().Type()] = nodeByType[:len(nodeByType)-1]
							break
						}
					}
				} else {
					harborsByCluster := cache.harborsByCluster[n.addr.LogicAddr().Cluster()]
					for k, v := range harborsByCluster {
						if v == n {
							harborsByCluster[k] = harborsByCluster[len(harborsByCluster)-1]
							cache.harborsByCluster[n.addr.LogicAddr().Cluster()] = harborsByCluster[:len(harborsByCluster)-1]
							break
						}
					}
				}
				localNodes[j].closeSocket()
			}
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

func (cache *nodeCache) getNodeByType(tt uint32) *node {
	cache.RLock()
	defer cache.RUnlock()
	if nodes, ok := cache.harborsByCluster[tt]; !ok || len(nodes) == 0 {
		return nil
	} else {
		return nodes[int(rand.Int31())%len(nodes)]
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
			if !m.deadline.IsZero() && now.After(m.deadline) {
				remove = true
			} else if m.ctx != nil {
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
	targetNode := n.sanguo.nodeCache.getNodeByLogicAddr(message.To())
	if targetNode == nil {
		if message.To().Cluster() != n.addr.LogicAddr().Cluster() {
			//不同cluster要求harbor转发
			targetNode = n.sanguo.nodeCache.getHarborByCluster(message.To().Cluster(), message.To())
			if nil != targetNode && targetNode.addr.LogicAddr() == n.addr.LogicAddr() {
				return
			}
		} else {
			//同group,server为0,则从本地随机选择一个符合type的server
			if message.To().Server() == 0 {
				targetNode = n.sanguo.nodeCache.getNodeByType(message.To().Type())
				//设置正确的目标地址
				message.ResetTo(targetNode.addr.LogicAddr())
			}
		}
	}

	if targetNode != nil {
		n.sendMessage(context.TODO(), message, time.Now().Add(time.Second))
	} else if rpcReq := message.GetRpcRequest(); rpcReq != nil {
		//对于无法路由的rpc请求，返回错误响应

		respMsg := ss.NewMessage(message.From(), message.To(), &rpcgo.ResponseMsg{
			Seq: rpcReq.Seq,
			Err: &rpcgo.Error{
				Code: rpcgo.ErrOther,
				Err:  fmt.Sprintf("can't send message to target:%s", message.To().String()),
			},
		})

		if fromNode := n.sanguo.nodeCache.getNodeByLogicAddr(message.From()); fromNode != nil {
			fromNode.sendMessage(context.TODO(), respMsg, time.Now().Add(time.Second))
		} else if harborNode := n.sanguo.nodeCache.getHarborByCluster(message.From().Cluster(), message.From()); harborNode != nil {
			harborNode.sendMessage(context.TODO(), respMsg, time.Now().Add(time.Second))
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
	codec := ss.NewCodec(n.addr.LogicAddr())
	n.socket = netgo.NewAsynSocket(netgo.NewTcpSocket(conn.(*net.TCPConn), codec),
		netgo.AsynSocketOption{
			Codec:    codec,
			AutoRecv: true,
		}).SetPacketHandler(func(_ context.Context, as *netgo.AsynSocket, packet interface{}) error {
		n.onMessage(packet)
		return nil
	}).Recv()
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
