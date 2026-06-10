# 整体架构

## 项目定位

clustergo 是一个 Go 语言实现的**集群内节点通信框架**。它为分布在不同机器上的服务节点提供：

- 基于**逻辑地址**的消息路由（支持同集群直连、跨集群通过 Harbor 转发）
- 透明的 TCP 连接管理（自动建连、重连、pending 队列）
- protobuf / 二进制消息分发
- 泛型 RPC 调用（同步 / 异步 / 超时）
- 基于 smux 的流式连接
- 可插拔的成员发现机制（redis / etcd）

## 核心模块

```
clustergo/
├── addr/          逻辑地址编码与解析
├── codec/
│   ├── buffer/    二进制读写工具
│   ├── pb/        protobuf 消息注册表（命名空间 + ID 映射）
│   └── ss/        节点间通信编解码器（flag + to + from + payload）
├── membership/    成员发现接口与数据结构
│   ├── redis/     Redis 实现（pub/sub + lua script）
│   └── etcd/      etcd 实现（watch + lease）
├── pkg/crypto/    AES-CBC 加解密（用于握手）
├── logger/        日志接口
├── logfile/       日志文件轮转
├── node.go        内部 node 结构（连接管理、pending 队列、dial 状态机）
├── cluster.go     Node 公开 API（Start/Stop/Send/RPC）、消息分发、nodeCache
├── rpc.go         RPC channel 适配（rpcChannel / selfChannel）、RPCClient/RPCServer 封装
└── logger.go      Logger 接口定义
```

## 启动流程

```
1. 调用者创建 Membership 实现（redis/etcd）
2. 调用 clustergo.Start(mb, logicAddr)
   ├── mb.Subscribe(callback)   // 订阅成员变更
   ├── 等待首次初始化完成（initC channel）
   ├── 在 nodeCache 中查找自身节点
   ├── 启动 KeepAlive 定时器（每 2 秒）
   ├── netgo.ListenTCP 监听自身网络地址
   └── 启动 goroutine 池（16 个 worker）
3. 节点开始接受连接、建立到其他节点的连接、收发消息
```

## 节点连接建立

每个节点同时作为 TCP server 和 TCP client：

- **Server 侧**：`netgo.ListenTCP` 监听，`onNewConnection` 处理入站连接，执行 AES 加密的 JSON 握手
- **Client 侧**：当有消息待发送时触发 `dial()`，向对端发起 TCP 连接并完成握手
- **同时 dial 冲突**：两端同时发起连接时，逻辑地址较小的一方放弃 accept（`onNewConnection` 中判断 `pendingMsg.Len() != 0`）

## 消息路由

```
发送消息到 to（逻辑地址）
├── to == self → 投递到自身 goroutine pool 处理
├── to.Cluster() == self.Cluster() → 从 nodeCache 查找对端 node，直接发送
└── to.Cluster() != self.Cluster() → 通过 Harbor 节点转发
    ├── self 是 Harbor → 选择与 to 同 cluster 的 Harbor
    └── self 非 Harbor → 选择本 cluster 内的 Harbor
```

当消息到达中间节点但 `to` 不是自身时，codec 层 Decode 返回 `RelayMessage`，`onRelayMessage` 负责转发到下一跳。如果无法路由且是 RPC 请求，会返回错误响应。

## goroutine 池

`gopool` 使用 16 个 worker goroutine 从 `taskqueue` 消费任务。队列满时 fallback 到新建 goroutine 执行。用于：
- 自发自收消息的分发
- 节点建连的 dial 发起
