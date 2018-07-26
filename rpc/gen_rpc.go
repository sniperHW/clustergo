package main
	
import (
	"sanguo/protocol/ss/proto_def"
	"fmt"
	"os"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/golog"
	"strings"
)


var	template string = `
package %s

import (
	"sanguo/cluster"
	"github.com/sniperHW/kendynet/rpc"
	ss_rpc "sanguo/protocol/ss/rpc"
)

type %s struct {
	replyer_ *rpc.RPCReplyer
}

func (this *%s) Reply(result *%s) {
	this.replyer_.Reply(result,nil)
}

func (this *%s) Error(err error) {
	this.replyer_.Reply(nil,err)
}

type %s interface {
	OnCall(*%s,*%s)
}

func Register(methodObj %s) {
	f := func(r *rpc.RPCReplyer, arg interface{}) {
		replyer_ := &%s{replyer_:r}
		methodObj.OnCall(replyer_,arg.(*%s))
	}

	cluster.RegisterMethod(&%s{},f)
}

func AsynCall(peer cluster.PeerID,arg *%s,timeout uint32,cb func(*%s,error)) {
	callback := func(r interface{},e error) {
		cb(r.(*%s),e)
	}
	cluster.AsynCall(peer,arg,timeout,callback)
}


func SyncCall(peer cluster.PeerID,arg *%s,timeout uint32) (ret *%s,err error) {
	respChan := make(chan interface{})
	AsynCall(peer,arg,timeout,func (ret_ *%s,err_ error) {
		ret = ret_
		err = err_
		respChan <- nil
	})
	_ = <- respChan
	return
}
`


func gen_rpc(array []string) {

	for _,v := range(array) {

		path := v
		filename := fmt.Sprintf("%s/%s.go",path,v)
		os.MkdirAll(path,os.ModePerm)
		f,err := os.OpenFile(filename,os.O_RDWR,os.ModePerm)
		if err != nil {
			if os.IsNotExist(err) {
				f,err = os.Create(filename)
				if err != nil {
					kendynet.Errorf("create %s failed:%s",filename,err.Error())
					return
				}
			} else {
				kendynet.Errorf("open %s failed:%s",filename,err.Error())			
				return
			}
		}
		err = os.Truncate(filename,0)
		if err != nil {
			kendynet.Errorf("Truncate %s failed:%s",filename,err.Error())		
			f.Close()	
			return
		}

		reqTypeStr := fmt.Sprintf("ss_rpc.%sReq",strings.Title(v))
		respTypeStr := fmt.Sprintf("ss_rpc.%sResp",strings.Title(v))
		replyerTypeStr := fmt.Sprintf("%sReplyer",strings.Title(v))
		packageStr := v
		interfaceStr := strings.Title(v)


		content := fmt.Sprintf(template,
							   packageStr,
							   replyerTypeStr,
							   replyerTypeStr,
							   respTypeStr,
							   replyerTypeStr,
							   interfaceStr,
							   replyerTypeStr,
							   reqTypeStr, 							   
							   interfaceStr,
							   replyerTypeStr,
							   reqTypeStr,
							   reqTypeStr,
							   reqTypeStr,
							   respTypeStr,
							   respTypeStr,
							   reqTypeStr,
							   respTypeStr,
							   respTypeStr,
							   )

		_,err = f.WriteString(content)

		fmt.Printf(content)

		if nil != err {
			kendynet.Errorf("%s Write error:%s\n",filename,err.Error())		
			return				
		} else {
			kendynet.Infof("%s Write ok\n",filename)		
		}

		f.Close()
	}
}


func main() {
	fmt.Printf("gen_rpc\n")
	outLogger := golog.NewOutputLogger("log","gen_rpc",1024*1024*1000)
	kendynet.InitLogger(outLogger)
	gen_rpc(proto_def.SS_rpc)
}