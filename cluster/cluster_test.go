package cluster

//go test -covermode=count -v -coverprofile=coverage.out -run=.
//go tool cover -html=coverage.out

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/rpc"
	center "github.com/sniperHW/sanguo/center"
	center_proto "github.com/sniperHW/sanguo/center/protocol"
	"github.com/sniperHW/sanguo/cluster/addr"
	"github.com/sniperHW/sanguo/cluster/rpcerr"
	"github.com/sniperHW/sanguo/protocol/cmdEnum"
	_ "github.com/sniperHW/sanguo/protocol/ss" //触发pb注册
	ss_rpc "github.com/sniperHW/sanguo/protocol/ss/rpc"
	ss_msg "github.com/sniperHW/sanguo/protocol/ss/ssmessage"
	"github.com/sniperHW/sanguo/util"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

var centerAddr string = "127.0.0.1:8000"

func init() {
	logger = util.NewLogger("./", "test_cluster", 1024*1024*50)
	kendynet.InitLogger(logger)
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

func TestPostMessage(t *testing.T) {

	center1 := center.New()

	go func() {
		center1.Start(centerAddr, logger)
	}()

	node1 := NewCluster()

	cc := make(chan struct{})

	node1.Register(cmdEnum.SS_Echo, func(from addr.LogicAddr, msg proto.Message) {
		fmt.Println("get echo from", from)
		echo := &ss_msg.Echo{}
		echo.Message = proto.String("world")
		node1.PostMessage(from, echo)
	})

	node2 := NewCluster()

	node2.Register(cmdEnum.SS_Echo, func(from addr.LogicAddr, msg proto.Message) {
		fmt.Println("get echo resp from", from)
		cc <- struct{}{}
	})

	{
		addr_, _ := addr.MakeAddr("1.1.1", "127.0.0.1:8001")
		assert.Equal(t, nil, node1.Start([]string{centerAddr}, addr_))
	}

	{
		addr_, _ := addr.MakeAddr("1.2.2", "127.0.0.1:8002")
		assert.Equal(t, nil, node2.Start([]string{centerAddr}, addr_))
	}

	node1Addr, _ := addr.MakeLogicAddr("1.1.1")

	//等待从center接收到node1的信息
	for nil == node2.serviceMgr.getEndPoint(node1Addr) {
		time.Sleep(time.Second)
	}

	node2.PostMessage(node1Addr, &ss_msg.Echo{
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

	node1.Stop(nil)

	node2.Stop(nil)

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
		assert.Equal(t, nil, node1.Start([]string{centerAddr}, addr_))
	}

	{
		addr_, _ := addr.MakeAddr("1.2.2", "127.0.0.1:8002")
		assert.Equal(t, nil, node2.Start([]string{centerAddr}, addr_))
	}

	node1Addr, _ := addr.MakeLogicAddr("1.1.1")

	//等待从center接收到node1的信息
	for nil == node2.serviceMgr.getEndPoint(node1Addr) {
		time.Sleep(time.Second)
	}

	//异步调用
	node2.AsynCall(node1Addr, &ss_rpc.EchoReq{Message: proto.String("hello")}, 10000, func(result interface{}, err error) {
		cc <- struct{}{}
	})

	_ = <-cc

	//异步调用
	node2.AsynCall(node1Addr, &ss_rpc.EchoReq{Message: proto.String("hello")}, 10000, func(result interface{}, err error) {
		cc <- struct{}{}
	})

	_ = <-cc

	node1.Stop(nil)

	node2.Stop(nil)

	center1.Stop()

}

func TestHarbor(t *testing.T) {

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

	harbor1Addr, _ := addr.MakeHarborAddr("1.255.1", "127.0.0.1:8011")
	harbor2Addr, _ := addr.MakeHarborAddr("2.255.1", "127.0.0.1:8012")

	assert.Equal(t, nil, harbor1.Start([]string{"127.0.0.1:8000", "127.0.0.1:8001"}, harbor1Addr))

	assert.Equal(t, nil, harbor2.Start([]string{"127.0.0.1:8000", "127.0.0.1:8002"}, harbor2Addr))

	node1 := NewCluster()

	node2 := NewCluster()

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

	assert.Equal(t, nil, node1.Start([]string{"127.0.0.1:8001"}, node1Addr, EXPORT))

	assert.Equal(t, nil, node2.Start([]string{"127.0.0.1:8002"}, node2Addr))

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
		cc <- struct{}{}
	})

	_ = <-cc

	node2.PostMessage(peer, &ss_msg.Echo{
		Message: proto.String("hello"),
	})

	_ = <-cc

	node1.Stop(nil, true)

	for {
		_, err := node2.Random(1)
		if nil == err {
			time.Sleep(time.Second)
		} else {
			assert.Equal(t, err, ERR_NO_AVAILABLE_SERVICE)
			break
		}
	}

	node2.AsynCall(peer, &ss_rpc.EchoReq{Message: proto.String("hello")}, 10000, func(result interface{}, err error) {
		assert.Equal(t, rpcerr.Err_RPC_RelayError, err)
		cc <- struct{}{}
	})

	_ = <-cc

	node2.Stop(nil)

	harbor1.Stop(nil)
	harbor2.Stop(nil)

	center1.Stop()
	center2.Stop()
	center3.Stop()

}
