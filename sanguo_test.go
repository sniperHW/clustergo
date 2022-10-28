package sanguo

//go test -race -covermode=atomic -v -coverprofile=coverage.out -run=.
//go tool cover -html=coverage.out

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/sniperHW/rpcgo"
	"github.com/sniperHW/sanguo/addr"
	"github.com/sniperHW/sanguo/codec/pb"
	"github.com/sniperHW/sanguo/codec/ss"
	"github.com/sniperHW/sanguo/discovery"
	"github.com/sniperHW/sanguo/logger/zap"
	"github.com/stretchr/testify/assert"
	"github.com/xtaci/smux"
	"google.golang.org/protobuf/proto"
)

type localDiscovery struct {
	nodes      map[addr.LogicAddr]*discovery.Node
	subscribes []func([]discovery.Node)
}

func (d *localDiscovery) LoadNodeInfo() (nodes []discovery.Node) {
	for _, v := range d.nodes {
		nodes = append(nodes, *v)
	}
	return nodes
}

// 订阅变更
func (d *localDiscovery) Subscribe(updateCB func([]discovery.Node)) error {
	d.subscribes = append(d.subscribes, updateCB)
	updateCB(d.LoadNodeInfo())
	return nil
}

func (d *localDiscovery) AddNode(n *discovery.Node) {
	d.nodes[n.Addr.LogicAddr()] = n
	nodes := d.LoadNodeInfo()
	for _, v := range d.subscribes {
		v(nodes)
	}
}

func (d *localDiscovery) RemoveNode(logicAddr addr.LogicAddr) {
	delete(d.nodes, logicAddr)
	nodes := d.LoadNodeInfo()
	for _, v := range d.subscribes {
		v(nodes)
	}
}

func (d *localDiscovery) ModifyNode(modify *discovery.Node) {
	if n, ok := d.nodes[modify.Addr.LogicAddr()]; ok {
		if n.Available != modify.Available || n.Addr.NetAddr() != modify.Addr.NetAddr() {
			logger.Debug("modify")
			d.nodes[modify.Addr.LogicAddr()] = modify
			nodes := d.LoadNodeInfo()
			for _, v := range d.subscribes {
				v(nodes)
			}
		}
	}
}

func init() {
	pb.Register(ss.Namespace, &ss.Echo{}, 1)
	l := zap.NewZapLogger("sanguo_test.log", "./logfile", "debug", 1024*1024*100, 14, 28, true)
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

	s := newSanguo(SanguoOption{
		RPCCodec: &JsonCodec{},
	})
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

	node1 := newSanguo(SanguoOption{
		RPCCodec: &JsonCodec{},
	})
	node1.RegisterMessageHandler(&ss.Echo{}, func(_ addr.LogicAddr, msg proto.Message) {
		logger.Debug(msg.(*ss.Echo).Msg)
	})

	node1.RegisterRPC("hello", func(_ context.Context, replyer *rpcgo.Replyer, arg *string) {
		logger.Debug("on hello call")
		replyer.Reply(fmt.Sprintf("hello world:%s", *arg), nil)
	})

	node2 := newSanguo(SanguoOption{
		RPCCodec: &JsonCodec{},
	})
	err := node2.Start(localDiscovery, node2Addr.LogicAddr())
	assert.Nil(t, err)

	logger.Debug("Start OK")

	var resp string
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*1)
	err = node2.Call(ctx, node1Addr.LogicAddr(), "hello", "sniperHW", &resp)
	cancel()
	logger.Debug(err)

	err = node1.Start(localDiscovery, node1Addr.LogicAddr())
	assert.Nil(t, err)

	node2.SendMessage(node1Addr.LogicAddr(), &ss.Echo{
		Msg: "hello",
	})

	//var resp string
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

	node1 := newSanguo(SanguoOption{
		RPCCodec: &JsonCodec{},
	})
	node1.RegisterMessageHandler(&ss.Echo{}, func(_ addr.LogicAddr, msg proto.Message) {
		logger.Debug(msg.(*ss.Echo).Msg)
	})

	node1.RegisterRPC("hello", func(_ context.Context, replyer *rpcgo.Replyer, arg *string) {
		logger.Debug("on hello call")
		replyer.Reply(fmt.Sprintf("hello world:%s", *arg), nil)
	})

	err := node1.Start(localDiscovery, node1Addr.LogicAddr())
	assert.Nil(t, err)

	node2 := newSanguo(SanguoOption{
		RPCCodec: &JsonCodec{},
	})
	err = node2.Start(localDiscovery, node2Addr.LogicAddr())
	assert.Nil(t, err)

	harbor1 := newSanguo(SanguoOption{
		RPCCodec: &JsonCodec{},
	})
	err = harbor1.Start(localDiscovery, harbor1Addr.LogicAddr())
	assert.Nil(t, err)

	harbor2 := newSanguo(SanguoOption{
		RPCCodec: &JsonCodec{},
	})
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
	assert.Equal(t, "route message to target:1.1.1 failed", err.Error())
	logger.Debug(err)

	//将harbor1移除
	localDiscovery.RemoveNode(harbor1Addr.LogicAddr())

	c := make(chan struct{})
	node2.CallWithCallback(type1Addr, time.Now().Add(time.Second), "hello", "sniperHW", &resp, func(resp interface{}, err error) {
		assert.Equal(t, "route message to target:1.1.1 failed", err.Error())
		logger.Debug(err)
		close(c)
	})

	<-c

	node1.Stop()
	node2.Stop()
	harbor1.Stop()
	harbor2.Stop()

	node1.Wait()
	node2.Wait()
	harbor1.Wait()
	harbor2.Wait()
}

func TestStream(t *testing.T) {
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

	node1 := newSanguo(SanguoOption{
		RPCCodec: &JsonCodec{},
	})

	node1.OnNewStream(func(s *smux.Stream) {
		go func() {
			buff := make([]byte, 64)
			n, _ := s.Read(buff)
			s.Write(buff[:n])
			s.Close()
		}()
	})

	err := node1.Start(localDiscovery, node1Addr.LogicAddr())
	assert.Nil(t, err)

	node2 := newSanguo(SanguoOption{
		RPCCodec: &JsonCodec{},
	})
	err = node2.Start(localDiscovery, node2Addr.LogicAddr())
	assert.Nil(t, err)

	logger.Debug("Start OK")
	{
		ss, err := node2.OpenStream(node1Addr.LogicAddr())
		if err != nil {
			panic(err)
		}

		ss.Write([]byte("hello"))
		buff := make([]byte, 64)
		n, _ := ss.Read(buff)
		assert.Equal(t, "hello", string(buff[:n]))
		ss.Close()
	}

	{
		ss, err := node2.OpenStream(node1Addr.LogicAddr())
		if err != nil {
			panic(err)
		}

		ss.Write([]byte("hello"))
		buff := make([]byte, 64)
		n, _ := ss.Read(buff)
		assert.Equal(t, "hello", string(buff[:n]))
		ss.Close()
	}

	localDiscovery.RemoveNode(node1Addr.LogicAddr())
	node1.Wait()

	_, err = node2.GetAddrByType(1)
	assert.NotNil(t, err)

	localDiscovery.RemoveNode(node2Addr.LogicAddr())
	node2.Wait()

}

func TestDefault(t *testing.T) {
	localDiscovery := &localDiscovery{
		nodes: map[addr.LogicAddr]*discovery.Node{},
	}

	node1Addr, _ := addr.MakeAddr("1.1.1", "localhost:8110")
	node2Addr, _ := addr.MakeAddr("1.2.3", "localhost:8111")

	localDiscovery.AddNode(&discovery.Node{
		Addr:      node1Addr,
		Available: true,
	})

	localDiscovery.AddNode(&discovery.Node{
		Addr:      node2Addr,
		Available: true,
	})

	SetRpcCodec(&JsonCodec{})

	RegisterMessageHandler(&ss.Echo{}, func(_ addr.LogicAddr, msg proto.Message) {
		logger.Debug(msg.(*ss.Echo).Msg)
	})

	RegisterRPC("hello", func(_ context.Context, replyer *rpcgo.Replyer, arg *string) {
		logger.Debug("on hello call")
		replyer.Reply(fmt.Sprintf("hello world:%s", *arg), nil)
	})

	err := Start(localDiscovery, node1Addr.LogicAddr())
	assert.Nil(t, err)

	//向自身发送消息
	SendMessage(node1Addr.LogicAddr(), &ss.Echo{Msg: "hello"})

	//调用自身hello
	var resp string
	err = Call(context.TODO(), node1Addr.LogicAddr(), "hello", "sniperHW", &resp)
	assert.Nil(t, err)
	assert.Equal(t, resp, "hello world:sniperHW")

	CallWithCallback(node1Addr.LogicAddr(), time.Now().Add(time.Second), "hello", "sniperHW", &resp, func(resp interface{}, err error) {
		logger.Debug(resp)
	})

	_, err = OpenStream(node1Addr.LogicAddr())
	assert.Equal(t, err.Error(), "cant't open stream to self")

	node2 := newSanguo(SanguoOption{
		RPCCodec: &JsonCodec{},
	})
	err = node2.Start(localDiscovery, node2Addr.LogicAddr())
	assert.Nil(t, err)

	Log().Debug("Start OK")

	node2.SendMessage(node1Addr.LogicAddr(), &ss.Echo{
		Msg: "hello",
	})

	node3Addr, _ := addr.MakeAddr("1.2.1", "localhost:8113")

	localDiscovery.AddNode(&discovery.Node{
		Addr:      node3Addr,
		Available: true,
	})

	time.Sleep(time.Second)

	localDiscovery.ModifyNode(&discovery.Node{
		Addr:      node3Addr,
		Available: false,
	})

	time.Sleep(time.Second)
	//变更网络地址
	node3Addr, _ = addr.MakeAddr("1.2.1", "localhost:8114")
	localDiscovery.ModifyNode(&discovery.Node{
		Addr:      node3Addr,
		Available: false,
	})

	//恢复available
	time.Sleep(time.Second)
	localDiscovery.ModifyNode(&discovery.Node{
		Addr:      node3Addr,
		Available: true,
	})

	time.Sleep(time.Second)
	//移除node3
	localDiscovery.RemoveNode(node3Addr.LogicAddr())

	time.Sleep(time.Second)

	node4Addr, _ := addr.MakeAddr("1.2.5", "localhost:8115")
	localDiscovery.AddNode(&discovery.Node{
		Addr:      node4Addr,
		Available: true,
	})

	time.Sleep(time.Second)
	localDiscovery.RemoveNode(node4Addr.LogicAddr())

	//var resp string
	err = node2.Call(context.TODO(), node1Addr.LogicAddr(), "hello", "sniperHW", &resp)
	assert.Nil(t, err)
	assert.Equal(t, resp, "hello world:sniperHW")

	localDiscovery.RemoveNode(node1Addr.LogicAddr())
	Wait()

	_, err = node2.GetAddrByType(1)
	assert.NotNil(t, err)

	localDiscovery.RemoveNode(node2Addr.LogicAddr())
	node2.Wait()
}

func TestBiDirectionDial(t *testing.T) {
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

	node1 := newSanguo(SanguoOption{
		RPCCodec: &JsonCodec{},
	})

	var wait sync.WaitGroup

	wait.Add(2)

	node1.RegisterMessageHandler(&ss.Echo{}, func(from addr.LogicAddr, msg proto.Message) {
		logger.Debug("message from ", from.String())
		wait.Done()
	})

	node2 := newSanguo(SanguoOption{
		RPCCodec: &JsonCodec{},
	})

	node2.RegisterMessageHandler(&ss.Echo{}, func(from addr.LogicAddr, msg proto.Message) {
		logger.Debug("message from ", from.String())
		wait.Done()
	})

	err := node1.Start(localDiscovery, node1Addr.LogicAddr())
	assert.Nil(t, err)

	err = node2.Start(localDiscovery, node2Addr.LogicAddr())
	assert.Nil(t, err)

	logger.Debug("Start OK")

	go func() {
		node2.SendMessage(node1Addr.LogicAddr(), &ss.Echo{
			Msg: "hello",
		})
	}()

	go func() {
		node1.SendMessage(node2Addr.LogicAddr(), &ss.Echo{
			Msg: "hello",
		})
	}()

	wait.Wait()

	localDiscovery.RemoveNode(node1Addr.LogicAddr())
	node1.Wait()

	localDiscovery.RemoveNode(node2Addr.LogicAddr())
	node2.Wait()

}
