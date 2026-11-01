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

## 公开 API（Node 的泛型成员函数）

所有 RPC 调用接口都是 `*Node` 上的泛型成员函数（Go 1.27 起方法可以自带类型参数），
类型参数由实参类型或 method 的函数类型推导，通常无需显式写出。
默认单例节点通过 `clustergo.GetDefaultNode()` 获取。

```go
node := clustergo.GetDefaultNode()
```

### 注册服务端方法

```go
// Arg 为请求参数类型，由 method 的函数类型推导
node.RegisterService[Arg](name string, method func(context.Context, *rpc.Replyer, *Arg)) error

```

### 调用方式

```go
// 同步调用（带 context）
ret, err := node.Call[Arg, Ret](ctx, peerAddr, "method", arg)

// 同步调用（带超时 duration）
ret, err := node.CallWithTimeout[Arg, Ret](peerAddr, "method", arg, time.Second)

// 异步调用（回调方式）
node.AsyncCall[Arg, Ret](peerAddr, "method", arg, deadline, func(ret *Ret, err error) {
    // 处理结果
})

// 单向调用（不等待响应）
node.Post[Arg](ctx, peerAddr, "method", arg)
```

### 多节点场景

传入不同的 `*Node` 即可在多个节点实例上注册/调用，无需额外的包级函数：

```go
var node1, node2 *clustergo.Node
node1.RegisterService("hello", handler)                                    // 在 node1 上注册
ret, err := node2.Call[string, string](ctx, node1Addr, "hello", "sniperHW") // 从 node2 调用
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
type Replyer struct { replyer *rpc.Replyer }

// 2. 定义服务接口
type Echo interface {
    ServeEcho(context.Context, *Replyer, *EchoReq)
}

// 3. 注册函数（node 为承载该服务的节点）
func Register(node *clustergo.Node, o Echo) {
    node.RegisterService("echo", func(ctx context.Context, r *rpc.Replyer, arg *EchoReq) {
        o.ServeEcho(ctx, &Replyer{replyer: r}, arg)
    })
}

// 4. 类型安全的客户端调用函数
func Call(node *clustergo.Node, ctx context.Context, peer addr.LogicAddr, arg *EchoReq) (*EchoRsp, error) {
    return node.Call[*EchoReq, EchoRsp](ctx, peer, "echo", arg)
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
