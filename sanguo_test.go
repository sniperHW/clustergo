package sanguo

//go test -race -covermode=atomic -v -coverprofile=coverage.out -run=.
//go tool cover -html=coverage.out

import (
	"context"
	"fmt"
	"testing"

	"github.com/sniperHW/rpcgo"
	"github.com/sniperHW/sanguo/addr"
	"github.com/sniperHW/sanguo/codec/pb"
	"github.com/sniperHW/sanguo/codec/ss"
	"github.com/sniperHW/sanguo/discovery"
	"github.com/sniperHW/sanguo/log/zap"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"
)

type cacheDiscovery struct {
	nodes      map[addr.LogicAddr]*discovery.Node
	subscribes []func([]discovery.Node)
}

func (d *cacheDiscovery) LoadNodeInfo() (nodes []discovery.Node, err error) {
	for _, v := range d.nodes {
		nodes = append(nodes, *v)
	}
	return nodes, err
}

// 订阅变更
func (d *cacheDiscovery) Subscribe(updateCB func([]discovery.Node)) {
	d.subscribes = append(d.subscribes, updateCB)
}

func (d *cacheDiscovery) AddNode(n *discovery.Node) {
	d.nodes[n.Addr.LogicAddr()] = n
	nodes, _ := d.LoadNodeInfo()
	for _, v := range d.subscribes {
		v(nodes)
	}
}

func (d *cacheDiscovery) RemoveNode(logicAddr addr.LogicAddr) {
	delete(d.nodes, logicAddr)
	nodes, _ := d.LoadNodeInfo()
	for _, v := range d.subscribes {
		v(nodes)
	}
}

func (d *cacheDiscovery) ModifyNode(logicAddr addr.LogicAddr, avaliable bool) {
	if n, ok := d.nodes[logicAddr]; ok && n.Available != avaliable {
		n.Available = avaliable
		nodes, _ := d.LoadNodeInfo()
		for _, v := range d.subscribes {
			v(nodes)
		}
	}
}

func init() {
	pb.Register(ss.Namespace, &ss.Echo{}, 1)
	InitLogger(zap.NewZapLogger("sanguo_test.log", "./logfile", "debug", 1024*1024*100, 14, 28, true).Sugar())
}

func TestSingleNode(t *testing.T) {
	cacheDiscovery := &cacheDiscovery{
		nodes: map[addr.LogicAddr]*discovery.Node{},
	}

	localAddr, _ := addr.MakeAddr("1.1.1", "localhost:8110")

	cacheDiscovery.AddNode(&discovery.Node{
		Addr:      localAddr,
		Available: true,
	})

	logger.Debug(cacheDiscovery.LoadNodeInfo())

	s := NewSanguo()
	s.RegisterMessageHandler(&ss.Echo{}, func(_ addr.LogicAddr, msg proto.Message) {
		logger.Debug(msg.(*ss.Echo).Msg)
	})

	s.RegisterRPC("hello", func(_ context.Context, replyer *rpcgo.Replyer, arg *string) {
		replyer.Reply(fmt.Sprintf("hello world:%s", *arg), nil)
	})

	err := s.Start(cacheDiscovery, localAddr.LogicAddr())
	assert.Nil(t, err)

	s.SendMessage(localAddr.LogicAddr(), &ss.Echo{
		Msg: "hello",
	})

	var resp string
	err = s.Call(context.TODO(), localAddr.LogicAddr(), "hello", "sniperHW", &resp)
	assert.Nil(t, err)
	assert.Equal(t, resp, "hello world:sniperHW")
	cacheDiscovery.RemoveNode(localAddr.LogicAddr())
	s.Wait()
}
