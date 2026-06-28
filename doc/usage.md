# 使用指南

## 初始化

```go
import (
    "github.com/sniperHW/clustergo"
    "github.com/sniperHW/clustergo/addr"
    "github.com/sniperHW/clustergo/logger/zap"
)

// 1. 初始化日志（必须在其他操作之前）
l := zap.NewZapLogger("1.1.1.log", "./logfile", "debug", 1024*1024*100, 14, 28, true)
clustergo.InitLogger(l.Sugar())

// 2. 创建 Membership 实现
mb := redis.Membership{RedisCli: redisCli}
// 或
mb := etcd.Membership{Cfg: clientv3.Config{Endpoints: []string{"localhost:2379"}}, PrefixConfig: "/cluster/nodes/", PrefixAlive: "/cluster/alive/"}

// 3. 解析本地逻辑地址
localAddr, _ := addr.MakeLogicAddr("1.1.1")

// 4. 注册消息处理器（必须在 Start 之前）
clustergo.RegisterProtoHandler(&MyPbMessage{}, func(ctx context.Context, from addr.LogicAddr, msg proto.Message) {
    // 处理 protobuf 消息
})

clustergo.RegisterBinaryHandler(cmdID, func(ctx context.Context, from addr.LogicAddr, cmd uint16, msg []byte) {
    // 处理二进制消息
})

// 5. 注册 RPC 服务（必须在 Start 之前）
clustergo.RegisterService[MyReq]("myMethod", func(ctx context.Context, r *rpcgo.Replyer, arg *MyReq) {
    r.Reply(&MyResp{...})
})

// 6. 启动节点
clustergo.Start(mb, localAddr)

// 7. 等待停止
clustergo.Wait()
```

## 消息收发

### 发送 Protobuf 消息

```go
// 必须先注册消息类型
pb.Register("ss", &MyMessage{}, 1001)

// 发送（带默认超时 200ms）
err := clustergo.SendPbMessage(toAddr, &MyMessage{Field: "value"})

// 发送（带自定义 deadline）
err := clustergo.SendPbMessage(toAddr, &MyMessage{}, time.Now().Add(time.Second))
```

### 发送二进制消息

```go
err := clustergo.SendBinMessage(toAddr, cmdID, []byte("data"))
err := clustergo.SendBinMessage(toAddr, cmdID, []byte("data"), time.Now().Add(time.Second))
```

### 注册消息处理器

```go
// Protobuf 消息处理器
clustergo.RegisterProtoHandler(&MyMessage{}, func(ctx context.Context, from addr.LogicAddr, msg proto.Message) {
    m := msg.(*MyMessage)
    // ...
})

// 二进制消息处理器
clustergo.RegisterBinaryHandler(100, func(ctx context.Context, from addr.LogicAddr, cmd uint16, msg []byte) {
    // ...
})
```

## RPC 调用

```go
// 同步调用
ret, err := clustergo.Call[MyReq, MyResp](ctx, peerAddr, "method", &MyReq{})

// 带超时
ret, err := clustergo.CallWithTimeout[MyReq, MyResp](peerAddr, "method", &MyReq{}, time.Second*3)

// 异步调用
clustergo.AsyncCall[MyReq, MyResp](peerAddr, "method", &MyReq{}, time.Now().Add(time.Second), func(ret *MyResp, err error) {
    // callback
})

// 单向（不等待响应）
clustergo.Post[MyReq](ctx, peerAddr, "method", &MyReq{})
```

## 节点发现

```go
// 按 type 随机获取一个可用节点地址
addr, err := clustergo.GetAddrByType(2)      // type=2 的节点
addr, err := clustergo.GetAddrByType(2, 5)   // type=2, index=5（取模选择）
```

## 流式连接

```go
// 服务端
clustergo.StartSmuxServer(func(s *smux.Stream) {
    go handleStream(s)
})

// 客户端
stream, err := clustergo.OpenStream(peerAddr)
stream.Write(data)
buf := make([]byte, 1024)
n, err := stream.Read(buf)
stream.Close()
```

## 优雅停止

```go
clustergo.Stop()   // 触发停止（关闭 listener、等待 RPC 完成、关闭所有连接）
clustergo.Wait()   // 阻塞直到完全停止
```

## 使用独立 Node 实例（非默认单例）

```go
node := clustergo.NewClusterNode(clustergo.RPCCodec)
node.RegisterProtoHandler(&MyMsg{}, handler)
node.Start(mb, localAddr)
// 使用 node 的方法而非包级函数
node.SendPbMessage(to, msg)
ret, err := clustergo.Call[Arg, Ret](ctx, node, to, "method", arg)  // 小写版本的独立 node 函数
node.Stop()
node.Wait()
```

## 配置参数

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `SendChanSize` | 256 | socket 异步发送 chan 大小 |
| `DefaultSendTimeout` | 200ms | 默认发送超时 |
| `MaxPendingMsgSize` | 1024 | pending 队列最大长度 |
| `MaxPacketSize` | 4096 | 最大数据包字节数 |

## 成员发现配置

### Redis

```go
import "github.com/sniperHW/clustergo/membership/redis"

mb := &redis.Membership{
    RedisCli: redisClient,  // *redis.Client
}
```

需要部署 Redis，Lua 脚本自动执行。Admin 操作通过 `redis.Membership` 的 `UpdateMember` / `RemoveMember` / `CheckTimeout` 方法。

### etcd

```go
import "github.com/sniperHW/clustergo/membership/etcd"

mb := &etcd.Membership{
    Cfg:          clientv3.Config{Endpoints: []string{"localhost:2379"}},
    PrefixConfig: "/cluster/nodes/",
    PrefixAlive:  "/cluster/alive/",
    TTL:          5 * time.Second,
}
```

需要部署 etcd 集群。

### 自定义

实现 `membership.Membership` 接口即可：

```go
type Membership interface {
    Subscribe(func(MemberInfo)) (func(), error)
    UpdateMember(Node) error
    RemoveMember(addr.LogicAddr) error
    KeepAlive(addr.LogicAddr, int) error
}
```

## 典型部署架构

```
Cluster 1                          Cluster 2
┌──────────────────┐               ┌──────────────────┐
│  Gate(1.1.x)     │               │  Gate(2.1.x)     │
│  Game(1.2.x)     │               │  Game(2.2.x)     │
│  Harbor(1.255.x) ├─── TCP ───────┤  Harbor(2.255.x) │
└──────────────────┘               └──────────────────┘
        │                                  │
        └──── Redis/etcd (member discovery) ┘
```

- 同 Cluster 内节点直接 TCP 互联
- 跨 Cluster 消息通过 Harbor 节点转发（RelayMessage 透传）
- 所有节点连接到同一个成员发现后端（Redis 或 etcd）
