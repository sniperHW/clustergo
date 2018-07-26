package main
import(
	"sanguo/cluster"
	"os"
	"fmt"
	_ "sanguo/protocol/ss" //触发pb注册
	ss_rpc "sanguo/protocol/ss/rpc"
	echo "sanguo/rpc/echo"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	"time"	
	"github.com/sniperHW/kendynet/golog"
)


var count int
var nextShow int64

type Echo struct {

}

func (this *Echo) OnCall(replyer *echo.EchoReplyer,arg *ss_rpc.EchoReq) {
	count++
	now := time.Now().Unix()
	if now >= nextShow {
		nextShow = now + 1
		fmt.Fprintf(os.Stderr,"count:%d qps/s\n",count)
		count = 0
	}
	echoResp := &ss_rpc.EchoResp{}
	echoResp.Message = proto.String(arg.GetMessage())
	replyer.Reply(echoResp)	
}

func main() {
	outLogger := golog.NewOutputLogger("log","benmark_rpc",1024*1024*50)
	kendynet.InitLogger(outLogger,"benmark_rpc")
	cluster.InitLogger(outLogger,"benmark_rpc")
	
	center_addr := "127.0.0.1:8010"//os.Args[1]
	tt          := "benmark_rpc"
	ip          := "localhost"
	port        := 8012 

	nextShow    = time.Now().Unix() + 1
	count       = 0

	//注册方法处理对象
	echo.Register(&Echo{})

	err := cluster.Start(center_addr,cluster.MakeService(tt,ip,int32(port)))

	peer := cluster.MakePeerID("benmark_rpc","localhost",8012)	

	if nil == err {
		time.Sleep(time.Second)
		sigStop := make(chan bool)

		echoReq1 := &ss_rpc.EchoReq{}
		echoReq1.Message = proto.String("hello")

		echoReq2 := &ss_rpc.EchoReq{}
		echoReq2.Message = proto.String("good mormiong everyone,my name is sniperHW,nice to meet you")			

		var callback1 func(result *ss_rpc.EchoResp,err error)
		var callback2 func(result *ss_rpc.EchoResp,err error)

		callback1 = func(result *ss_rpc.EchoResp,err error){
			if nil != err {
				kendynet.Infof("error %s\n",err.Error())
			}
			echo.AsynCall(peer,echoReq1,1000,callback1)
		}

		callback2 = func(result *ss_rpc.EchoResp,err error){
			if nil != err {
				kendynet.Infof("error %s\n",err.Error())
			}
			echo.AsynCall(peer,echoReq2,1000,callback2)
		}
		for i := 0; i < 20; i++ {
			echo.AsynCall(peer,echoReq1,1000,callback1)
		}

		for i := 0; i < 10; i++ {
			echo.AsynCall(peer,echoReq2,1000,callback2)
		}
		_,_ = <- sigStop
	} else {
		fmt.Printf("cluster Start error:%s\n",err.Error())
	}

}