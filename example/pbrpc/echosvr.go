package main

import (
	"context"

	"github.com/sniperHW/sanguo"
	"github.com/sniperHW/sanguo/addr"
	"github.com/sniperHW/sanguo/example/discovery"
	"github.com/sniperHW/sanguo/log/zap"
	"github.com/sniperHW/sanguo/pbrpc/service/echo"
)

type echoService struct {
}

func (e *echoService) OnCall(ctx context.Context, replyer *echo.Replyer, request *echo.Request) {
	sanguo.Logger().Debug("echo:", request.Msg)
	replyer.Reply(&echo.Response{Msg: request.Msg}, nil)
}

func main() {
	l := zap.NewZapLogger("1.1.1.log", "./logfile", "debug", 1024*1024*100, 14, 28, true)
	sanguo.InitLogger(l.Sugar())
	echo.Register(&echoService{})

	localaddr, _ := addr.MakeLogicAddr("1.1.1")
	sanguo.Start(discovery.NewClient("127.0.0.1:8110"), localaddr)

	sanguo.Wait()

}
