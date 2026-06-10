# RPC 框架

## 概述

clustergo 在节点间通信之上封装了泛型 RPC 框架，底层使用 `github.com/sniperHW/rpcgo`。RPC 请求/响应作为 `ss.Message` 的 payload 传输（flag 为 `RpcReq` / `RpcResp`）。

## Channel 适配

RPC 需要一个 `rpcgo.Channel` 实现，clustergo 提供两种：

### rpcChannel（远程调用）
```go
type rpcChannel struct {
    peer addr.LogicAddr
    node *node
    self *Node
}
```
- `Request` / `RequestWithContext`：通过 `node.sendMessage` 发送 RPC 请求
- `Reply`：通过 `node.sendMessage` 发送 RPC 响应
- `IsRetryAbleError`：`ErrPendingQueueFull`、`ErrSendQueueFull`、`ErrPushToSendQueueTimeout` 返回 true

### selfChannel（自调用）
```go
type selfChannel struct {
    self *Node
}
```
- 当 `to == self.localAddr` 时使用
- `Request`：通过 `Go(func())` 将请求投递到 goroutine 池处理
- `Reply`：通过 `Go(func())` 将响应投递回 RPC client
- `IsRetryAbleError`：始终返回 false（自调用不应重试）

## Codec

默认使用 `PbCodec`（protobuf 序列化），也提供 `JsonCodec`：

```go
var RPCCodec rpcgo.Codec = PbCodec{}
```

## 公开 API（包级函数，使用默认 Node）

### 注册服务端方法

```go
// 泛型注册，Arg 为请求参数类型
clustergo.RegisterService[Arg](name string, method func(context.Context, *rpcgo.Replyer, *Arg)) error
```

### 调用方式

```go
// 同步调用（带 context）
ret, err := clustergo.Call[Arg, Ret](ctx, peerAddr, "method", arg)

// 同步调用（带超时 duration）
ret, err := clustergo.CallWithTimeout[Arg, Ret](peerAddr, "method", arg, time.Second)

// 异步调用（回调方式）
clustergo.AsyncCall[Arg, Ret](peerAddr, "method", arg, deadline, func(ret *Ret, err error) {
    // 处理结果
})

// 单向调用（不等待响应）
clustergo.Post[Arg](ctx, peerAddr, "method", arg)
```

### 使用独立 Node 实例

所有包级函数都有对应的小写版本，接受额外的 `*Node` 参数（用于测试）：

```go
ret, err := clustergo.call[Arg, Ret](ctx, node, peerAddr, "method", arg)
err := clustergo.post[Arg](ctx, node, peerAddr, "method", arg)
```

## RPCServer 拦截器

```go
// 设置入站拦截器
node.GetRPCServer().SetInInterceptor([]func(*rpcgo.Replyer, *rpcgo.RequestMsg) bool{})
```

默认内置一个拦截器统计 `pendingRespCount`，用于优雅停机（等待所有 RPC 请求完成）。

## RPCClient 拦截器

```go
// 设置入站拦截器（响应处理前）
node.GetRPCClient().SetInInterceptor([]func(*rpcgo.RequestMsg, interface{}, error){})

// 设置出站拦截器（请求发送前）
node.GetRPCClient().SetOutInterceptor([]func(*rpcgo.RequestMsg, interface{}){})
```

## 错误处理

RPC 调用返回的错误会区分可重试错误：
- `ErrBusy`（由 `ErrPendingQueueFull` 或 `ErrSendQueueFull` 转换而来）
- 其他错误（目标不可达、超时等）

## 最佳实践：封装服务

参考 `example/pbrpc/service/echo/echo.go`，为每个 RPC 服务封装类型安全的调用函数：

```go
// 1. 定义 Replyer 包装
type Replyer struct { replyer *rpcgo.Replyer }

// 2. 定义服务接口
type Echo interface {
    ServeEcho(context.Context, *Replyer, *EchoReq)
}

// 3. 注册函数
func Register(o Echo) {
    clustergo.RegisterService("echo", func(ctx context.Context, r *rpcgo.Replyer, arg *EchoReq) {
        o.ServeEcho(ctx, &Replyer{replyer: r}, arg)
    })
}

// 4. 类型安全的客户端调用函数
func Call(ctx context.Context, peer addr.LogicAddr, arg *EchoReq) (*EchoRsp, error) {
    return clustergo.Call[*EchoReq, EchoRsp](ctx, peer, "echo", arg)
}
```

在服务端 handler 中可以通过 `replyer.Channel().(clustergo.RPCChannel).Peer()` 获取请求来源的逻辑地址。

## 获取对端地址

```go
// RPCChannel 接口
type RPCChannel interface {
    Peer() addr.LogicAddr
}
```
