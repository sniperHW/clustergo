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

func (r *Replyer) Reply(result *{{.Response}}) {
	r.replyer.Reply(result)
}

func (r *Replyer) Error(err error) {
	r.replyer.Error(err)
}

func (r *Replyer) Channel() rpcgo.Channel {
	return r.replyer.Channel()
}

type {{.Service}} interface {
	Serve{{.Service}}(context.Context, *Replyer,*{{.Request}})
}

func Register(o {{.Service}}) {
	clustergo.GetRPCServer().RegisterService("{{.Method}}",func(ctx context.Context, r *rpcgo.Replyer,arg *{{.Request}}) {
		o.Serve{{.Service}}(ctx,&Replyer{replyer:r},arg)
	})
}

var client *clustergo.RPCClient = clustergo.GetRPCClient()

func Call(ctx context.Context, peer addr.LogicAddr,arg *{{.Request}}) (*{{.Response}},error) {
	var resp {{.Response}}
	err := client.Call(ctx,peer,"{{.Method}}",arg,&resp)
	return &resp,err
}

func CallWithTimeout(peer addr.LogicAddr,arg *{{.Request}},d time.Duration) (*{{.Response}},error) {
	var resp {{.Response}}
	err := client.CallWithTimeout(peer,"{{.Method}}",arg,&resp,d)
	return &resp,err
}

func AsyncCall(peer addr.LogicAddr,arg *{{.Request}},deadline time.Time,callback func(*{{.Response}},error)) error {
	var resp {{.Response}}
	err := client.AsyncCall(peer,"{{.Method}}",arg,&resp,deadline,func(res interface{},err error){
		callback(res.(*{{.Response}}),err)
	})
	return err
}

`

type method struct {
	Method   string
	Request  string
	Response string
	Service  string
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

	err = tmpl.Execute(f, method{
		Method:   name,
		Service:  strings.Title(name),
		Request:  fmt.Sprintf("%sReq", strings.Title(name)),
		Response: fmt.Sprintf("%sRsp", strings.Title(name)),
	})
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
