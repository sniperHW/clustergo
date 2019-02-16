package main

import (
	"fmt"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/golog"
	"github.com/sniperHW/sanguo/protocol/cs/proto_def"
	"os"
	//"strings"
)

//产生协议注册文件
func gen_lua_id(out_path string) {

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

	str := `
return {
`

	for _, v := range proto_def.CS_message {
		str = str + fmt.Sprintf(`	[%d] = {name='%s',desc='%s'},`, v.MessageID, v.Name, v.Desc) + "\n"
	}

	str = str + "}\n"

	_, err = f.WriteString(str)

	//fmt.Printf(str)

	if nil != err {
		kendynet.Errorf("%s Write error:%s\n", out_path, err.Error())
	} else {
		kendynet.Infof("%s Write ok\n", out_path)
	}

	f.Close()

}

func main() {
	outLogger := golog.NewOutputLogger("log", "proto_gen_cs", 1024*1024*1000)
	kendynet.InitLogger(golog.New("proto_gen_cs", outLogger))

	os.MkdirAll("../message", os.ModePerm)
	gen_lua_id("../messageID.lua")
	kendynet.Infoln("------------------------------------------")
	kendynet.Infoln("cs gen_proto_lua ok!")
}
