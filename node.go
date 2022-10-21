package sanguo

import (
	"container/list"
	"context"
	"fmt"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/sniperHW/netgo"
	"github.com/sniperHW/rpcgo"
	"github.com/sniperHW/sanguo/addr"
	"github.com/sniperHW/sanguo/codec/ss"
)

const maxDialCount int = 3

// 从配置系统获得的node视图缓存
type nodeCache struct {
	sync.RWMutex
	nodes            map[addr.LogicAddr]*node
	nodeByType       map[uint32][]*node
	harborsByCluster map[uint32][]*node
}

func (n *nodeCache) getHarborByCluster(cluster uint32, m addr.LogicAddr) *node {
	n.RLock()
	defer n.RUnlock()
	if harbors, ok := n.harborsByCluster[cluster]; !ok || len(harbors) == 0 {
		return nil
	} else {
		return harbors[int(m)%len(harbors)]
	}
}

func (n *nodeCache) getNodeByType(tt uint32) *node {
	n.RLock()
	defer n.RUnlock()
	if nodes, ok := n.harborsByCluster[tt]; !ok || len(nodes) == 0 {
		return nil
	} else {
		return nodes[int(rand.Int31())%len(nodes)]
	}
}

func (n *nodeCache) getNodeByLogicAddr(logicAddr addr.LogicAddr) *node {
	n.RLock()
	defer n.RUnlock()
	return n.nodes[logicAddr]
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

func (n *node) isHarbor() bool {
	return n.addr.LogicAddr().Type() == addr.HarbarType
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
	targetNode := n.sanguo.nodeCache.getNodeByLogicAddr(message.To)
	if targetNode == nil {
		if message.To.Cluster() != n.addr.LogicAddr().Cluster() {
			//不同cluster要求harbor转发
			targetNode = n.sanguo.nodeCache.getHarborByCluster(message.To.Cluster(), message.To)
			if nil != targetNode && targetNode.addr.LogicAddr() == n.addr.LogicAddr() {
				return
			}
		} else {
			//同group,server为0,则从本地随机选择一个符合type的server
			if message.To.Server() == 0 {
				targetNode = n.sanguo.nodeCache.getNodeByType(message.To.Type())
				//设置正确的目标地址
				message.ResetTo(targetNode.addr.LogicAddr())
			}
		}
	}

	if targetNode != nil {
		n.sendMessage(nil, message, time.Now().Add(time.Second))
	} else if rpcReq := message.GetRpcRequest(); rpcReq != nil {
		//对于无法路由的rpc请求，返回错误响应

		respMsg := ss.NewMessage(message.From, message.To, &rpcgo.ResponseMsg{
			Seq: rpcReq.Seq,
			Err: &rpcgo.Error{
				Code: rpcgo.ErrOther,
				Err:  fmt.Sprintf("can't send message to target:%s", message.To.String()),
			},
		})

		if fromNode := n.sanguo.nodeCache.getNodeByLogicAddr(message.From); fromNode != nil {
			fromNode.sendMessage(nil, respMsg, time.Now().Add(time.Second))
		} else if harborNode := n.sanguo.nodeCache.getHarborByCluster(message.From.Cluster(), message.From); harborNode != nil {
			harborNode.sendMessage(nil, respMsg, time.Now().Add(time.Second))
		}
	}
}

func (n *node) onMessage(msg interface{}, from *node) {
	switch msg := msg.(type) {
	case *ss.Message:
		/*switch m := msg.Data().(type) {
		case proto.Message:
		case *rpcgo.RequestMsg:
		case *rpcgo.ResponseMsg:
		}*/

	case *ss.RelayMessage:
		n.onRelayMessage(msg)
	}
}

func (n *node) dialOK(conn net.Conn) {
	if n == n.sanguo.nodeCache.getNodeByLogicAddr(n.addr.LogicAddr()) {
		n.Lock()
		defer n.Unlock()
		if n.socket != nil {
			//两段同时建立连接
			conn.Close()
		} else {
			//n.socket =
		}
	} else {
		//不再是合法的node
		conn.Close()
		n.dialError(ErrInvaildNode)
	}
}

func (n *node) dial() {
	go func() {
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
	}()
}

func (n *node) sendMessage(ctx context.Context, msg interface{}, deadline time.Time) {
	n.Lock()
	socket := n.socket
	if socket != nil {
		n.Unlock()
		var err error
		if ctx != nil {
			err = socket.SendWithContext(ctx, msg)
		} else {
			err = socket.Send(msg, deadline)
		}
		if err != nil {
			//记录日志
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
			n.dial()
		}
		n.Unlock()
	}
}
