package main

import (
	"context"

	"github.com/sniperHW/clustergo"
	"github.com/sniperHW/clustergo/addr"
	"github.com/sniperHW/clustergo/example/discovery"
	"github.com/sniperHW/clustergo/logger/zap"
	"github.com/sniperHW/clustergo/pbrpc/service/echo"
)

func main() {
	l := zap.NewZapLogger("1.2.1.log", "./logfile", "debug", 1024*1024*100, 14, 28, true)
	clustergo.InitLogger(l.Sugar())
	localaddr, _ := addr.MakeLogicAddr("1.2.1")
	clustergo.Start(discovery.NewClient("127.0.0.1:18110"), localaddr)

	echoAddr, _ := clustergo.GetAddrByType(1)

	for i := 0; i < 10; i++ {
		resp, err := echo.Call(context.TODO(), echoAddr, &echo.EchoReq{Msg: "hello"})
		clustergo.Log().Debug(resp, err)
	}
	clustergo.Stop()
	clustergo.Wait()
}
