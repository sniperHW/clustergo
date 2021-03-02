package pprof

import (
	"fmt"
	"github.com/sniperHW/sanguo/node/common/addrDispose"
	"net/http"
	_ "net/http/pprof"
	"strings"
)

/*
 pprof 启动格式  pprof@0.0.0.0:12345
*/

func StartPProf(args []string) {
	for _, arg := range args {
		if strings.Contains(arg, "pprof") {
			addr := strings.TrimPrefix(arg, "pprof@")
			address := addrDispose.AddrDispose(addr)
			fmt.Println("has pprof:", addr, address)
			go func() {
				err := http.ListenAndServe(address, nil)
				if err != nil {
					panic(err)
				}
			}()
			break
		}
	}
}
