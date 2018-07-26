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
	"time"	
	"github.com/sniperHW/kendynet/golog"	
)

func main() {
	outLogger := golog.NewOutputLogger("log","test_node1",1024*1024*1000)
	kendynet.InitLogger(outLogger)
	
	center_addr := os.Args[1]
	tt          := "test_node1"
	ip          := "localhost"
	port        := 8011 

	cluster.Register(&ss_msg.Echo{},func (session kendynet.StreamSession,msg proto.Message){
		fmt.Printf("on echo\n")
	})

	err := cluster.Start(center_addr,cluster.MakeService(tt,ip,int32(port)))

	peer := cluster.MakePeerID("test_node2","localhost",8012)

	if nil == err {
		go func(){
			for {

				time.Sleep(time.Second)
				echo := &ss_msg.Echo{}
				echo.Message = proto.String("hello")
				cluster.PostMessage(peer,echo)

				echoReq := &ss_rpc.EchoReq{}
				echoReq.Message = proto.String("hello")
				
				//同步调用
				ret,err := cluster.SyncCall(peer,echoReq,0)
				if nil == err {
					resp := ret.(*ss_rpc.EchoResp)
					kendynet.Infof("echo response %s\n",resp.GetMessage())
				}

				//异步调用
				/*cluster.AsynCall(peer,echoReq,0,func(result interface{},err error){
					if nil == err {
						resp := result.(*ss_rpc.EchoResp)
						kendynet.Infof("echo response %s\n",resp.GetMessage())
					}
				})*/
			}
		}()
		sigStop := make(chan bool)
		_,_ = <- sigStop
	} else {
		fmt.Printf("cluster Start error:%s\n",err.Error())
	}
}