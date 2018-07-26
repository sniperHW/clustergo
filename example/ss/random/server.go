package main 

import(
	"sanguo/cluster"
	"os"
	"fmt"
	_ "sanguo/protocol/ss" //触发pb注册
	ss_msg "sanguo/protocol/ss/message"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/golog"
	"strconv"	
)

func main() {

	outLogger := golog.NewOutputLogger("log","random_server",1024*1024*1000)
	kendynet.InitLogger(outLogger)
	
	center_addr := os.Args[1]
	tt          := "server"
	ip          := os.Args[2]
	port , _    := strconv.Atoi(os.Args[3])

	cluster.Register(&ss_msg.Echo{},func (session kendynet.StreamSession,msg proto.Message){
		fmt.Printf("on echo\n")
		session.Send(msg)
	})

	err := cluster.Start(center_addr,cluster.MakeService(tt,ip,int32(port)))

	if nil == err {
		sigStop := make(chan bool)
		_,_ = <- sigStop
	} else {
		fmt.Printf("cluster Start error:%s\n",err.Error())
	}
}