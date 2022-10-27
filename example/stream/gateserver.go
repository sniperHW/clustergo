package main

import (
	"io"
	"net"
	"sync"

	"github.com/sniperHW/netgo"
	"github.com/sniperHW/sanguo"
	"github.com/sniperHW/sanguo/addr"
	"github.com/sniperHW/sanguo/example/discovery"
	"github.com/sniperHW/sanguo/logger/zap"
)

func main() {
	l := zap.NewZapLogger("1.2.1.log", "./logfile", "debug", 1024*1024*100, 14, 28, true)
	sanguo.InitLogger(l.Sugar())
	localaddr, _ := addr.MakeLogicAddr("1.2.1")
	sanguo.Start(discovery.NewClient("127.0.0.1:8110"), localaddr)

	gameAddr, _ := sanguo.GetAddrByType(1)

	_, serve, _ := netgo.ListenTCP("tcp", "127.0.0.1:8113", func(conn *net.TCPConn) {
		go func() {
			cliStream, err := sanguo.OpenStream(gameAddr)
			if err != nil {
				conn.Close()
				return
			}

			defer func() {
				conn.Close()
				cliStream.Close()
			}()

			var wait sync.WaitGroup
			wait.Add(2)

			go func() {
				io.Copy(cliStream, conn)
				wait.Done()
			}()

			go func() {
				io.Copy(conn, cliStream)
				wait.Done()
			}()
			wait.Wait()
		}()
	})
	go serve()

	sanguo.Wait()
}
