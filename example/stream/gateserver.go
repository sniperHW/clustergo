package main

import (
	"io"
	"net"
	"sync"

	"github.com/sniperHW/netgo"
	"github.com/sniperHW/clustergo"
	"github.com/sniperHW/clustergo/addr"
	"github.com/sniperHW/clustergo/example/discovery"
	"github.com/sniperHW/clustergo/logger/zap"
)

func main() {
	l := zap.NewZapLogger("1.2.1.log", "./logfile", "debug", 1024*1024*100, 14, 28, true)
	clustergo.InitLogger(l.Sugar())
	localaddr, _ := addr.MakeLogicAddr("1.2.1")
	clustergo.Start(discovery.NewClient("127.0.0.1:8110"), localaddr)

	gameAddr, _ := clustergo.GetAddrByType(1)

	_, serve, _ := netgo.ListenTCP("tcp", "127.0.0.1:8113", func(conn *net.TCPConn) {
		go func() {
			cliStream, err := clustergo.OpenStream(gameAddr)
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

	clustergo.Wait()
}
