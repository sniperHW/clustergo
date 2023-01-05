# 游戏服务端框架 

clustergo是一个简单的网络游戏服务端框架。可以快速构建服务器集群内部以及服务器与客户端时间的通信。集群内部采用tcp通信。服务器客户端之间
支持tcp,websocket,kcp通信方式。

服务器集群间支持两种通信模式：PRC及普通的消息传递。服务器节点使用一个逻辑地址作为标识，只要知道对端逻辑地址就可以与对端通信。集群内部通信节点会建立tcp连接，连接在首次通信请求时建立。

框架同时提供了[c#版本](https://github.com/sniperHW/ClusterDotNet)，如需要在服务端运行客户端的C#逻辑，可以选择c#版本。


## 逻辑地址

各节点使用一个32位逻辑地址标识，逻辑地址被分为3段，高12位表示服务器组，中8位表示节点类型(255保留给harbor使用),低12位表示进程id。

## rpc

clustergo默认采用pbrpc

将协议文件添加到pbrpc/proto目录中

例如对于echo服务,首先在proto目录添加echo.proto文件，之后填充文件内容如下：

```go
syntax = "proto3";

option go_package = "../service/echo";

message request {
	string msg = 1;
}

message response {
    string msg = 1;
}
```

对于任务服务，请求参数必须命名为request,返回值必须命名为response,go_package永远设置为"../service/echo"。

在pbrpc目录执行./gen.sh,这个操作会在pbrpc/service目录下生成echo.pb.go和echo.go两个文件。

其中echo.go的内容如下：

```go
package echo

import (
	"context"
	"time"

	"github.com/sniperHW/rpcgo"
	"github.com/sniperHW/clustergo"
	"github.com/sniperHW/clustergo/addr"
)

type Replyer struct {
	replyer *rpcgo.Replyer
}

func (this *Replyer) Reply(result *Response, err error) {
	this.replyer.Reply(result, err)
}

type EchoService interface {
	OnCall(context.Context, *Replyer, *Request)
}

func Register(o EchoService) {
	clustergo.RegisterRPC("echo", func(ctx context.Context, r *rpcgo.Replyer, arg *Request) {
		o.OnCall(ctx, &Replyer{replyer: r}, arg)
	})
}

func Call(ctx context.Context, peer addr.LogicAddr, arg *Request) (*Response, error) {
	var resp Response
	err := clustergo.Call(ctx, peer, "echo", arg, &resp)
	return &resp, err
}

func CallWithCallback(peer addr.LogicAddr, deadline time.Time, arg *Request, cb func(*Response, error)) func() bool {
	var resp Response
	var fn func(interface{}, error)
	if cb != nil {
		fn = func(ret interface{}, err error) {
			if ret != nil {
				cb(ret.(*Response), err)
			} else {
				cb(nil, err)
			}
		}
	}

	return clustergo.CallWithCallback(peer, deadline, "echo", arg, &resp, fn)
}
```

### 服务端

在服务端定义实现了EchoService的类型，通过Register注册服务：

```go

package main

import (
	"context"

	"github.com/sniperHW/clustergo"
	"github.com/sniperHW/clustergo/addr"
	"github.com/sniperHW/clustergo/example/discovery"
	"github.com/sniperHW/clustergo/log/zap"
	"github.com/sniperHW/clustergo/pbrpc/service/echo"
)

//实现echo.EchoService 
type echoService struct {
}

func (e *echoService) OnCall(ctx context.Context, replyer *echo.Replyer, request *echo.Request) {
	clustergo.Logger().Debug("echo:", request.Msg)
	replyer.Reply(&echo.Response{Msg: request.Msg}, nil)
}

func main() {
	l := zap.NewZapLogger("1.1.1.log", "./logfile", "debug", 1024*1024*100, 14, 28, true)
	clustergo.InitLogger(l.Sugar())

	//注册服务
	echo.Register(&echoService{})

	localaddr, _ := addr.MakeLogicAddr("1.1.1")
	clustergo.Start(discovery.NewClient("127.0.0.1:8110"), localaddr)

	clustergo.Wait()

}

```

### 客户端

```go

package main

import (
	"context"

	"github.com/sniperHW/clustergo"
	"github.com/sniperHW/clustergo/addr"
	"github.com/sniperHW/clustergo/example/discovery"
	"github.com/sniperHW/clustergo/log/zap"
	"github.com/sniperHW/clustergo/pbrpc/service/echo"
)

func main() {
	l := zap.NewZapLogger("1.2.1.log", "./logfile", "debug", 1024*1024*100, 14, 28, true)
	clustergo.InitLogger(l.Sugar())
	localaddr, _ := addr.MakeLogicAddr("1.2.1")
	clustergo.Start(discovery.NewClient("127.0.0.1:8110"), localaddr)

	//假设echo服务全部由逻辑地址type=1的节点提供，这里任意获取一个type=1的节点
	echoAddr, _ := clustergo.GetAddrByType(1)

	//执行10次同步调用
	for i := 0; i < 10; i++ {
		resp, err := echo.Call(context.TODO(), echoAddr, &echo.Request{Msg: "hello"})
		l.Sugar().Debug(resp, err)
	}
	clustergo.Stop()
	clustergo.Wait()
}


```

## 集群

逻辑地址最高字段位表示集群

集群内的节点可以建立通信连接直接通信(harbor不受这个限制)，跨集群节点通信请看下一小节。

默认情况下，集群A的节点无法通过GetAddrByType获取到集群B中的节点地址（如果已经获得对端的逻辑地址，则不受此限制）。如果希望将节点暴露到集群外，可以将节点的Export字段设置为true。

例如对于一个分服，开房间的游戏。

每个服属于一个集群，房间服务器作为公共服务，属于另外的集群。此时需要将战斗节点的Export字段设置为true。游戏服中的节点便可以通过GetAddrByType获取到可用的战斗节点。



### 跨集群通信

如果通信目标与自己不在同一集群内，则无法与目标建立直接通信连接，为了跨集群通信，需要启动harbor节点，消息路由如下:

本机 -> 本集群内harbor节点 -> 目标所在集群harbor节点 -> 目标节点

多集群部署示意图:

![Alt text](cluster.png)

如上图所示，有A,B两个集群，集群各自的harbor节点分别连接本集群的center以及harbors center。
当节点B向节点A发消息时，发现A不在本集群内无法建立直接通信，因此将消息发往harbor B,harbor B接收到路由请求后，根据目标地址
2.1.1将请求转发给harbor A(harbor A和harbor B连接了同一个harbors center,因此他们之间可以建立直接通信连接)。harbor A
接收到之后发现目标A可以直达，于是将消息发送给A。


## Stream（在单个连接上建立多个流）

clustergo支持在同一cluster内的节点之间建立stream。典型的使用方式由gateway接受客户端连接，并且为每一个连接建立一个到gameserver的stream。在gameserver看来，每个stream代表一个客户端连接。

gameserver.go

```go

package main

import (
	"github.com/sniperHW/clustergo"
	"github.com/sniperHW/clustergo/addr"
	"github.com/sniperHW/clustergo/example/discovery"
	"github.com/sniperHW/clustergo/logger/zap"
	"github.com/xtaci/smux"
)

func main() {
	l := zap.NewZapLogger("1.1.1.log", "./logfile", "debug", 1024*1024*100, 14, 28, true)
	clustergo.InitLogger(l.Sugar())
	localaddr, _ := addr.MakeLogicAddr("1.1.1")
	clustergo.Start(discovery.NewClient("127.0.0.1:8110"), localaddr)
	clustergo.OnNewStream(func(s *smux.Stream) {
		//处理stream
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
	clustergo.Wait()
}


```

gateserver.go

```go
package main

import (
	"io"
	"net"
	"sync"

	"github.com/sniperHW/netgo"
	"github.com/sniperHW/clustergo"
	"github.com/sniperHW/clustergo/addr"
	"github.com/sniperHW/clustergo/example/discovery"
	"github.com/sniperHW/clustergo/logger/zap"
)

func main() {
	l := zap.NewZapLogger("1.2.1.log", "./logfile", "debug", 1024*1024*100, 14, 28, true)
	clustergo.InitLogger(l.Sugar())
	localaddr, _ := addr.MakeLogicAddr("1.2.1")
	clustergo.Start(discovery.NewClient("127.0.0.1:8110"), localaddr)

	gameAddr, _ := clustergo.GetAddrByType(1)

	_, serve, _ := netgo.ListenTCP("tcp", "127.0.0.1:8113", func(conn *net.TCPConn) {
		go func() {
			//客户端连接到达，建立到1.1.1的stream
			cliStream, err := clustergo.OpenStream(gameAddr)
			if err != nil {
				conn.Close()
				return
			}

			defer func() {
				conn.Close()
				cliStream.Close()
			}()

			var wait sync.WaitGroup
			wait.Add(2)

			//将来自客户端的数据通过stream透传到1.1.1
			go func() {
				io.Copy(cliStream, conn)
				wait.Done()
			}()

			//将来自1.1.1的stream数据透传回客户端
			go func() {
				io.Copy(conn, cliStream)
				wait.Done()
			}()
			wait.Wait()
		}()
	})
	go serve()

	clustergo.Wait()
}
```

## Discovery

clustergo是为游戏服务端设计的分布式框架，游戏服务通常由一个个服务节点组成，每个节点提供一组内聚的功能服务。因此clustergo只提供粗粒度的节点发现功能。

Discovery更确切的说是一个配置管理服务，服务保存了所有成员的如下信息：

```go
type Node struct {
	Addr      addr.Addr
	Export    bool //是否将节点暴露到cluster外部
	Available bool //是否可用,
}
```

节点启动时，首先连接Discovery获取配置信息，之后通过配置的逻辑地址从配置中查询自身节点信息。如果找到则使用配置中的网络地址启动服务，否则启动失败。

clustergo没有提供默认的Discovery服务，使用者可以使用自己熟悉的组件来实现。

### 节点发现

普通节点只会将配置中Cluster相同或Export为true的节点保存到本地。可以通过`GetAddrByType`来获取某个类型的可用节点（Available=true）,如果有多个节点可用`GetAddrByType`将随机返回一个节点。




