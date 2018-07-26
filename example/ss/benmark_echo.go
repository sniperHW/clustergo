package main
import(
	"sanguo/cluster"
	"os"
	"fmt"
	_ "sanguo/protocol/ss" //触发pb注册
	ss_message "sanguo/protocol/ss/message"
	"github.com/golang/protobuf/proto"
	"github.com/sniperHW/kendynet"
	"time"	
	"github.com/sniperHW/kendynet/golog"		
)

func main() {

	outLogger := golog.NewOutputLogger("log","benmark_echo",1024*1024*50)
	kendynet.InitLogger(outLogger,"benmark_echo")
	cluster.InitLogger(outLogger,"benmark_echo")
	
	center_addr := os.Args[1]
	tt          := "benmark_echo"
	ip          := "localhost"
	port        := 8012 

	nextShow    := time.Now().Unix() + 1
	count       := 0



	cluster.Register(&ss_message.Echo{},func (session kendynet.StreamSession,msg proto.Message){
		count++
		now := time.Now().Unix()
		if now >= nextShow {
			nextShow = now + 1
			//fmt.Errorf("count:%d qps/s\n",count)
			fmt.Fprintf(os.Stderr,"count:%d qps/s\n",count)
			count = 0
		}
		session.Send(msg)
	})

	err := cluster.Start(center_addr,cluster.MakeService(tt,ip,int32(port)))

	peer := cluster.MakePeerID("benmark_echo","localhost",8012)

	if nil == err {
		time.Sleep(time.Second)
		sigStop := make(chan bool)

		/*for i := 0; i < 10; i++ {

			Echo := &ss_message.Echo{}
			Echo.Message = proto.String("hello")

			cluster.PostMessage(peer,Echo)
		}*/

		for i := 0; i < 10; i++ {
			Echo := &ss_message.Echo{}
			Echo.Message = proto.String("good mormiong everyone,my name is sniperHW,nice to meet you")
			cluster.PostMessage(peer,Echo)
		}

		_,_ = <- sigStop
	} else {
		fmt.Printf("cluster Start error:%s\n",err.Error())
	}
}