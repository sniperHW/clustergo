package main
import(
	"sanguo/cluster"
	"os"
	"fmt"
	_ "sanguo/protocol/ss" //触发pb注册
	ss_rpc "sanguo/protocol/ss/rpc"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	"time"	
	"github.com/sniperHW/kendynet/golog"	
)

func main() {

	outLogger := golog.NewOutputLogger("log","rpcserver",1024*1024*50)
	kendynet.InitLogger(outLogger)
	
	center_addr := os.Args[1]
	tt          := "rpcserver"
	ip          := "localhost"
	port        := 8012 

	nextShow    := time.Now().Unix() + 1
	count       := 0

	cluster.RegisterMethod(&ss_rpc.EchoReq{},func (replyer *cluster.Replyer,arg proto.Message){
		count++
		now := time.Now().Unix()
		if now >= nextShow {
			nextShow = now + 1
			fmt.Printf("count:%d qps/s\n",count)
			count = 0
		}
		echoResp := &ss_rpc.EchoResp{}
		echoResp.Message = proto.String("world")
		replyer.Reply(echoResp)
	})

	err := cluster.Start(center_addr,cluster.MakeService(tt,ip,int32(port)))



	if nil == err {
		time.Sleep(time.Second)
		sigStop := make(chan bool)
		_,_ = <- sigStop
	} else {
		fmt.Printf("cluster Start error:%s\n",err.Error())
	}
}