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

	outLogger := golog.NewOutputLogger("log","rpcclient",1024*1024*50)
	kendynet.InitLogger(outLogger)
	
	center_addr := os.Args[1]
	tt          := "rpcclient"
	ip          := "localhost"
	port        := 8013 

	err := cluster.Start(center_addr,cluster.MakeService(tt,ip,int32(port)))

	peer := cluster.MakePeerID("rpcserver","localhost",8012)

	if nil == err {
		time.Sleep(time.Second)
		sigStop := make(chan bool)
		echoReq := &ss_rpc.EchoReq{}
		echoReq.Message = proto.String("hello")
		for i := 0; i < 20; i++ {
			go func(){
				for {
					_,err := cluster.SyncCall(peer,echoReq,2000)
					if nil != err {
						fmt.Printf("%s\n",err.Error())
					}
				}
			}()
		}

		_,_ = <- sigStop
	} else {
		fmt.Printf("cluster Start error:%s\n",err.Error())
	}
}