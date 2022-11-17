package clustergo

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	"unsafe"

	"github.com/sniperHW/clustergo/addr"
	"github.com/sniperHW/clustergo/codec/ss"
	"github.com/sniperHW/rpcgo"
	"google.golang.org/protobuf/proto"
)

type SanguoRPCChannel interface {
	Peer() addr.LogicAddr
}

type rpcChannel struct {
	peer addr.LogicAddr
	node *node
	self *Node
}

func (c *rpcChannel) SendRequest(request *rpcgo.RequestMsg, deadline time.Time) error {
	return c.node.sendMessage(context.TODO(), c.self, ss.NewMessage(c.peer, c.self.localAddr.LogicAddr(), request), deadline)
}

func (c *rpcChannel) SendRequestWithContext(ctx context.Context, request *rpcgo.RequestMsg) error {
	return c.node.sendMessage(ctx, c.self, ss.NewMessage(c.peer, c.self.localAddr.LogicAddr(), request), time.Time{})
}

func (c *rpcChannel) Reply(response *rpcgo.ResponseMsg) error {
	return c.node.sendMessage(context.TODO(), c.self, ss.NewMessage(c.peer, c.self.localAddr.LogicAddr(), response), time.Now().Add(time.Second))
}

func (c *rpcChannel) Name() string {
	return fmt.Sprintf("%s <-> %s", c.self.localAddr.LogicAddr(), c.peer.String())
}

func (c *rpcChannel) Identity() uint64 {
	return *(*uint64)(unsafe.Pointer(c.node))
}

func (c *rpcChannel) Peer() addr.LogicAddr {
	return c.peer
}

// 自连接channel
type selfChannel struct {
	self *Node
}

func (c *selfChannel) SendRequest(request *rpcgo.RequestMsg, deadline time.Time) error {
	go func() {
		c.self.rpcSvr.OnMessage(context.TODO(), c, request)
	}()
	return nil
}

func (c *selfChannel) SendRequestWithContext(ctx context.Context, request *rpcgo.RequestMsg) error {
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
	return fmt.Sprintf("%s <-> %s", c.self.localAddr.LogicAddr(), c.self.localAddr.LogicAddr())
}

func (c *selfChannel) Identity() uint64 {
	return *(*uint64)(unsafe.Pointer(c.self))
}

func (c *selfChannel) Peer() addr.LogicAddr {
	return c.self.localAddr.LogicAddr()
}

type JsonCodec struct {
}

func (c *JsonCodec) Encode(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func (c *JsonCodec) Decode(b []byte, v interface{}) error {
	return json.Unmarshal(b, v)
}

type PbCodec struct {
}

func (c *PbCodec) Encode(v interface{}) ([]byte, error) {
	return proto.Marshal(v.(proto.Message))
}

func (c *PbCodec) Decode(b []byte, v interface{}) error {
	return proto.Unmarshal(b, v.(proto.Message))
}
