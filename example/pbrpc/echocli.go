package main

import (
	"context"

	"github.com/sniperHW/sanguo"
	"github.com/sniperHW/sanguo/addr"
	"github.com/sniperHW/sanguo/example/discovery"
	"github.com/sniperHW/sanguo/log/zap"
	"github.com/sniperHW/sanguo/pbrpc/service/echo"
)

func main() {
	l := zap.NewZapLogger("1.2.1.log", "./logfile", "debug", 1024*1024*100, 14, 28, true)
	sanguo.InitLogger(l.Sugar())
	localaddr, _ := addr.MakeLogicAddr("1.2.1")
	sanguo.Start(discovery.NewClient("127.0.0.1:8110"), localaddr)

	echoAddr, _ := sanguo.GetAddrByType(1)

	for i := 0; i < 10; i++ {
		resp, err := echo.Call(context.TODO(), echoAddr, &echo.Request{Msg: "hello"})
		l.Sugar().Debug(resp, err)
	}
	sanguo.Stop()
	sanguo.Wait()
}
