package clustergo

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
	"unsafe"

	"github.com/sniperHW/clustergo/addr"
	"github.com/sniperHW/clustergo/codec/ss"
	"github.com/sniperHW/netgo"
	"github.com/sniperHW/rpcgo"
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

func (c *rpcChannel) SendRequest(ctx context.Context, request *rpcgo.RequestMsg) error {
	return c.node.sendMessageWithContext(ctx, c.self, ss.NewMessage(c.peer, c.self.localAddr.LogicAddr(), request))
}

func (c *rpcChannel) Reply(response *rpcgo.ResponseMsg) error {
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

func (c *rpcChannel) Identity() uint64 {
	return *(*uint64)(unsafe.Pointer(c.node))
}

func (c *rpcChannel) Peer() addr.LogicAddr {
	return c.peer
}

func (c *rpcChannel) IsRetryAbleError(err error) bool {
	switch err {
	case ErrPendingQueueFull, netgo.ErrSendQueueFull, netgo.ErrPushToSendQueueTimeout:
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

func (c *selfChannel) SendRequest(ctx context.Context, request *rpcgo.RequestMsg) error {
	go func() {
		c.self.rpcSvr.OnMessage(context.TODO(), c, request)
	}()
	return nil
}

func (c *selfChannel) Reply(response *rpcgo.ResponseMsg) error {
	go func() {
		c.self.rpcCli.OnMessage(context.TODO(), response)
	}()
	return nil
}

func (c *selfChannel) Name() string {
	c.once.Do(func() {
		localAddrStr := c.self.localAddr.LogicAddr().String()
		c.name = fmt.Sprintf("%s <-> %s", localAddrStr, localAddrStr)
	})
	return c.name
}

func (c *selfChannel) Identity() uint64 {
	return *(*uint64)(unsafe.Pointer(c.self))
}

func (c *selfChannel) Peer() addr.LogicAddr {
	return c.self.localAddr.LogicAddr()
}

func (c *selfChannel) IsRetryAbleError(_ error) bool {
	return false
}

type JsonCodec struct {
}

func (c JsonCodec) Encode(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func (c JsonCodec) Decode(b []byte, v interface{}) error {
	return json.Unmarshal(b, v)
}

type PbCodec struct {
}

func (c PbCodec) Encode(v interface{}) ([]byte, error) {
	return proto.Marshal(v.(proto.Message))
}

func (c PbCodec) Decode(b []byte, v interface{}) error {
	return proto.Unmarshal(b, v.(proto.Message))
}
