package clustergo

//go test -race -covermode=atomic -v -coverprofile=coverage.out -run=.
//go tool cover -html=coverage.out

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sniperHW/clustergo/addr"
	"github.com/sniperHW/clustergo/codec/pb"
	"github.com/sniperHW/clustergo/codec/ss"
	"github.com/sniperHW/clustergo/discovery"
	"github.com/sniperHW/clustergo/logger/zap"
	"github.com/sniperHW/rpcgo"
	"github.com/stretchr/testify/assert"
	"github.com/xtaci/smux"
	"google.golang.org/protobuf/proto"
)

type localDiscovery struct {
	nodes      map[addr.LogicAddr]*discovery.Node
	subscribes []func(discovery.DiscoveryInfo)
}

// 订阅变更
func (d *localDiscovery) Subscribe(updateCB func(discovery.DiscoveryInfo)) error {
	d.subscribes = append(d.subscribes, updateCB)
	i := discovery.DiscoveryInfo{}
	for _, v := range d.nodes {
		i.Add = append(i.Add, *v)
	}
	updateCB(i)
	return nil
}

func (d *localDiscovery) AddNode(n *discovery.Node) {
	d.nodes[n.Addr.LogicAddr()] = n
	add := discovery.DiscoveryInfo{
		Add: []discovery.Node{*n},
	}
	for _, v := range d.subscribes {
		v(add)
	}
}

func (d *localDiscovery) RemoveNode(logicAddr addr.LogicAddr) {
	if n := d.nodes[logicAddr]; n != nil {
		delete(d.nodes, logicAddr)
		remove := discovery.DiscoveryInfo{
			Remove: []discovery.Node{*n},
		}
		for _, v := range d.subscribes {
			v(remove)
		}
	}
}

func (d *localDiscovery) ModifyNode(modify *discovery.Node) {
	if n, ok := d.nodes[modify.Addr.LogicAddr()]; ok {
		if n.Available != modify.Available || n.Addr.NetAddr() != modify.Addr.NetAddr() {
			logger.Debug("modify")
			d.nodes[modify.Addr.LogicAddr()] = modify
			//nodes := d.LoadNodeInfo()
			update := discovery.DiscoveryInfo{
				Update: []discovery.Node{*modify},
			}

			for _, v := range d.subscribes {
				v(update)
			}
		}
	}
}

func (d *localDiscovery) Close() {

}

func init() {
	pb.Register(ss.Namespace, &ss.Echo{}, 1)
	l := zap.NewZapLogger("sanguo_test.log", "./logfile", "debug", 1024*1024*100, 14, 28, true)
	InitLogger(l.Sugar())
}

func TestBenchmarkRPC(t *testing.T) {
	localDiscovery := &localDiscovery{
		nodes: map[addr.LogicAddr]*discovery.Node{},
	}

	node1Addr, _ := addr.MakeAddr("1.1.1", "localhost:18110")
	node2Addr, _ := addr.MakeAddr("1.2.1", "localhost:18111")

	localDiscovery.AddNode(&discovery.Node{
		Addr:      node1Addr,
		Available: true,
	})

	localDiscovery.AddNode(&discovery.Node{
		Addr:      node2Addr,
		Available: true,
	})

	node1 := newNode(JsonCodec{})
	node1.RegisterPbHandler(&ss.Echo{}, func(_ addr.LogicAddr, msg proto.Message) {
		logger.Debug(msg.(*ss.Echo).Msg)
	})

	node1.RegisterRPC("hello", func(_ context.Context, replyer *rpcgo.Replyer, arg *string) {
		replyer.Reply(fmt.Sprintf("hello world:%s", *arg))
	})

	node2 := newNode(JsonCodec{})
	err := node2.Start(localDiscovery, node2Addr.LogicAddr())
	assert.Nil(t, err)

	logger.Debug("Start OK")

	var resp string

	err = node1.Start(localDiscovery, node1Addr.LogicAddr())
	assert.Nil(t, err)

	node2.Call(context.TODO(), node1Addr.LogicAddr(), "hello", "sniperHW", &resp)

	begtime := time.Now()

	counter := int32(0)

	var wait sync.WaitGroup

	for i := 0; i < 25; i++ {
		wait.Add(1)
		go func() {
			for atomic.AddInt32(&counter, 1) <= 100000 {
				node2.Call(context.TODO(), node1Addr.LogicAddr(), "hello", "sniperHW", &resp)
			}
			wait.Done()
		}()
	}

	wait.Wait()

	fmt.Println("10W call,use:", time.Now().Sub(begtime))

	localDiscovery.RemoveNode(node1Addr.LogicAddr())
	node1.Wait()

	_, err = node2.GetAddrByType(1)
	assert.NotNil(t, err)

	localDiscovery.RemoveNode(node2Addr.LogicAddr())
	node2.Wait()
}

func TestSingleNode(t *testing.T) {
	localDiscovery := &localDiscovery{
		nodes: map[addr.LogicAddr]*discovery.Node{},
	}

	localAddr, _ := addr.MakeAddr("1.1.1", "localhost:18110")

	localDiscovery.AddNode(&discovery.Node{
		Addr:      localAddr,
		Available: true,
	})

	s := newNode(JsonCodec{})

	assert.NotNil(t, s.Stop())
	assert.NotNil(t, s.Wait())
	assert.NotNil(t, s.SendPbMessage(localAddr.LogicAddr(), &ss.Echo{
		Msg: "hello",
	}))

	s.RegisterPbHandler(&ss.Echo{}, func(_ addr.LogicAddr, msg proto.Message) {
		logger.Debug(msg.(*ss.Echo).Msg)
	})

	s.RegisterRPC("hello", func(_ context.Context, replyer *rpcgo.Replyer, arg *string) {
		logger.Debugf("on hello call,channel:%s", replyer.Channel().Name())
		replyer.Reply(fmt.Sprintf("hello world:%s", *arg))
	})

	err := s.Start(localDiscovery, localAddr.LogicAddr())
	assert.Nil(t, err)

	s.SendPbMessage(localAddr.LogicAddr(), &ss.Echo{
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

	node1Addr, _ := addr.MakeAddr("1.1.1", "localhost:18110")
	node2Addr, _ := addr.MakeAddr("1.2.1", "localhost:18111")

	localDiscovery.AddNode(&discovery.Node{
		Addr:      node1Addr,
		Available: true,
	})

	localDiscovery.AddNode(&discovery.Node{
		Addr:      node2Addr,
		Available: true,
	})

	node1 := newNode(JsonCodec{})
	node1.RegisterPbHandler(&ss.Echo{}, func(_ addr.LogicAddr, msg proto.Message) {
		logger.Debug(msg.(*ss.Echo).Msg)
	})

	node1.RegisterRPC("hello", func(_ context.Context, replyer *rpcgo.Replyer, arg *string) {
		logger.Debugf("on hello call,channel:%s", replyer.Channel().Name())
		replyer.Reply(fmt.Sprintf("hello world:%s", *arg))
	})

	node2 := newNode(JsonCodec{})
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

	node2.SendPbMessage(node1Addr.LogicAddr(), &ss.Echo{
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
	node1Addr, _ := addr.MakeAddr("1.1.1", "localhost:18110")
	harbor1Addr, _ := addr.MakeHarborAddr("1.255.1", "localhost:19110")

	//cluster:2
	node2Addr, _ := addr.MakeAddr("2.2.1", "localhost:18111")
	harbor2Addr, _ := addr.MakeHarborAddr("2.255.1", "localhost:19111")

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

	node1 := newNode(&JsonCodec{})
	node1.RegisterPbHandler(&ss.Echo{}, func(_ addr.LogicAddr, msg proto.Message) {
		logger.Debug(msg.(*ss.Echo).Msg)
	})

	node1.RegisterRPC("hello", func(_ context.Context, replyer *rpcgo.Replyer, arg *string) {
		logger.Debugf("on hello call,channel:%s", replyer.Channel().Name())
		replyer.Reply(fmt.Sprintf("hello world:%s", *arg))
	})

	err := node1.Start(localDiscovery, node1Addr.LogicAddr())
	assert.Nil(t, err)

	node2 := newNode(JsonCodec{})
	err = node2.Start(localDiscovery, node2Addr.LogicAddr())
	assert.Nil(t, err)

	harbor1 := newNode(JsonCodec{})
	err = harbor1.Start(localDiscovery, harbor1Addr.LogicAddr())
	assert.Nil(t, err)

	harbor2 := newNode(JsonCodec{})
	err = harbor2.Start(localDiscovery, harbor2Addr.LogicAddr())
	assert.Nil(t, err)

	var type1Addr addr.LogicAddr

	for {
		if type1Addr, err = node2.GetAddrByType(1); err == nil {
			break
		}
	}

	node1.SendPbMessage(type1Addr, &ss.Echo{
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

	node1Addr, _ := addr.MakeAddr("1.1.1", "localhost:18110")
	node2Addr, _ := addr.MakeAddr("1.2.1", "localhost:18111")

	localDiscovery.AddNode(&discovery.Node{
		Addr:      node1Addr,
		Available: true,
	})

	localDiscovery.AddNode(&discovery.Node{
		Addr:      node2Addr,
		Available: true,
	})

	node1 := newNode(JsonCodec{})

	node1.StartSmuxServer(func(s *smux.Stream) {
		go func() {
			buff := make([]byte, 64)
			n, _ := s.Read(buff)
			s.Write(buff[:n])
			s.Close()
		}()
	})

	err := node1.Start(localDiscovery, node1Addr.LogicAddr())
	assert.Nil(t, err)

	node2 := newNode(JsonCodec{})
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

func TestBiDirectionDial(t *testing.T) {
	localDiscovery := &localDiscovery{
		nodes: map[addr.LogicAddr]*discovery.Node{},
	}

	node1Addr, _ := addr.MakeAddr("1.1.1", "localhost:18110")
	node2Addr, _ := addr.MakeAddr("1.2.1", "localhost:18111")

	localDiscovery.AddNode(&discovery.Node{
		Addr:      node1Addr,
		Available: true,
	})

	localDiscovery.AddNode(&discovery.Node{
		Addr:      node2Addr,
		Available: true,
	})

	node1 := newNode(JsonCodec{})

	var wait sync.WaitGroup

	wait.Add(2)

	node1.RegisterPbHandler(&ss.Echo{}, func(from addr.LogicAddr, msg proto.Message) {
		logger.Debug("message from ", from.String())
		wait.Done()
	})

	node2 := newNode(JsonCodec{})

	node2.RegisterPbHandler(&ss.Echo{}, func(from addr.LogicAddr, msg proto.Message) {
		logger.Debug("message from ", from.String())
		wait.Done()
	})

	err := node1.Start(localDiscovery, node1Addr.LogicAddr())
	assert.Nil(t, err)

	err = node2.Start(localDiscovery, node2Addr.LogicAddr())
	assert.Nil(t, err)

	logger.Debug("Start OK")

	//双方同时向对方发消息，使得两边同时dial,最终两节点之间应该只能成功建立一条通信连接
	go func() {
		node2.SendPbMessage(node1Addr.LogicAddr(), &ss.Echo{
			Msg: "hello",
		})
	}()

	go func() {
		node1.SendPbMessage(node2Addr.LogicAddr(), &ss.Echo{
			Msg: "hello",
		})
	}()

	wait.Wait()

	localDiscovery.RemoveNode(node1Addr.LogicAddr())
	node1.Wait()

	localDiscovery.RemoveNode(node2Addr.LogicAddr())
	node2.Wait()

}

func TestDefault(t *testing.T) {

	RPCCodec = JsonCodec{}

	localDiscovery := &localDiscovery{
		nodes: map[addr.LogicAddr]*discovery.Node{},
	}

	node1Addr, _ := addr.MakeAddr("1.1.1", "localhost:18110")
	node2Addr, _ := addr.MakeAddr("1.2.3", "localhost:18111")

	localDiscovery.AddNode(&discovery.Node{
		Addr:      node1Addr,
		Available: true,
	})

	localDiscovery.AddNode(&discovery.Node{
		Addr:      node2Addr,
		Available: true,
	})

	RegisterPbMessageHandler(&ss.Echo{}, func(_ addr.LogicAddr, msg proto.Message) {
		logger.Debug(msg.(*ss.Echo).Msg)
	})

	RegisterRPC("hello", func(_ context.Context, replyer *rpcgo.Replyer, arg *string) {
		logger.Debugf("on hello call,channel:%s", replyer.Channel().Name())
		replyer.Reply(fmt.Sprintf("hello world:%s", *arg))
	})

	OnBinaryMessage(func(_ addr.LogicAddr, msg []byte) {
		logger.Debug(string(msg))
	})

	err := Start(localDiscovery, node1Addr.LogicAddr())
	assert.Nil(t, err)

	//向自身发送消息
	SendPbMessage(node1Addr.LogicAddr(), &ss.Echo{Msg: "hello"})

	SendBinMessage(node1Addr.LogicAddr(), []byte("binMessage"))

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

	node2 := newNode(JsonCodec{})
	err = node2.Start(localDiscovery, node2Addr.LogicAddr())
	assert.Nil(t, err)

	Log().Debug("Start OK")

	node2.SendPbMessage(node1Addr.LogicAddr(), &ss.Echo{
		Msg: "hello",
	})

	logger.Debug("send bin")
	node2.SendBinMessage(node1Addr.LogicAddr(), []byte("binMessage"))

	node3Addr, _ := addr.MakeAddr("1.2.1", "localhost:18113")

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
	node3Addr, _ = addr.MakeAddr("1.2.1", "localhost:18114")
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

	node4Addr, _ := addr.MakeAddr("1.2.5", "localhost:18115")
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
