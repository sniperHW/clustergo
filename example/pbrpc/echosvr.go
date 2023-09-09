package main

import (
	"context"

	"github.com/sniperHW/clustergo"
	"github.com/sniperHW/clustergo/addr"
	"github.com/sniperHW/clustergo/example/discovery"
	"github.com/sniperHW/clustergo/logger/zap"
	"github.com/sniperHW/clustergo/pbrpc/service/echo"
)

type echoService struct {
}

func (e *echoService) OnCall(ctx context.Context, replyer *echo.Replyer, request *echo.Request) {
	from := replyer.Channel().(clustergo.RPCChannel).Peer() //获取请求的对端逻辑地址
	clustergo.Log().Debug("from:", from.String(), ",echo:", request.Msg)
	replyer.Reply(&echo.Response{Msg: request.Msg}, nil)
}

func main() {
	l := zap.NewZapLogger("1.1.1.log", "./logfile", "debug", 1024*1024*100, 14, 28, true)
	clustergo.InitLogger(l.Sugar())
	echo.Register(&echoService{})

	localaddr, _ := addr.MakeLogicAddr("1.1.1")
	clustergo.Start(discovery.NewClient("127.0.0.1:18110"), localaddr)

	clustergo.Wait()

}
