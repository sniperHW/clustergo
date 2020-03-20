package main

import (
	"fmt"
	"github.com/sniperHW/kendynet/golog"
	csproto_def "github.com/sniperHW/sanguo/protocol/cs/proto_def"
	ssproto_def "github.com/sniperHW/sanguo/protocol/ss/proto_def"
	"os"
	"strings"

	"github.com/sniperHW/kendynet"
)

var cmd_tmp string = `
package cmdEnum

const (
	//cs消息
%s
	//rpc消息
%s
	//ss消息
%s
)
`

//产生协议注册文件
func gen_cmd(out_path string) {

	f, err := os.OpenFile(out_path, os.O_RDWR, os.ModePerm)
	if err != nil {
		if os.IsNotExist(err) {
			f, err = os.Create(out_path)
			if err != nil {
				kendynet.Errorf("create %s failed:%s", out_path, err.Error())
				return
			}
		} else {
			kendynet.Errorf("open %s failed:%s", out_path, err.Error())
			return
		}
	}

	err = os.Truncate(out_path, 0)

	if err != nil {
		kendynet.Errorf("Truncate %s failed:%s", out_path, err.Error())
		return
	}
	//用户定义ID开始区段
	ss_start := 1000

	cs_str, rpc_str, ss_str := "", "", ""
	for _, v := range csproto_def.CS_message {
		cs_str += fmt.Sprintf("    CS_%s uint16 = %d\n", strings.Title(v.Name), v.MessageID)
	}
	for _, v := range ssproto_def.SS_rpc {
		rpc_str += fmt.Sprintf("    RPC_%s uint16 = %d\n", strings.Title(v.Name), v.MessageID)
	}
	for _, v := range ssproto_def.SS_message {
		ss_str += fmt.Sprintf("    SS_%s uint16 = %d\n", strings.Title(v.Name), v.MessageID+ss_start)
	}

	content := fmt.Sprintf(cmd_tmp, cs_str, rpc_str, ss_str)

	_, err = f.WriteString(content)

	if nil != err {
		kendynet.Errorf("%s Write error:%s\n", out_path, err.Error())
	} else {
		kendynet.Infof("%s Write ok\n", out_path)
	}

	f.Close()

}

func main() {
	outLogger := golog.NewOutputLogger("log", "gen_cmd", 1024*1024*1000)
	kendynet.InitLogger(golog.New("gen_cmd", outLogger))
	kendynet.Infoln("start gen_cmd ...")

	gen_cmd("../cmdEnum.go")
	kendynet.Infoln("gen_cmd ok!")
}
