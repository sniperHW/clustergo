package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"text/template"
)

var templateStr string = `
package {{.Method}}

import (
	"github.com/sniperHW/clustergo"
	"github.com/sniperHW/clustergo/addr"
	"github.com/sniperHW/rpcgo"
	"context"
	"time"
)

type Replyer struct {
	replyer *rpcgo.Replyer
}

func (r *Replyer) Reply(result *Response,err error) {
	r.replyer.Reply(result,err)
}

func (r *Replyer) Channel() rpcgo.Channel {
	return r.replyer.Channel()
}

type {{.Service}} interface {
	OnCall(context.Context, *Replyer,*Request)
}

func Register(o {{.Service}}) {
	clustergo.RegisterRPC("{{.Method}}",func(ctx context.Context, r *rpcgo.Replyer,arg *Request) {
		o.OnCall(ctx,&Replyer{replyer:r},arg)
	})
}

func Call(ctx context.Context, peer addr.LogicAddr,arg *Request) (*Response,error) {
	var resp Response
	err := clustergo.Call(ctx,peer,"{{.Method}}",arg,&resp)
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


	return clustergo.CallWithCallback(peer,deadline,"{{.Method}}",arg,&resp,fn) 		
}

`

type method struct {
	Method  string
	Service string
}

var (
	inputPath  *string
	outputPath *string
)

func gen(tmpl *template.Template, name string) {
	filename := fmt.Sprintf("%s/%s/%s.go", *outputPath, name, name)
	os.MkdirAll(fmt.Sprintf("%s/%s", *outputPath, name), os.ModePerm)
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

	err = tmpl.Execute(f, method{name, strings.Title(name) + "Service"})
	if err != nil {
		panic(err)
	} else {
		log.Printf("%s Write ok\n", filename)
	}
}

func main() {

	inputPath = flag.String("inputPath", "proto", "inputPath")
	outputPath = flag.String("outputPath", "service", "outputPath")

	flag.Parse()

	tmpl, err := template.New("test").Parse(templateStr)
	if err != nil {
		panic(err)
	}

	//遍历proto目录获取所有.proto文件
	if f, err := os.Open(*inputPath); err == nil {
		var fi []os.FileInfo
		if fi, err = f.Readdir(0); err == nil {
			for _, v := range fi {
				t := strings.Split(v.Name(), ".")
				if len(t) == 2 && t[1] == "proto" {
					gen(tmpl, t[0])
				}
			}
		}
		f.Close()
	}
}
