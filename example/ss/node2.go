package main 

import(
	"sanguo/cluster"
	"os"
	"fmt"
	_ "sanguo/protocol/ss" //触发pb注册
	ss_msg "sanguo/protocol/ss/message"
	ss_rpc "sanguo/protocol/ss/rpc"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/rpc"
	"github.com/sniperHW/kendynet/golog"
)

func main() {

	outLogger := golog.NewOutputLogger("log","test_node2",1024*1024*1000)
	kendynet.InitLogger(outLogger)
	
	center_addr := os.Args[1]
	tt          := "test_node2"
	ip          := "localhost"
	port        := 8012 

	cluster.Register(&ss_msg.Echo{},func (session kendynet.StreamSession,msg proto.Message){
		fmt.Printf("on echo\n")
		session.Send(msg)
	})

	cluster.RegisterService(&ss_rpc.EchoReq{},func (replyer *rpc.RPCReplyer,arg interface{}){
		echo := arg.(*ss_rpc.EchoReq)
		kendynet.Infof("echo message:%s\n",echo.GetMessage())
		echoResp := &ss_rpc.EchoResp{}
		echoResp.Message = proto.String("world")
		replyer.Reply(echoResp,nil)
	})

	err := cluster.Start(center_addr,cluster.MakeService(tt,ip,int32(port)))

	if nil == err {
		sigStop := make(chan bool)
		_,_ = <- sigStop
	} else {
		fmt.Printf("cluster Start error:%s\n",err.Error())
	}
}