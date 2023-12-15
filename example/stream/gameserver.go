package main

import (
	"github.com/sniperHW/clustergo"
	"github.com/sniperHW/clustergo/addr"
	"github.com/sniperHW/clustergo/example/membership"
	"github.com/sniperHW/clustergo/logger/zap"
	"github.com/xtaci/smux"
)

func main() {
	l := zap.NewZapLogger("1.1.1.log", "./logfile", "debug", 1024*1024*100, 14, 28, true)
	clustergo.InitLogger(l.Sugar())
	localaddr, _ := addr.MakeLogicAddr("1.1.1")
	clustergo.Start(membership.NewClient("127.0.0.1:18110"), localaddr)
	clustergo.StartSmuxServer(func(s *smux.Stream) {
		go func() {
			buff := make([]byte, 64)
			for {
				n, err := s.Read(buff)
				if err != nil {
					break
				}
				n, err = s.Write(buff[:n])
				if err != nil {
					break
				}
			}
			s.Close()
		}()
	})
	clustergo.Wait()
}
