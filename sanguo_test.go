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

type localDiscovery struct {
	nodes      map[addr.LogicAddr]*discovery.Node
	subscribes []func([]discovery.Node)
}

func (d *localDiscovery) LoadNodeInfo() (nodes []discovery.Node, err error) {
	for _, v := range d.nodes {
		nodes = append(nodes, *v)
	}
	return nodes, err
}

// 订阅变更
func (d *localDiscovery) Subscribe(updateCB func([]discovery.Node)) {
	d.subscribes = append(d.subscribes, updateCB)
}

func (d *localDiscovery) AddNode(n *discovery.Node) {
	d.nodes[n.Addr.LogicAddr()] = n
	nodes, _ := d.LoadNodeInfo()
	for _, v := range d.subscribes {
		v(nodes)
	}
}

func (d *localDiscovery) RemoveNode(logicAddr addr.LogicAddr) {
	delete(d.nodes, logicAddr)
	nodes, _ := d.LoadNodeInfo()
	for _, v := range d.subscribes {
		v(nodes)
	}
}

func (d *localDiscovery) ModifyNode(logicAddr addr.LogicAddr, avaliable bool) {
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
	l := zap.NewZapLogger("sanguo_test.log", "./logfile", "debug", 1024*1024*100, 14, 28, true)
	rpcgo.InitLogger(l)
	InitLogger(l.Sugar())
}

func TestSingleNode(t *testing.T) {
	localDiscovery := &localDiscovery{
		nodes: map[addr.LogicAddr]*discovery.Node{},
	}

	localAddr, _ := addr.MakeAddr("1.1.1", "localhost:8110")

	localDiscovery.AddNode(&discovery.Node{
		Addr:      localAddr,
		Available: true,
	})

	s := NewSanguo()
	s.RegisterMessageHandler(&ss.Echo{}, func(_ addr.LogicAddr, msg proto.Message) {
		logger.Debug(msg.(*ss.Echo).Msg)
	})

	s.RegisterRPC("hello", func(_ context.Context, replyer *rpcgo.Replyer, arg *string) {
		replyer.Reply(fmt.Sprintf("hello world:%s", *arg), nil)
	})

	err := s.Start(localDiscovery, localAddr.LogicAddr())
	assert.Nil(t, err)

	s.SendMessage(localAddr.LogicAddr(), &ss.Echo{
		Msg: "hello",
	})

	var resp string
	err = s.Call(context.TODO(), localAddr.LogicAddr(), "hello", "sniperHW", &resp)
	assert.Nil(t, err)
	assert.Equal(t, resp, "hello world:sniperHW")
	localDiscovery.RemoveNode(localAddr.LogicAddr())
	s.Wait()
}

func TestTwoNode(t *testing.T) {
	localDiscovery := &localDiscovery{
		nodes: map[addr.LogicAddr]*discovery.Node{},
	}

	node1Addr, _ := addr.MakeAddr("1.1.1", "localhost:8110")
	node2Addr, _ := addr.MakeAddr("1.2.1", "localhost:8111")

	localDiscovery.AddNode(&discovery.Node{
		Addr:      node1Addr,
		Available: true,
	})

	localDiscovery.AddNode(&discovery.Node{
		Addr:      node2Addr,
		Available: true,
	})

	node1 := NewSanguo()
	node1.RegisterMessageHandler(&ss.Echo{}, func(_ addr.LogicAddr, msg proto.Message) {
		logger.Debug(msg.(*ss.Echo).Msg)
	})

	node1.RegisterRPC("hello", func(_ context.Context, replyer *rpcgo.Replyer, arg *string) {
		logger.Debug("on hello call")
		replyer.Reply(fmt.Sprintf("hello world:%s", *arg), nil)
	})

	err := node1.Start(localDiscovery, node1Addr.LogicAddr())
	assert.Nil(t, err)

	node2 := NewSanguo()
	err = node2.Start(localDiscovery, node2Addr.LogicAddr())
	assert.Nil(t, err)

	logger.Debug("Start OK")

	node2.SendMessage(node1Addr.LogicAddr(), &ss.Echo{
		Msg: "hello",
	})

	var resp string
	err = node2.Call(context.TODO(), node1Addr.LogicAddr(), "hello", "sniperHW", &resp)
	assert.Nil(t, err)
	assert.Equal(t, resp, "hello world:sniperHW")

	localDiscovery.RemoveNode(node1Addr.LogicAddr())
	node1.Wait()

	_, err = node2.GetAddrByType(1)
	assert.NotNil(t, err)

	localDiscovery.RemoveNode(node2Addr.LogicAddr())
	node2.Wait()

}

func TestHarbor(t *testing.T) {
	localDiscovery := &localDiscovery{
		nodes: map[addr.LogicAddr]*discovery.Node{},
	}

	//cluster:1
	node1Addr, _ := addr.MakeAddr("1.1.1", "localhost:8110")
	harbor1Addr, _ := addr.MakeHarborAddr("1.255.1", "localhost:9110")

	//cluster:2
	node2Addr, _ := addr.MakeAddr("2.2.1", "localhost:8111")
	harbor2Addr, _ := addr.MakeHarborAddr("2.255.1", "localhost:9111")

	localDiscovery.AddNode(&discovery.Node{
		Addr:      node1Addr,
		Available: true,
		Export:    true, //将节点暴露到cluster以外
	})

	localDiscovery.AddNode(&discovery.Node{
		Addr:      harbor1Addr,
		Available: true,
	})

	localDiscovery.AddNode(&discovery.Node{
		Addr:      node2Addr,
		Available: true,
	})

	localDiscovery.AddNode(&discovery.Node{
		Addr:      harbor2Addr,
		Available: true,
	})

	node1 := NewSanguo()
	node1.RegisterMessageHandler(&ss.Echo{}, func(_ addr.LogicAddr, msg proto.Message) {
		logger.Debug(msg.(*ss.Echo).Msg)
	})

	node1.RegisterRPC("hello", func(_ context.Context, replyer *rpcgo.Replyer, arg *string) {
		logger.Debug("on hello call")
		replyer.Reply(fmt.Sprintf("hello world:%s", *arg), nil)
	})

	err := node1.Start(localDiscovery, node1Addr.LogicAddr())
	assert.Nil(t, err)

	node2 := NewSanguo()
	err = node2.Start(localDiscovery, node2Addr.LogicAddr())
	assert.Nil(t, err)

	harbor1 := NewSanguo()
	err = harbor1.Start(localDiscovery, harbor1Addr.LogicAddr())
	assert.Nil(t, err)

	harbor2 := NewSanguo()
	err = harbor2.Start(localDiscovery, harbor2Addr.LogicAddr())
	assert.Nil(t, err)

	var type1Addr addr.LogicAddr

	for {
		if type1Addr, err = node2.GetAddrByType(1); err == nil {
			break
		}
	}

	node1.SendMessage(type1Addr, &ss.Echo{
		Msg: "hello",
	})

	var resp string
	err = node2.Call(context.TODO(), type1Addr, "hello", "sniperHW", &resp)
	assert.Nil(t, err)
	assert.Equal(t, resp, "hello world:sniperHW")

	//将1.1.1移除
	localDiscovery.RemoveNode(node1Addr.LogicAddr())
	err = node2.Call(context.TODO(), type1Addr, "hello", "sniperHW", &resp)
	assert.Equal(t, "can't send message to target:1.1.1", err.Error())
	logger.Debug(err)

	node1.Stop()
	node2.Stop()
	harbor1.Stop()
	harbor2.Stop()

	node1.Wait()
	node2.Wait()
	harbor1.Wait()
	harbor2.Wait()
}
