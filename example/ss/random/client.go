package main 

import(
	"sanguo/cluster"
	"os"
	"fmt"
	_ "sanguo/protocol/ss" //触发pb注册
	ss_msg "sanguo/protocol/ss/message"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	"time"	
	"github.com/sniperHW/kendynet/golog"
	"strconv"			
)

func main() {
	outLogger := golog.NewOutputLogger("log","random_client",1024*1024*1000)
	kendynet.InitLogger(outLogger)
	
	center_addr := os.Args[1]
	tt          := "client"
	ip          := os.Args[2]
	port , _    := strconv.Atoi(os.Args[3])

	cluster.Register(&ss_msg.Echo{},func (session kendynet.StreamSession,msg proto.Message){
		peerID,err := cluster.GetPeerIDBySession(session)
		if nil == err {
			fmt.Printf("on echo from:%s\n",peerID.ToString())
		}
	})

	err := cluster.Start(center_addr,cluster.MakeService(tt,ip,int32(port)))

	echo := &ss_msg.Echo{}
	echo.Message = proto.String("hello")

	if nil == err {
		go func(){
			for {
				time.Sleep(time.Second)				
				/*
				//随机选择一个节点发送
				peer,err := cluster.Random("server")
				if nil == err {
					cluster.PostMessage(peer,echo)
				}*/

				//测试广播
				cluster.Brocast("server",echo)

			}
		}()
		sigStop := make(chan bool)
		_,_ = <- sigStop
	} else {
		fmt.Printf("cluster Start error:%s\n",err.Error())
	}
}