package main

import (
	"fmt"
	"github.com/sniperHW/kendynet"
	"github.com/sniperHW/kendynet/golog"
	"github.com/sniperHW/sanguo/protocol/ss/proto_def"
	"os"
	"strings"
)

var template string = `
package [s1]

import (
	"github.com/sniperHW/sanguo/cluster"
	"github.com/sniperHW/sanguo/cluster/addr"
	"github.com/sniperHW/kendynet/rpc"
	ss_rpc "github.com/sniperHW/sanguo/protocol/ss/rpc"
)

type [s2] struct {
	replyer_ *rpc.RPCReplyer
}

func (this *[s2]) Reply(result *ss_rpc.[s3]) {
	this.replyer_.Reply(result,nil)
}

func (this *[s2]) Error(err error) {
	this.replyer_.Reply(nil,err)
}


func (this *[s2]) GetChannel() rpc.RPCChannel {
	return this.replyer_.GetChannel()
}


type [s5] interface {
	OnCall(*[s2],*ss_rpc.[s4])
}

func Register(methodObj [s5]) {
	f := func(r *rpc.RPCReplyer, arg interface{}) {
		replyer_ := &[s2]{replyer_:r}
		methodObj.OnCall(replyer_,arg.(*ss_rpc.[s4]))
	}

	cluster.RegisterMethod(&ss_rpc.[s4]{},f)
}

func AsynCall(peer addr.LogicAddr,arg *ss_rpc.[s4],timeout uint32,cb func(*ss_rpc.[s3],error)) {
	callback := func(r interface{},e error) {
		if nil != r {
			cb(r.(*ss_rpc.[s3]),e)
		} else {
			cb(nil,e)
		}
	}
	cluster.AsynCall(peer,arg,timeout,callback)
}

func SyncCall(peer addr.LogicAddr,arg *ss_rpc.[s4],timeout uint32) (ret *ss_rpc.[s3], err error) {
	respChan := make(chan struct{})
	f := func(ret_ *ss_rpc.[s3], err_ error) {
		ret = ret_
		err = err_
		respChan <- struct{}{}
	}
	AsynCall(peer,arg,timeout,f)
	_ = <-respChan
	return
}

`

func gen_rpc(array []proto_def.St) {

	for _, v := range array {

		path := v.Name
		filename := fmt.Sprintf("%s/%s.go", path, v.Name)
		os.MkdirAll(path, os.ModePerm)
		f, err := os.OpenFile(filename, os.O_RDWR, os.ModePerm)
		if err != nil {
			if os.IsNotExist(err) {
				f, err = os.Create(filename)
				if err != nil {
					kendynet.Errorf("create %s failed:%s", filename, err.Error())
					return
				}
			} else {
				kendynet.Errorf("open %s failed:%s", filename, err.Error())
				return
			}
		}
		err = os.Truncate(filename, 0)
		if err != nil {
			kendynet.Errorf("Truncate %s failed:%s", filename, err.Error())
			f.Close()
			return
		}

		content := template
		content = strings.Replace(content, "[s1]", v.Name, -1)
		content = strings.Replace(content, "[s2]", strings.Title(v.Name)+"Replyer", -1)
		content = strings.Replace(content, "[s3]", strings.Title(v.Name)+"Resp", -1)
		content = strings.Replace(content, "[s4]", strings.Title(v.Name)+"Req", -1)
		content = strings.Replace(content, "[s5]", strings.Title(v.Name), -1)
		_, err = f.WriteString(content)

		//fmt.Printf(content)

		if nil != err {
			kendynet.Errorf("%s Write error:%s\n", filename, err.Error())
			return
		} else {
			kendynet.Infof("%s Write ok\n", filename)
		}

		f.Close()
	}
}

func main() {
	fmt.Printf("gen_rpc\n")
	outLogger := golog.NewOutputLogger("log", "gen_rpc", 1024*1024*1000)
	kendynet.InitLogger(golog.New("gen_rpc", outLogger))
	gen_rpc(proto_def.SS_rpc)
	kendynet.Infoln("------------------------------------------")
	kendynet.Infoln("gen_rpc ok!")
}
