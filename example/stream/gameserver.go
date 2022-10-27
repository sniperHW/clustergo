package main

import (
	"github.com/sniperHW/sanguo"
	"github.com/sniperHW/sanguo/addr"
	"github.com/sniperHW/sanguo/example/discovery"
	"github.com/sniperHW/sanguo/logger/zap"
	"github.com/xtaci/smux"
)

func main() {
	l := zap.NewZapLogger("1.1.1.log", "./logfile", "debug", 1024*1024*100, 14, 28, true)
	sanguo.InitLogger(l.Sugar())
	localaddr, _ := addr.MakeLogicAddr("1.1.1")
	sanguo.Start(discovery.NewClient("127.0.0.1:8110"), localaddr)
	sanguo.OnNewStream(func(s *smux.Stream) {
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
	sanguo.Wait()
}
