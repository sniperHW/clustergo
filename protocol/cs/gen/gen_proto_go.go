package main
	
import (
	"sanguo/protocol/cs/proto_def"
	"fmt"
	"os"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/golog"
	"strings"
)

var message_template string = "syntax = \"proto2\";\npackage message;\n\nmessage %s_toS {}\n\nmessage %s_toC {}\n"


func gen_proto(out_path string) {



	kendynet.Infof("gen_proto message ............\n")


	for _,v := range(proto_def.CS_message) {
		filename := fmt.Sprintf("%s/%s.proto",out_path,v)
		//检查文件是否存在，如果存在跳过不存在创建
		f,err := os.Open(filename)
		if nil != err && os.IsNotExist(err) {
			f,err = os.Create(filename)
			if nil == err {
				var content string
				content = fmt.Sprintf(message_template,v,v)
				_,err = f.WriteString(content)

				if nil != err {
					kendynet.Errorf("%s Write error:%s\n",v,err.Error())						
				}

				f.Close()

			} else{
				kendynet.Errorf("%s Create error:%s\n",v,err.Error())
			}
		} else if nil != f {
			kendynet.Infof("%s.proto exist skip\n",v)
			f.Close()
		}
	}
}


var	register_template string = `
package cs
import (
	"sanguo/codec/pb"
	"sanguo/protocol/cs/message"
)

func init() {
	//toS
%s
	//toC
%s
}
`

//产生协议注册文件
func gen_register(out_path string) {

	f,err := os.OpenFile(out_path,os.O_RDWR,os.ModePerm)
	if err != nil {
		if os.IsNotExist(err) {
			f,err = os.Create(out_path)
			if err != nil {
				kendynet.Errorf("create %s failed:%s",out_path,err.Error())
				return
			}
		} else {
			kendynet.Errorf("open %s failed:%s",out_path,err.Error())			
			return
		}
	}

	err = os.Truncate(out_path,0)

	if err != nil {
		kendynet.Errorf("Truncate %s failed:%s",out_path,err.Error())			
		return
	}


	toS_str := ""
	toC_str := ""
	for id,v := range(proto_def.CS_message) {
		toS_str  = toS_str + fmt.Sprintf(`	pb.Register("cs",&message.%sToS{},%d)`,strings.Title(v),id+1) + "\n"
		toC_str = toC_str + fmt.Sprintf(`	pb.Register("sc",&message.%sToC{},%d)`,strings.Title(v),id+1) + "\n"
	}

	content := fmt.Sprintf(register_template,toS_str,toC_str)

	_,err = f.WriteString(content)

	fmt.Printf(content)

	if nil != err {
		kendynet.Errorf("%s Write error:%s\n",out_path,err.Error())						
	} else {
		kendynet.Infof("%s Write ok\n",out_path)		
	}

	f.Close()

}


func main() {
	outLogger := golog.NewOutputLogger("log","proto_gen_cs",1024*1024*1000)
	kendynet.InitLogger(outLogger)

	os.MkdirAll("../message",os.ModePerm)
	gen_proto("../proto/message")
	gen_register("../register.go")
}