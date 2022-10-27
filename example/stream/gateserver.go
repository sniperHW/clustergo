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

			/*go func() {
				buff := make([]byte, 1024)
				for {
					n, err := conn.Read(buff)
					if err != nil {
						break
					}
					_, err = cliStream.Write(buff[:n])
					if err != nil {
						break
					}
				}
				wait.Done()
			}()

			go func() {
				buff := make([]byte, 1024)
				for {
					n, err := cliStream.Read(buff)
					if err != nil {
						break
					}
					_, err = conn.Write(buff[:n])
					if err != nil {
						break
					}
				}
				wait.Done()
			}()*/
			wait.Wait()
		}()
	})
	go serve()

	sanguo.Wait()
}
