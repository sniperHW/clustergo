package cluster

//go test -covermode=count -v -coverprofile=coverage.out -run=.
//go tool cover -html=coverage.out

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/rpc"
	"github.com/sniperHW/kendynet/timer"
	center "github.com/sniperHW/sanguo/center"
	center_proto "github.com/sniperHW/sanguo/center/protocol"
	"github.com/sniperHW/sanguo/cluster/addr"
	cluster_proto "github.com/sniperHW/sanguo/cluster/proto"
	"github.com/sniperHW/sanguo/cluster/rpcerr"
	"github.com/sniperHW/sanguo/common"
	"github.com/sniperHW/sanguo/protocol/cmdEnum"
	_ "github.com/sniperHW/sanguo/protocol/ss" //触发pb注册
	ss_rpc "github.com/sniperHW/sanguo/protocol/ss/rpc"
	ss_msg "github.com/sniperHW/sanguo/protocol/ss/ssmessage"
	"github.com/sniperHW/sanguo/util"
	"github.com/stretchr/testify/assert"
	"math/rand"
	"testing"
	"time"
)

var centerAddr string = "127.0.0.1:8000"

func init() {
	logger = util.NewLogger("./", "test_cluster", 1024*1024*50)
	kendynet.InitLogger(logger)
	InitLogger(logger)
	rand.Seed(time.Now().Unix())
}

func TestDiff2(t *testing.T) {
	{
		a := []uint32{uint32(1), uint32(2)}
		b := []uint32{}

		add, remove := diff2(a, b)

		assert.Equal(t, 0, len(remove))
		assert.Equal(t, 2, len(add))
	}

	{
		a := []uint32{}
		b := []uint32{uint32(1), uint32(2)}

		add, remove := diff2(a, b)

		assert.Equal(t, 0, len(add))
		assert.Equal(t, 2, len(remove))
	}

	{
		a := []uint32{1, 3, 4}
		b := []uint32{uint32(1), uint32(2)}

		add, remove := diff2(a, b)

		assert.Equal(t, 3, len(add))
		assert.Equal(t, 1, len(remove))

		assert.Equal(t, uint32(1), add[0])
		assert.Equal(t, uint32(3), add[1])
		assert.Equal(t, uint32(4), add[2])
		assert.Equal(t, uint32(2), remove[0])
	}

	{
		a := []uint32{1, 2}
		b := []uint32{1, 3, 4}

		add, remove := diff2(a, b)

		assert.Equal(t, 2, len(add))
		assert.Equal(t, 2, len(remove))

		assert.Equal(t, uint32(1), add[0])
		assert.Equal(t, uint32(2), add[1])
		assert.Equal(t, uint32(3), remove[0])
		assert.Equal(t, uint32(4), remove[1])
	}

}

func TestDiff(t *testing.T) {

	{
		a := []*center_proto.NodeInfo{
			&center_proto.NodeInfo{LogicAddr: proto.Uint32(uint32(1))},
			&center_proto.NodeInfo{LogicAddr: proto.Uint32(uint32(2))},
			&center_proto.NodeInfo{LogicAddr: proto.Uint32(uint32(3))},
			&center_proto.NodeInfo{LogicAddr: proto.Uint32(uint32(4))},
			&center_proto.NodeInfo{LogicAddr: proto.Uint32(uint32(5))},
			&center_proto.NodeInfo{LogicAddr: proto.Uint32(uint32(6))},
			&center_proto.NodeInfo{LogicAddr: proto.Uint32(uint32(7))},
		}

		b := []*center_proto.NodeInfo{
			&center_proto.NodeInfo{LogicAddr: proto.Uint32(uint32(3))},
			&center_proto.NodeInfo{LogicAddr: proto.Uint32(uint32(5))},
		}

		addI := []uint32{1, 2, 3, 4, 5, 6, 7}
		add, remove := diff(a, b)

		assert.Equal(t, len(add), len(addI))

		for k, v := range add {
			assert.Equal(t, v.GetLogicAddr(), addI[k])
		}

		assert.Equal(t, len(remove), 0)

	}

	{
		a := []*center_proto.NodeInfo{}
		b := []*center_proto.NodeInfo{
			&center_proto.NodeInfo{LogicAddr: proto.Uint32(uint32(3))},
			&center_proto.NodeInfo{LogicAddr: proto.Uint32(uint32(5))},
		}

		removeI := []uint32{3, 5}

		add, remove := diff(a, b)

		assert.Equal(t, len(add), 0)

		assert.Equal(t, len(remove), len(removeI))

		for k, v := range remove {
			assert.Equal(t, v.GetLogicAddr(), removeI[k])
		}

	}

	{
		a := []*center_proto.NodeInfo{
			&center_proto.NodeInfo{LogicAddr: proto.Uint32(uint32(1))},
			&center_proto.NodeInfo{LogicAddr: proto.Uint32(uint32(3))},
			&center_proto.NodeInfo{LogicAddr: proto.Uint32(uint32(5))},
			&center_proto.NodeInfo{LogicAddr: proto.Uint32(uint32(7))},
		}

		b := []*center_proto.NodeInfo{
			&center_proto.NodeInfo{LogicAddr: proto.Uint32(uint32(5))},
			&center_proto.NodeInfo{LogicAddr: proto.Uint32(uint32(3))},
			&center_proto.NodeInfo{LogicAddr: proto.Uint32(uint32(6))},
		}

		add, remove := diff(a, b)

		addI := []uint32{1, 3, 5, 7}
		removeI := []uint32{6}

		assert.Equal(t, len(add), len(addI))

		for k, v := range add {
			assert.Equal(t, v.GetLogicAddr(), addI[k])
		}

		assert.Equal(t, len(remove), len(removeI))

		for k, v := range remove {
			assert.Equal(t, v.GetLogicAddr(), removeI[k])
		}

	}
}

type uniLocker struct {
	lockReturnFalse bool
}

func (this uniLocker) Lock(_ addr.Addr) bool {
	if this.lockReturnFalse {
		return false
	} else {
		return true
	}
}

func (this uniLocker) Unlock() {

}

func TestMsgManager(t *testing.T) {
	node1 := NewCluster()
	node1.Register(uint16(1), nil)
	node1.Register(uint16(1), func(from addr.LogicAddr, msg proto.Message) {
		fmt.Println("1 func")
	})
	node1.Register(uint16(1), func(from addr.LogicAddr, msg proto.Message) {})
	node1.SetPeerDisconnected(func(addr.LogicAddr, error) { panic("test error") })
	node1.onPeerDisconnected(addr.LogicAddr(0), nil)
	node1.dispatch(addr.LogicAddr(0), nil, uint16(1), nil)

	node1.Register(uint16(2), func(from addr.LogicAddr, msg proto.Message) {
		panic("test error")
	})

	node1.msgMgr.dispatch(addr.LogicAddr(0), uint16(2), nil)

	node1.msgMgr.dispatch(addr.LogicAddr(0), uint16(3), nil)

}

func TestEndPoint(t *testing.T) {

	{
		center1 := center.New()

		go func() {
			center1.Start(centerAddr, logger)
			fmt.Println("center1 stop")
		}()

		back := common.HeartBeat_Timeout

		common.HeartBeat_Timeout = time.Second * 6

		//测试连接通道空闲超时关闭

		node1 := NewCluster()

		node1.Register(cmdEnum.SS_Echo, func(from addr.LogicAddr, msg proto.Message) {
			fmt.Println("get echo from", from)
		})

		node2 := NewCluster()

		{
			addr_, _ := addr.MakeAddr("1.1.1", "127.0.0.1:8001")
			assert.Equal(t, nil, node1.Start([]string{centerAddr}, addr_, uniLocker{}))
		}

		{
			addr_, _ := addr.MakeAddr("1.2.2", "127.0.0.1:8002")
			assert.Equal(t, nil, node2.Start([]string{centerAddr}, addr_, uniLocker{}))
		}

		node1Addr, _ := addr.MakeLogicAddr("1.1.1")

		//等待从center接收到node1的信息
		for nil == node2.serviceMgr.getEndPoint(node1Addr) {
			time.Sleep(time.Second)
		}

		node2.PostMessage(node1Addr, &ss_msg.Echo{
			Message: proto.String("hello"),
		})

		time.Sleep((common.HeartBeat_Timeout * 2))

		var s kendynet.StreamSession
		e := node2.serviceMgr.getEndPoint(node1Addr)
		e.Lock()
		s = e.session
		e.Unlock()

		assert.Nil(t, s)

		node1.Stop(nil)

		node1 = NewCluster()

		fmt.Println("---------------------- test1 -------------------------")

		{
			//用一个不同的物理地址启动实例
			addr_, _ := addr.MakeAddr("1.1.1", "127.0.0.1:8003")
			assert.Equal(t, nil, node1.Start([]string{centerAddr}, addr_, uniLocker{}))
		}

		time.Sleep(time.Second)

		fmt.Println("---------------------- test2 -------------------------")

		common.HeartBeat_Timeout = back

		back = delayRemoveEndPointTime

		delayRemoveEndPointTime = time.Second * 2

		center1.Stop()

		time.Sleep(time.Second * 2)

		//重启center
		center1 = center.New()

		go func() {
			center1.Start(centerAddr, logger)
		}()

		time.Sleep(delayRemoveEndPointTime * 3)

		fmt.Println("---------------------- test3 -------------------------")

		center1.Stop()

		node1.Stop(nil)

		time.Sleep(time.Second * 2)

		//再次重启center
		center1 = center.New()

		go func() {
			center1.Start(centerAddr, logger)
		}()

		time.Sleep(delayRemoveEndPointTime * 3)

		node2.Stop(nil)

		center1.Stop()

		delayRemoveEndPointTime = back

	}

}

func TestBothConnect(t *testing.T) {
	center1 := center.New()

	go func() {
		center1.Start(centerAddr, logger)
	}()

	node1 := NewCluster()
	node2 := NewCluster()

	cc1 := make(chan struct{})
	cc2 := make(chan struct{})

	node1.RegisterMethod(&ss_rpc.EchoReq{}, func(replyer *rpc.RPCReplyer, arg interface{}) {
		echoResp := &ss_rpc.EchoResp{}
		echoResp.Message = proto.String("world")
		replyer.Reply(echoResp, nil)
	})

	node2.RegisterMethod(&ss_rpc.EchoReq{}, func(replyer *rpc.RPCReplyer, arg interface{}) {
		echoResp := &ss_rpc.EchoResp{}
		echoResp.Message = proto.String("world")
		replyer.Reply(echoResp, nil)
	})

	{
		addr_, _ := addr.MakeAddr("1.1.1", "127.0.0.1:8001")
		assert.Equal(t, nil, node1.Start([]string{centerAddr}, addr_, uniLocker{}))
	}

	{
		addr_, _ := addr.MakeAddr("1.2.2", "127.0.0.1:8002")
		assert.Equal(t, nil, node2.Start([]string{centerAddr}, addr_, uniLocker{}))
	}

	for {
		_, err := node2.Random(1)
		if nil != err {
			time.Sleep(time.Second)
		} else {
			break
		}
	}

	for {
		_, err := node1.Random(2)
		if nil != err {
			time.Sleep(time.Second)
		} else {
			break
		}
	}

	go func() {
		nodeAddr, _ := addr.MakeLogicAddr("1.2.2")
		node1.AsynCall(nodeAddr, &ss_rpc.EchoReq{Message: proto.String("hello")}, 10000, func(result interface{}, err error) {
			close(cc1)
		})
	}()

	go func() {
		nodeAddr, _ := addr.MakeLogicAddr("1.1.1")
		node2.AsynCall(nodeAddr, &ss_rpc.EchoReq{Message: proto.String("hello")}, 10000, func(result interface{}, err error) {
			close(cc2)
		})
	}()

	<-cc1
	<-cc2

	node1.Stop(nil)
	node2.Stop(nil)
	center1.Stop()

}

func TestPostMessage(t *testing.T) {

	center1 := center.New()

	go func() {
		center1.Start(centerAddr, logger)
	}()

	node1 := NewCluster()

	cc := make(chan struct{})

	node1.Register(cmdEnum.SS_Echo, func(from addr.LogicAddr, msg proto.Message) {

		if from != node1.SelfAddr().Logic {
			fmt.Println("get echo from", from)
			echo := &ss_msg.Echo{}
			echo.Message = proto.String("world")
			node1.PostMessage(from, echo)
		}
	})

	node2 := NewCluster()

	node2.Register(cmdEnum.SS_Echo, func(from addr.LogicAddr, msg proto.Message) {
		fmt.Println("get echo resp from", from)
		cc <- struct{}{}
	})

	{
		n := NewCluster()
		addr_, _ := addr.MakeAddr("1.1.1", "127.0.0.1:8001")
		assert.NotNil(t, n.Start([]string{centerAddr}, addr_, uniLocker{lockReturnFalse: true}))

	}

	{

		assert.Equal(t, ERR_SERVERADDR_ZERO, node1.Start([]string{centerAddr}, addr.Addr{}, uniLocker{}))

		addr_, _ := addr.MakeAddr("1.1.1", "127.0.0.1:8001")
		assert.Equal(t, nil, node1.Start([]string{centerAddr}, addr_, uniLocker{}))

		assert.Equal(t, ERR_STARTED, node1.Start([]string{centerAddr}, addr_, uniLocker{}))

		node1.GetEventQueue()

		node1.PostTask(func() { fmt.Println("test") })

		node1.SelfAddr()
	}

	node2.PostMessage(addr.LogicAddr(0), &ss_msg.Echo{
		Message: proto.String("hello"),
	})

	{
		addr1, _ := addr.MakeLogicAddr("1.1.10")
		node1.PostMessage(addr1, &ss_msg.Echo{
			Message: proto.String("hello"),
		})

		addr2, _ := addr.MakeLogicAddr("2.1.10")
		node1.PostMessage(addr2, &ss_msg.Echo{
			Message: proto.String("hello"),
		})

	}

	{
		addr_, _ := addr.MakeAddr("1.2.2", "127.0.0.1:8002")
		assert.Equal(t, nil, node2.Start([]string{centerAddr}, addr_, uniLocker{}))
	}

	node2.PostMessage(addr.LogicAddr(0), &ss_msg.Echo{
		Message: proto.String("hello"),
	})

	node1Addr, _ := addr.MakeLogicAddr("1.1.1")

	//等待从center接收到node1的信息
	for nil == node2.serviceMgr.getEndPoint(node1Addr) {
		time.Sleep(time.Second)
	}

	node2.PostMessage(node1Addr, &ss_msg.Echo{
		Message: proto.String("hello"),
	})

	//向自己发
	node1.PostMessage(node1Addr, &ss_msg.Echo{
		Message: proto.String("hello"),
	})

	_ = <-cc

	node1Addr, err := node2.Random(1)

	assert.Equal(t, nil, err)

	node2.PostMessage(node1Addr, &ss_msg.Echo{
		Message: proto.String("hello"),
	})

	_ = <-cc

	node1Addr, err = node2.Mod(1, 10)

	assert.Equal(t, nil, err)

	node2.PostMessage(node1Addr, &ss_msg.Echo{
		Message: proto.String("hello"),
	})

	_ = <-cc

	addrs, err := node1.Select(1)

	assert.Equal(t, nil, err)

	node2.PostMessage(addrs[0], &ss_msg.Echo{
		Message: proto.String("hello"),
	})

	_ = <-cc

	node2.Brocast(1, &ss_msg.Echo{
		Message: proto.String("hello"),
	})

	_ = <-cc

	node2.BrocastToAll(&ss_msg.Echo{
		Message: proto.String("hello"),
	}, 2)

	_ = <-cc

	node1.Stop(func() {
		fmt.Println("test stop func")
	})

	node2.Stop(nil)

	center1.Stop()

}

func TestExportMethod(t *testing.T) {
	center1 := center.New()

	go func() {
		center1.Start(centerAddr, logger)
	}()

	node1 := NewCluster()

	cc := make(chan struct{})

	node1.RegisterMethod(&ss_rpc.EchoReq{}, func(replyer *rpc.RPCReplyer, arg interface{}) {
		echo := arg.(*ss_rpc.EchoReq)
		logger.Infof("echo message:%s\n", echo.GetMessage())
		echoResp := &ss_rpc.EchoResp{}
		echoResp.Message = proto.String("world")
		replyer.Reply(echoResp, nil)
	})

	node1.Register(cmdEnum.SS_Echo, func(from addr.LogicAddr, msg proto.Message) {
		if from != node1.SelfAddr().Logic {
			fmt.Println("get echo from", from)
			echo := &ss_msg.Echo{}
			echo.Message = proto.String("world")
			node1.PostMessage(from, echo)
		}
	})

	Register(cmdEnum.SS_Echo, func(from addr.LogicAddr, msg proto.Message) {
		fmt.Println("get echo resp from", from)
		cc <- struct{}{}
	})

	RegisterMethod(&ss_rpc.EchoReq{}, func(replyer *rpc.RPCReplyer, arg interface{}) {
		echo := arg.(*ss_rpc.EchoReq)
		logger.Infof("echo message:%s\n", echo.GetMessage())
		echoResp := &ss_rpc.EchoResp{}
		echoResp.Message = proto.String("world")
		replyer.Reply(echoResp, nil)
	})

	node1Addr, _ := addr.MakeLogicAddr("1.1.1")

	AsynCall(node1Addr, &ss_rpc.EchoReq{Message: proto.String("hello")}, 10000, func(result interface{}, err error) {
	})

	{
		addr_, _ := addr.MakeAddr("1.1.1", "127.0.0.1:8001")
		assert.Equal(t, nil, node1.Start([]string{centerAddr}, addr_, uniLocker{}))
	}

	{
		addr_, _ := addr.MakeAddr("1.2.2", "127.0.0.1:8002")
		assert.Equal(t, nil, Start([]string{centerAddr}, addr_, uniLocker{}))
	}

	SelfAddr()
	GetEventQueue()

	assert.Equal(t, false, IsStoped())

	AsynCall(addr.LogicAddr(0), &ss_rpc.EchoReq{Message: proto.String("hello")}, 10000, func(result interface{}, err error) {
		cc <- struct{}{}
	})

	_ = <-cc

	//等待从center接收到node1的信息
	for {
		_, err := Random(1)
		if nil != err {
			time.Sleep(time.Second)
		} else {
			break
		}
	}

	_, err := Mod(1, 1)

	assert.Equal(t, nil, err)

	e, _ := Select(1)
	assert.Equal(t, 1, len(e))

	fmt.Println("test 1")

	PostMessage(node1Addr, &cluster_proto.Heartbeat{})

	PostMessage(node1Addr, &ss_msg.Echo{
		Message: proto.String("hello"),
	})

	_ = <-cc

	Brocast(1, &ss_msg.Echo{
		Message: proto.String("hello"),
	})

	_ = <-cc

	BrocastToAll(&ss_msg.Echo{
		Message: proto.String("hello"),
	}, 2)

	_ = <-cc

	fmt.Println("test 2")

	//异步调用
	AsynCall(node1Addr, &ss_rpc.EchoReq{Message: proto.String("hello")}, 10000, func(result interface{}, err error) {
		assert.Equal(t, result.(*ss_rpc.EchoResp).GetMessage(), "world")
		cc <- struct{}{}
	})

	_ = <-cc

	{
		nodeAddr, _ := addr.MakeLogicAddr("2.1.1")
		AsynCall(nodeAddr, &ss_rpc.EchoReq{Message: proto.String("hello")}, 10000, func(result interface{}, err error) {
			cc <- struct{}{}
		})

		_ = <-cc
	}

	{
		nodeAddr, _ := addr.MakeLogicAddr("1.1.2")
		AsynCall(nodeAddr, &ss_rpc.EchoReq{Message: proto.String("hello")}, 10000, func(result interface{}, err error) {
			cc <- struct{}{}
		})

		_ = <-cc
	}

	PostTask(func() {
		cc <- struct{}{}
	})

	_ = <-cc

	RegisterTimerOnce(time.Second, func(*timer.Timer, interface{}) {
		cc <- struct{}{}
	}, nil)

	_ = <-cc

	RegisterTimer(time.Second, func(t *timer.Timer, _ interface{}) {
		t.Cancel()
		cc <- struct{}{}
	}, nil)

	_ = <-cc

	node1.Stop(nil)

	SetPeerDisconnected(func(addr.LogicAddr, error) {
		cc <- struct{}{}
	})
	_ = <-cc

	Stop(nil)

	center1.Stop()

}

func TestRPC(t *testing.T) {

	center1 := center.New()

	go func() {
		center1.Start(centerAddr, logger)
	}()

	node1 := NewCluster()

	cc := make(chan struct{})

	c := 0

	node1.RegisterMethod(&ss_rpc.EchoReq{}, func(replyer *rpc.RPCReplyer, arg interface{}) {
		c++
		if c == 2 {
			//断掉连接，测试SendResponse中如连接中断尝试建立再发response的功能
			node2Addr, _ := addr.MakeLogicAddr("1.2.2")

			//等待从center接收到node1的信息
			end := node1.serviceMgr.getEndPoint(node2Addr)

			end.closeSession("test")

		}
		echo := arg.(*ss_rpc.EchoReq)
		logger.Infof("echo message:%s\n", echo.GetMessage())
		echoResp := &ss_rpc.EchoResp{}
		echoResp.Message = proto.String("world")
		replyer.Reply(echoResp, nil)
	})

	node2 := NewCluster()

	{
		addr_, _ := addr.MakeAddr("1.1.1", "127.0.0.1:8001")
		assert.Equal(t, nil, node1.Start([]string{centerAddr}, addr_, uniLocker{}))
	}

	{
		addr_, _ := addr.MakeAddr("1.2.2", "127.0.0.1:8002")
		assert.Equal(t, nil, node2.Start([]string{centerAddr}, addr_, uniLocker{}))
	}

	node1Addr, _ := addr.MakeLogicAddr("1.1.1")

	//等待从center接收到node1的信息
	for nil == node2.serviceMgr.getEndPoint(node1Addr) {
		time.Sleep(time.Second)
	}

	//异步调用
	node2.AsynCall(node1Addr, &ss_rpc.EchoReq{Message: proto.String("hello")}, 10000, func(result interface{}, err error) {
		assert.Equal(t, result.(*ss_rpc.EchoResp).GetMessage(), "world")
		cc <- struct{}{}
	})

	_ = <-cc

	//异步调用
	node2.AsynCall(node1Addr, &ss_rpc.EchoReq{Message: proto.String("hello")}, 10000, func(result interface{}, err error) {
		assert.Equal(t, rpc.ErrChannelDisconnected, err)
		cc <- struct{}{}
	})

	_ = <-cc

	node1.Stop(nil)

	node2.Stop(nil)

	center1.Stop()

}

func TestHarbor(t *testing.T) {

	testRPCTimeout = true

	center1 := center.New() //harbor center

	center2 := center.New() //group 1 center
	center3 := center.New() //group 2 center

	go func() {
		center1.Start("127.0.0.1:8000", logger)
	}()

	go func() {
		center2.Start("127.0.0.1:8001", logger)
	}()

	go func() {
		center3.Start("127.0.0.1:8002", logger)
	}()

	harbor1 := NewCluster()
	harbor2 := NewCluster()
	harbor3 := NewCluster()

	harbor1Addr, _ := addr.MakeHarborAddr("1.255.1", "127.0.0.1:8011")
	harbor2Addr, _ := addr.MakeHarborAddr("2.255.1", "127.0.0.1:8012")
	harbor3Addr, _ := addr.MakeHarborAddr("2.255.2", "127.0.0.1:8013")

	assert.Equal(t, nil, harbor1.Start([]string{"127.0.0.1:8000", "127.0.0.1:8001"}, harbor1Addr, uniLocker{}))

	assert.Equal(t, nil, harbor2.Start([]string{"127.0.0.1:8000", "127.0.0.1:8002"}, harbor2Addr, uniLocker{}))

	assert.Equal(t, nil, harbor3.Start([]string{"127.0.0.1:8000", "127.0.0.1:8002"}, harbor3Addr, uniLocker{}))

	node1 := NewCluster()

	node2 := NewCluster()

	node3 := NewCluster()

	node1.RegisterMethod(&ss_rpc.EchoReq{}, func(replyer *rpc.RPCReplyer, arg interface{}) {
		echo := arg.(*ss_rpc.EchoReq)
		logger.Infof("echo message:%s\n", echo.GetMessage())
		echoResp := &ss_rpc.EchoResp{}
		echoResp.Message = proto.String("world")
		replyer.Reply(echoResp, nil)
	})

	node1.Register(cmdEnum.SS_Echo, func(from addr.LogicAddr, msg proto.Message) {
		fmt.Println("get echo from", from)
		echo := &ss_msg.Echo{}
		echo.Message = proto.String("world")
		node1.PostMessage(from, echo)
	})

	cc := make(chan struct{})

	node2.Register(cmdEnum.SS_Echo, func(from addr.LogicAddr, msg proto.Message) {
		fmt.Println("get echo resp from", from)
		cc <- struct{}{}
	})

	//time.Sleep(time.Second * 2)

	node1Addr, _ := addr.MakeAddr("1.1.1", "127.0.0.1:8111")

	node2Addr, _ := addr.MakeAddr("2.2.1", "127.0.0.1:8112")

	assert.Equal(t, nil, node1.Start([]string{"127.0.0.1:8001"}, node1Addr, uniLocker{}, EXPORT))

	assert.Equal(t, nil, node2.Start([]string{"127.0.0.1:8002"}, node2Addr, uniLocker{}))

	var peer addr.LogicAddr

	for {
		peer, _ = node2.Random(1)
		if peer.Empty() {
			time.Sleep(time.Second)
		} else {
			break
		}
	}

	node2.AsynCall(peer, &ss_rpc.EchoReq{Message: proto.String("hello")}, 10000, func(result interface{}, err error) {
		assert.Equal(t, result.(*ss_rpc.EchoResp).GetMessage(), "world")
		cc <- struct{}{}
	})

	fmt.Println("test1------------------------")

	_ = <-cc

	node2.PostMessage(peer, &ss_msg.Echo{
		Message: proto.String("hello"),
	})

	_ = <-cc

	peer, _ = addr.MakeLogicAddr("1.1.0")

	node2.PostMessage(peer, &ss_msg.Echo{
		Message: proto.String("hello"),
	})

	fmt.Println("test2------------------------")

	_ = <-cc

	node1.Stop(nil, true)

	fmt.Println("test3------------------------")

	for {
		_, err := node2.Random(1)
		if nil == err {
			time.Sleep(time.Second)
		} else {
			assert.Equal(t, err, ERR_NO_AVAILABLE_SERVICE)
			break
		}
	}

	fmt.Println("test4------------------------")
	node2.AsynCall(peer, &ss_rpc.EchoReq{Message: proto.String("hello")}, 10000, func(result interface{}, err error) {
		assert.Equal(t, rpcerr.Err_RPC_RelayError, err)
		cc <- struct{}{}
	})

	fmt.Println("test5------------------------")

	_ = <-cc

	fmt.Println("test6------------------------")

	node3Addr, _ := addr.MakeAddr("1.2.1", "127.0.0.1:8113")

	assert.Equal(t, nil, node3.Start([]string{"127.0.0.1:8001"}, node3Addr, uniLocker{}, EXPORT))

	time.Sleep(time.Second * 2)

	fmt.Println("test7------------------------")

	node3.Stop(nil, true)

	fmt.Println("test8------------------------")

	time.Sleep(time.Second * 2)

	fmt.Println("test9------------------------")

	harbor3.Stop(nil, true)

	time.Sleep(time.Second * 2)

	fmt.Println("test10------------------------")

	node2.Stop(nil)

	fmt.Println("test11------------------------")

	harbor1.Stop(nil)

	fmt.Println("test12------------------------")
	harbor2.Stop(nil)

	fmt.Println("test13------------------------")
	center1.Stop()
	fmt.Println("test14------------------------")
	center2.Stop()
	fmt.Println("test15------------------------")
	center3.Stop()

	testRPCTimeout = false

}
