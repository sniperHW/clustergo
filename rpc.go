package clustergo

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sniperHW/clustergo/addr"
	"github.com/sniperHW/clustergo/codec/ss"
	"github.com/sniperHW/clustergo/rpc"
	"github.com/sniperHW/clustergo/socket"
	"google.golang.org/protobuf/proto"
)

type RPCChannel interface {
	Peer() addr.LogicAddr
}

type rpcChannel struct {
	peer addr.LogicAddr
	node *node
	self *Node
	name string
	once sync.Once
}

func (c *rpcChannel) RequestWithContext(ctx context.Context, request *rpc.RequestMsg) error {
	return c.node.sendMessageWithContext(ctx, c.self, ss.NewMessage(c.peer, c.self.localAddr.LogicAddr(), request))
}

func (c *rpcChannel) Request(request *rpc.RequestMsg) error {
	return c.node.sendMessage(c.self, ss.NewMessage(c.peer, c.self.localAddr.LogicAddr(), request), time.Time{})
}

func (c *rpcChannel) Reply(response *rpc.ResponseMsg) error {
	return c.node.sendMessage(c.self, ss.NewMessage(c.peer, c.self.localAddr.LogicAddr(), response), time.Now().Add(time.Second))
}

func (c *rpcChannel) Name() string {
	c.once.Do(func() {
		selfAddr := c.self.localAddr.LogicAddr().String()
		peerAddr := c.peer.String()
		if selfAddr > peerAddr {
			c.name = fmt.Sprintf("%s <-> %s", selfAddr, peerAddr)
		} else {
			c.name = fmt.Sprintf("%s <-> %s", peerAddr, selfAddr)
		}
	})
	return c.name
}

func (c *rpcChannel) Peer() addr.LogicAddr {
	return c.peer
}

func (c *rpcChannel) IsRetryAbleError(err error) bool {
	switch err {
	case ErrPendingQueueFull, socket.ErrSendQueueFull, socket.ErrPushToSendQueueTimeout:
		return true
	default:
		return false
	}
}

// 自连接channel
type selfChannel struct {
	self *Node
	name string
	once sync.Once
}

func (c *selfChannel) RequestWithContext(ctx context.Context, request *rpc.RequestMsg) error {
	return c.self.Go(func() {
		c.self.rpcSvr.svr.OnMessage(context.TODO(), c, request)
	})
}

func (c *selfChannel) Request(request *rpc.RequestMsg) error {
	return c.self.Go(func() {
		c.self.rpcSvr.svr.OnMessage(context.TODO(), c, request)
	})
}

func (c *selfChannel) Reply(response *rpc.ResponseMsg) error {
	return c.self.Go(func() {
		c.self.rpcCli.cli.OnMessage(response)
	})
}

func (c *selfChannel) Name() string {
	c.once.Do(func() {
		localAddrStr := c.self.localAddr.LogicAddr().String()
		c.name = fmt.Sprintf("%s <-> %s", localAddrStr, localAddrStr)
	})
	return c.name
}

func (c *selfChannel) Peer() addr.LogicAddr {
	return c.self.localAddr.LogicAddr()
}

func (c *selfChannel) IsRetryAbleError(_ error) bool {
	return false
}

type JsonCodec struct {
}

func (c JsonCodec) Encode(dst []byte, v interface{}) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return dst, err
	}
	return append(dst, b...), nil
}

func (c JsonCodec) Decode(b []byte, v interface{}) error {
	return json.Unmarshal(b, v)
}

type PbCodec struct {
}

func (c PbCodec) Encode(dst []byte, v interface{}) ([]byte, error) {
	return proto.MarshalOptions{}.MarshalAppend(dst, v.(proto.Message))
}

func (c PbCodec) Decode(b []byte, v interface{}) error {
	return proto.Unmarshal(b, v.(proto.Message))
}

type RPCServer struct {
	pendingRespCount atomic.Int32 //尚未响应的rpc数量
	svr              *rpc.Server
}

func (s *RPCServer) SetInInterceptor(interceptor []func(*rpc.Replyer, *rpc.RequestMsg) bool) {
	s.svr.SetInInterceptor(append(interceptor, func(replyer *rpc.Replyer, req *rpc.RequestMsg) bool {
		s.pendingRespCount.Add(1)
		replyer.AppendOutInterceptor(func(req *rpc.RequestMsg, ret interface{}, err error) {
			s.pendingRespCount.Add(-1)
		})
		return true
	}))
}

type RPCClient struct {
	n   *Node
	cli *rpc.Client
}

func (c *RPCClient) SetInInterceptor(interceptor []func(*rpc.RequestMsg, interface{}, error)) {
	c.cli.SetInInterceptor(interceptor)
}

func (c *RPCClient) SetOutInterceptor(interceptor []func(*rpc.RequestMsg, interface{})) {
	c.cli.SetOutInterceptor(interceptor)
}

func (c *RPCClient) AsyncCall(to addr.LogicAddr, method string, arg interface{}, ret interface{}, deadline time.Time, callback func(interface{}, error)) error {
	s := c.n
	select {
	case <-s.die:
		return rpc.NewError(rpc.ErrOther, "server die")
	case <-s.started:
	default:
		return rpc.NewError(rpc.ErrOther, "server not start")
	}
	var err error
	if to == s.localAddr.LogicAddr() {
		err = c.cli.AsyncCall(&selfChannel{self: s}, method, arg, ret, deadline, callback)
	} else if n := s.getNodeByLogicAddr(to); n != nil {
		err = c.cli.AsyncCall(&rpcChannel{peer: to, node: n, self: s}, method, arg, ret, deadline, callback)
	} else {
		return ErrInvaildNode
	}

	switch err {
	case ErrPendingQueueFull, socket.ErrSendQueueFull:
		return ErrBusy
	default:
		return err
	}
}

func (c *RPCClient) CallWithTimeout(to addr.LogicAddr, method string, arg interface{}, ret interface{}, d time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	return c.Call(ctx, to, method, arg, ret)
}

func (c *RPCClient) Call(ctx context.Context, to addr.LogicAddr, method string, arg interface{}, ret interface{}) error {
	s := c.n
	select {
	case <-s.die:
		return rpc.NewError(rpc.ErrOther, "server die")
	case <-s.started:
	default:
		return rpc.NewError(rpc.ErrOther, "server not start")
	}
	if to == s.localAddr.LogicAddr() {
		return c.cli.Call(ctx, &selfChannel{self: s}, method, arg, ret)
	} else {
		if n := s.getNodeByLogicAddr(to); n != nil {
			return c.cli.Call(ctx, &rpcChannel{peer: to, node: n, self: s}, method, arg, ret)
		} else {
			return ErrInvaildNode
		}
	}
}
