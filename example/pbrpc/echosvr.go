package main

import (
	"context"

	"github.com/sniperHW/clustergo"
	"github.com/sniperHW/clustergo/addr"
	"github.com/sniperHW/clustergo/example/discovery"
	"github.com/sniperHW/clustergo/example/pbrpc/service/echo"
	"github.com/sniperHW/clustergo/logger/zap"
)

type echoService struct {
}

func (e *echoService) ServeEcho(ctx context.Context, replyer *echo.Replyer, request *echo.EchoReq) {
	from := replyer.Channel().(clustergo.RPCChannel).Peer() //获取请求的对端逻辑地址
	clustergo.Log().Debug("from:", from.String(), ",echo:", request.Msg)
	replyer.Reply(&echo.EchoRsp{Msg: request.Msg})
}

func main() {
	l := zap.NewZapLogger("1.1.1.log", "./logfile", "debug", 1024*1024*100, 14, 28, true)
	clustergo.InitLogger(l.Sugar())
	echo.Register(&echoService{})

	localaddr, _ := addr.MakeLogicAddr("1.1.1")
	clustergo.Start(discovery.NewClient("127.0.0.1:18110"), localaddr)

	clustergo.Wait()

}
