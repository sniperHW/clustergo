package sanguo

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	"unsafe"

	"github.com/sniperHW/rpcgo"
	"github.com/sniperHW/sanguo/addr"
	"github.com/sniperHW/sanguo/codec/ss"
)

type rpcChannel struct {
	peer addr.LogicAddr
	node *node
}

func (c *rpcChannel) SendRequest(request *rpcgo.RequestMsg, deadline time.Time) error {
	return c.node.sendMessage(context.TODO(), ss.NewMessage(c.peer, c.node.addr.LogicAddr(), request), deadline)
}

func (c *rpcChannel) SendRequestWithContext(ctx context.Context, request *rpcgo.RequestMsg) error {
	return c.node.sendMessage(ctx, ss.NewMessage(c.peer, c.node.addr.LogicAddr(), request), time.Time{})
}

func (c *rpcChannel) Reply(response *rpcgo.ResponseMsg) error {
	return c.node.sendMessage(context.TODO(), ss.NewMessage(c.node.addr.LogicAddr(), c.peer, response), time.Now().Add(time.Second))
}

func (c *rpcChannel) Name() string {
	return fmt.Sprintf("%s <-> %s", c.node.addr.LogicAddr().String(), c.peer.String())
}

func (c *rpcChannel) Identity() uint64 {
	return *(*uint64)(unsafe.Pointer(c.node))
}

// 自连接channel
type selfChannel struct {
	sanguo *Sanguo
}

func (c *selfChannel) SendRequest(request *rpcgo.RequestMsg, deadline time.Time) error {
	c.sanguo.rpcSvr.OnMessage(context.TODO(), c, request)
	return nil
}

func (c *selfChannel) SendRequestWithContext(ctx context.Context, request *rpcgo.RequestMsg) error {
	c.sanguo.rpcSvr.OnMessage(context.TODO(), c, request)
	return nil
}

func (c *selfChannel) Reply(response *rpcgo.ResponseMsg) error {
	c.sanguo.rpcCli.OnMessage(context.TODO(), response)
	return nil
}

func (c *selfChannel) Name() string {
	return "self channel"
}

func (c *selfChannel) Identity() uint64 {
	return 0
}

type JsonCodec struct {
}

func (c *JsonCodec) Encode(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func (c *JsonCodec) Decode(b []byte, v interface{}) error {
	return json.Unmarshal(b, v)
}
