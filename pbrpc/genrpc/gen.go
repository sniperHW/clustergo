package main

import (
	"fmt"
	"log"
	"os"
	"strings"
)

var template string = `
package [method]

import (
	"github.com/sniperHW/sanguo"
	"github.com/sniperHW/sanguo/addr"
	"github.com/sniperHW/rpcgo"
	"context"
	"time"
)

type Replyer struct {
	replyer *rpcgo.Replyer
}

func (this *Replyer) Reply(result *Response,err error) {
	this.replyer.Reply(result,err)
}

type [methodI] interface {
	OnCall(context.Context, *Replyer,*Request)
}

func Register(o [methodI]) {
	sanguo.RegisterRPC("[method]",func(ctx context.Context, r *rpcgo.Replyer,arg *Request) {
		o.OnCall(ctx,&Replyer{replyer:r},arg)
	})
}

func Call(ctx context.Context, peer addr.LogicAddr,arg *Request) (*Response,error) {
	var resp Response
	err := sanguo.Call(ctx,peer,"[method]",arg,&resp)
	return &resp,err
}

func CallWithCallback(peer addr.LogicAddr,deadline time.Time,arg *Request,cb func(*Response,error)) func() bool {
	var resp Response
	var fn func(interface{}, error)
	if cb != nil {
		fn = func (ret interface{}, err error){
			if ret != nil {
				cb(ret.(*Response),err)
			} else {
				cb(nil,err)
			}
		}
	}


	return sanguo.CallWithCallback(peer,deadline,"[method]",arg,&resp,fn) 		
}

`

func gen(name string) {
	filename := fmt.Sprintf("service/%s/%s.go", name, name)
	os.MkdirAll(fmt.Sprintf("service/%s", name), os.ModePerm)
	f, err := os.OpenFile(filename, os.O_RDWR, os.ModePerm)
	if err != nil {
		if os.IsNotExist(err) {
			f, err = os.Create(filename)
			if err != nil {
				log.Printf("------ error -------- create %s failed:%s", filename, err.Error())
				return
			}
		} else {
			log.Printf("------ error -------- open %s failed:%s", filename, err.Error())
			return
		}
	}
	defer f.Close()

	err = os.Truncate(filename, 0)
	if err != nil {
		log.Printf("------ error -------- Truncate %s failed:%s", filename, err.Error())
		return
	}

	content := template
	content = strings.Replace(content, "[method]", name, -1)
	content = strings.Replace(content, "[methodI]", strings.Title(name)+"Service", -1)
	_, err = f.WriteString(content)

	if nil != err {
		log.Printf("------ error -------- %s Write error:%s\n", filename, err.Error())
	} else {
		log.Printf("%s Write ok\n", filename)
	}
}

func main() {
	//遍历proto目录获取所有.proto文件
	if f, err := os.Open("./proto"); err == nil {
		var fi []os.FileInfo
		if fi, err = f.Readdir(0); err == nil {
			for _, v := range fi {
				t := strings.Split(v.Name(), ".")
				if len(t) == 2 && t[1] == "proto" {
					gen(t[0])
				}
			}
		}
		f.Close()
	}
}
