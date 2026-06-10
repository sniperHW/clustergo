# 流式连接（Stream）

## 概述

clustergo 通过 `github.com/xtaci/smux` 在节点间 TCP 连接上复用出多条独立的双向流（Stream）。适用于需要持续数据传输的场景，如代理转发。

## Server 端

```go
// 注册流处理器
clustergo.StartSmuxServer(func(s *smux.Stream) {
    // 处理新到达的 stream
    go func() {
        buf := make([]byte, 1024)
        for {
            n, err := s.Read(buf)
            if err != nil { break }
            s.Write(buf[:n])
        }
        s.Close()
    }()
})
```

内部机制：
- 入站 TCP 连接握手时 `IsStream == true`，`onNewConnection` 创建 `smux.Server(conn)`
- 为每个 smux session 启动 `listenStream` goroutine，持续 `AcceptStream` 并回调 `onNewStream`
- session 关闭时从 `smuxSessions` sync.Map 中移除

## Client 端

```go
// 打开到对端的 stream
stream, err := clustergo.OpenStream(peerAddr)
if err != nil {
    return err
}
stream.Write(data)
stream.Read(buf)
stream.Close()
```

内部机制（`node.openStream`）：
- 每个远程 `node` 维护一个 `streamClient`（互斥锁 + smux session）
- 首次 `OpenStream` 时建立 TCP 连接、执行 Stream 握手、创建 `smux.Client(conn)`
- 后续 `OpenStream` 复用已有 session
- session 故障时自动重建
- `smux.ErrGoAway`（stream ID 溢出）时返回错误，由调用方决定是否重试

## 示例：TCP 代理转发

`example/stream/` 展示了 Gate 代理 Game 的场景：

```
外部客户端 → TCP连接 → Gate(1.2.1) → smux Stream → Game(1.1.1)
```

Gate 收到外部 TCP 连接后：
1. 通过 `GetAddrByType(1)` 找到 Game 节点
2. 调用 `OpenStream(gameAddr)` 建立到 Game 的流
3. 启动两个 goroutine 做 `io.Copy` 双向转发

## 注意事项

- Stream 连接与普通消息连接是**独立的 TCP 连接**（握手时 `IsStream` 区分）
- 每个 node 最多维护一个 smux session（通过 `streamClient` 互斥管理）
- smux stream ID 溢出时返回 `ErrGoAway`，需要新建连接
