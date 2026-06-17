# 重构 clustergo：移除 rpcgo 与 netgo.AsynSocket 依赖，poolbuff 优化发送

- 日期：2026-06-17
- 状态：已批准（设计阶段）
- 关联：`docs/superpowers/specs/2026-06-16-nats-jetstream-membership-design.md`（无关，仅同目录约定参考）

## 1. 目标与非目标

### 目标
1. 移除对 `github.com/sniperHW/rpcgo` 的依赖：将 RPC 实现**内联**进 clustergo，不再考虑跨项目复用。
2. 移除对 `netgo.AsynSocket` 的依赖：用 clustergo 内部的、**仅支持 TCP** 的异步 socket 取代。
3. 发送流程改用 `netgo/poolbuff` 优化：send goroutine 把一批已编码消息 gather 进单个 poolbuff 取出的 `[]byte`，单次 `conn.Write` 发送，写完复用缓冲（`[:0]`），退出时归还。
4. `netgo` 模块保留在 `go.mod`，但**只**通过 `github.com/sniperHW/netgo/poolbuff` 引入。

### 非目标
- 不改动 `membership`、`addr`、`pkg/crypto`、`codec/buffer`、`codec/pb` 等无关包。
- 不改动消息帧的业务语义（`flag|to|from|cmd|payload` 结构与 RPC 编码格式保持不变）。
- 不追求向后兼容公开 API（已确认可自由调整）。
- `example/stream`、`example/membership` 自行构造 `netgo.AsynSocket`，不属于 clustergo 的 `node` socket，本次不动。

## 2. 现状摘要（重构基线）

- `node.socket` 为 `*netgo.AsynSocket`。`netgo.AsynSocket` 含 send/recv 两个 goroutine：
  - sendloop 把消息经 `ObjCodec.Encode` 编码进 `net.Buffers`，凑到 `batchSendSize` 后经 `sendBuffs` 发送（TCP 走 `net.Buffers.WriteTo` 的 writev）。
  - recvloop 经 `tcpSocket.Recv`（即 `PacketReceiver.Recv`）读帧，`ObjCodec.Decode` 解码，回调 packet handler。
- `codec/ss.SSCodec` 同时实现 `netgo.ObjCodec`（Encode/Decode）与 `netgo.PacketReceiver`（通过内嵌 `codec.LengthPayloadPacketReceiver`，依赖 `netgo.ReadAble`）。
- `rpcgo` 提供 `Client/Server/Replyer/Channel/Codec/RequestMsg/ResponseMsg/Encode*/Decode*/BytesReader|Writer/NewError/错误码/Register[Arg]`。这些类型当前**泄漏进 clustergo 公开 API**（`NewClusterNode(rpccodec rpcgo.Codec)`、`RegisterService[Arg](... *rpcgo.Replyer ...)`、`RPCServer.SetInInterceptor([]func(*rpcgo.Replyer,*rpcgo.RequestMsg) bool)` 等）。
- `clustergo/logger.go` 的 `InitLogger` 把 logger 转发给 `rpcgo.InitLogger`，两个 `Logger` 接口方法集完全一致。
- `MaxPacketSize = 1024*4`（`codec/ss/message.go`）。
- `rpcChannel.Identity()` 无任何调用方（死代码，可删）。

## 3. 架构与依赖图

```
clustergo(root) ──► rpc, socket, codec/ss, codec, addr, membership, pkg/crypto
  rpc            ──► (仅标准库)            [内联简化后的 rpcgo]
  socket         ──► netgo/poolbuff         [TCP-only 异步 socket]
  codec/ss       ──► rpc, socket(Codec 接口 + MaxPacketSize), addr, codec/buffer, codec/pb
  netgo/poolbuff ──► (外部，唯一保留的 netgo 引入)
```

无循环依赖。方向：root → 各子包；codec/ss → socket（取 `Codec` 接口与 `MaxPacketSize`）；socket → poolbuff。socket 不依赖 codec/ss（接口由 codec/ss 隐式实现）。

## 4. 新增 `clustergo/socket` 包（取代 netgo.AsynSocket）

TCP-only 异步 socket，发送走 poolbuff gather，接收自带 length-prefix 分帧。

### 4.1 对外 API

```go
package socket

type Codec interface {
    // Encode 把一条完整帧（4 字节长度前缀 + payload）追加到 dst，返回新 dst 与本次追加字节数。
    Encode(dst []byte, o interface{}) (newDst []byte, n int)
    // Decode 解析 payload（长度前缀已由 socket 剥离）为消息对象。
    Decode(payload []byte) (interface{}, error)
}

type Options struct {
    SendChanSize  int           // 默认 256
    BatchSendSize int           // 默认 65535
    SendTimeout   time.Duration // 异步写的 deadline；0 表示不设
    AutoRecv      bool          // 处理完一包后自动继续 Recv
    Context       context.Context
}

type Socket struct { /* conn *net.TCPConn; codec Codec; die chan struct{}; sendReq chan interface{}; ... */ }

// 用 stdlib 包装 net.ListenTCP（约 15 行），取代 netgo.ListenTCP。
func ListenTCP(addr string, onConn func(*net.TCPConn)) (net.Listener, func(), error)

func New(conn *net.TCPConn, codec Codec, opts Options) *Socket

func (s *Socket) Send(o interface{}, deadline time.Time) error
func (s *Socket) SendWithContext(ctx context.Context, o interface{}) error
func (s *Socket) Recv()
func (s *Socket) Close(err error)
func (s *Socket) SetPacketHandler(func(ctx context.Context, packet interface{}) error)
func (s *Socket) SetCloseCallback(func(err error))
```

`MaxPacketSize` 常量与本包的错误哨兵值（见 §7）定义在 `socket` 包；`codec/ss` 通过 `socket.MaxPacketSize` 引用以保持单一来源。

### 4.2 sendloop（poolbuff gather）

每个连接的 send goroutine 生命周期内**只取一次** poolbuf，循环用 `[:0]` 复用，goroutine 退出时归还：

```go
buf := poolbuff.Get()
defer poolbuff.Put(buf)
total := 0
for {
    select {
    case <-s.die:
        if total > 0 { _ = s.write(buf) } // 尽力把残留 flush
        return
    case o := <-s.sendReq:
        before := len(buf)
        buf, _ = s.codec.Encode(buf, o) // 整帧 append 进 buf
        total += len(buf) - before
        if total >= s.batchSendSize || len(s.sendReq) == 0 {
            if err := s.write(buf); err != nil {
                s.close(err)
                return
            }
            buf = buf[:0]
            total = 0
        }
    }
}
```

- `s.write(buf)` = `conn.SetWriteDeadline(dl)` + 单次 `conn.Write(buf)`。
- 每批仅 1 次 syscall；gather 缓冲跨批复用，稳态发送零分配。
- `deadline` 语义沿用现状：不传 → sendReq 满则永久等待；`IsZero()` 或已过期 → 满即返回 `ErrSendQueueFull`；否则等到 deadline 返回 `ErrPushToSendQueueTimeout`。

### 4.3 recvloop（自带分帧）

socket 内联 length-prefix 分帧逻辑（即原 `LengthPayloadPacketReceiver`，约 40 行，直接作用于 `*net.TCPConn`）：读 `[4 字节 len][len 字节 payload]`，`len` 越界返回错误；剥掉长度前缀后把 payload 交给 `codec.Decode`，再回调 packet handler；`AutoRecv` 为真则重新投递 recv 请求。网络超时 → `onRecvTimeout`（随后 close，与现状一致）；真实错误 → close。

### 4.4 关闭协调

比 `AsynSocket` 的 `wrCounter` 更简单：用两个 `done chan struct{}` 跟踪 send/recv goroutine 存活；`doClose`（回调 + `conn.Close`，`sync.Once` 保证一次）在两者均已退出时触发；若二者都未启动则 `Close` 时直接 `doClose`。

## 5. 新增 `clustergo/rpc` 包（内联简化 rpcgo）

把 `rpcgo` 的 `rpc.go / server.go / client.go / bytes.go / logger.go` 原样移植为 `clustergo/rpc`，再做下述简化（依据：node 从不使用断连快速失败路径，始终 `OnMessage(nil, resp)`）。

### 5.1 删除
- `ChannelInterestDisconnect`、`PendingCall` 接口，以及 `Client` 中 `putPending/loadAndDeletePending/deletePending` 的 channel 类型分支：`OnMessage(resp)` 一律走 32 分片 pending map。
- 未被调用的 `Identity()` 方法（rpcChannel 上）。
- rpcgo 包级泛型助手 `Call/Post/CallWithTimeout/AsyncCall`（clustergo 在根包有自己的同名函数）。

### 5.2 保留
- `Client`：`Call / AsyncCall / CallWithTimeout / OnMessage / SetInInterceptor / SetOutInterceptor / makeSequence`（序列号生成逻辑不变）。
- `Server`：`OnMessage / SetInInterceptor / Stop`，方法表与 in 拦截器。
- `Replyer`（`Reply/Error/Channel/AppendOutInterceptor`）、`Channel`（`Request/RequestWithContext/Reply/Name/IsRetryAbleError`）、`Codec`。
- `RequestMsg/ResponseMsg`、`Encode/DecodeRequest/Response`、`BytesReader/Writer`、错误码与 `NewError`、泛型 `Register[Arg]`、`Logger` 接口。

### 5.3 日志接入
`rpc` 包持有 package-level `logger Logger` 与 `InitLogger(Logger)`。`clustergo.InitLogger` 改为同时设置根包 `logger` 与 `rpc.InitLogger(l)`（`Logger` 接口方法集与现状完全一致，无破坏）。

## 6. codec/ss 与 codec/receiver 重构

- `codec/ss/codec.go`：
  - `SSCodec` 改为实现 `socket.Codec`。
  - `Encode(dst []byte, o interface{}) ([]byte, int)`：把整帧 `[len][flag][to][from][cmd][data]` 追加进 `dst`（取代基于 `net.Buffers` 的版本）；内部 `encode(...)` 同步改写为 `[]byte` 追加。
  - `Decode(payload []byte)`：不变（仍解析 `flag|to|from|cmd|data`）。
  - **移除**内嵌的 `codec.LengthPayloadPacketReceiver`（分帧已移入 socket）。`NewCodec(selfAddr)` 不再初始化该内嵌字段。
- `codec/ss/message.go`：`rpcgo` 引入改为 `rpc`，其余不变；删除本地 `MaxPacketSize` 常量，改引用 `socket.MaxPacketSize`（单一来源，见 §4.1）。
- **删除 `codec/receiver.go`**：其唯一消费者是 `SSCodec` 内嵌的接收器，现已无引用。

## 7. node.go / rpc.go / cluster.go 重接

- `node.go`：
  - `node.socket` 类型 `*netgo.AsynSocket` → `*socket.Socket`。
  - `onEstablish`：`socket.New(conn, ssCodec, socket.Options{SendChanSize, BatchSendSize, AutoRecv:true, Context})`，随后 `SetPacketHandler`/`SetCloseCallback`/`Recv()`。
  - `Send/SendWithContext/Recv/Close/SetPacketHandler/SetCloseCallback` 对接到新 API；pending 消息重放逻辑不变。
  - `rpcgo` → `rpc`。
- `rpc.go`：
  - `rpcgo.*` → `rpc.*`。
  - `netgo.ErrSendQueueFull`/`netgo.ErrPushToSendQueueTimeout` → `socket.ErrSendQueueFull`/`socket.ErrPushToSendQueueTimeout`（`IsRetryAbleError`、`AsyncCall` 的 `ErrBusy` 映射同步更新）。
  - 删除 `rpcChannel.Identity()`。
- `cluster.go`：
  - `netgo.ListenTCP` → `socket.ListenTCP`。
  - `rpcgo.Codec/NewServer/NewClient/Register` → `rpc.*`。
  - 公开签名更新：`NewClusterNode(rpccodec rpc.Codec)`、`RegisterService[Arg](name, func(ctx, *rpc.Replyer, *Arg))`、`RPCServer.SetInInterceptor([]func(*rpc.Replyer, *rpc.RequestMsg) bool)`、`RPCClient.SetInInterceptor([]func(*rpc.RequestMsg, any, error))`、`var RPCCodec rpc.Codec = PbCodec{}`。

## 8. 错误、配置与日志

- `socket` 包定义哨兵错误：`ErrSendQueueFull`、`ErrPushToSendQueueTimeout`、`ErrSocketClosed`、`ErrRecvTimeout`，并提供 `IsTimeout`/`IsNetTimeoutError` 辅助（迁移自 netgo）。
- 配置变量保留在根包：`SendChanSize`(256)、`DefaultSendTimeout`、`MaxPendingMsgSize`(1024)；新增 `BatchSendSize`(65535)。
- `logger.go`：移除 `rpcgo` 引入；`InitLogger` 改设 `rpc` 包 logger。

## 9. 测试与示例

- `cluster_test.go`、`codec/ss/ss_test.go`：`rpcgo` → `rpc`；`registerService`/服务函数签名使用 `*rpc.Replyer`、`*rpc.RequestMsg`。这是主要验证手段（端到端建连、RPC Call/AsyncCall/Post、SendPbMessage/SendBinMessage、relay、拦截器）。
- `example/pbrpc/service/{echo,test}` 及 `genrpc/gen.go` 模板：`*rpcgo.Replyer` → `*rpc.Replyer`，`rpcgo.Channel` → `rpc.Channel`。
- `example/stream`、`example/membership`：维持现状（它们直接用 netgo，不经过 clustergo 的 `node` socket）。
- `go.mod`：保留 `netgo`；从 `require` 中**移除** `rpcgo`。

## 10. 验证计划

1. `go build ./...` 通过。
2. `go vet ./...` 通过。
3. `go test ./...` 通过，重点：
   - `cluster_test.go`：节点间 RPC（同步/异步/oneway）、消息收发、harbor relay、拦截器与 pending 计数。
   - `codec/ss/ss_test.go`：Encode/Decode 往返。
4. 可选：为 `socket` 包加一个 loopback TCP 单元测试（起一对 conn，Send→Recv 一条消息，校验 poolbuff 路径）。
5. 确认 `grep -rn 'rpcgo\|netgo.AsynSocket\|netgo\.NewAsynSocket\|netgo\.NewTcpSocket'` 在 clustergo 非 example 代码中无残留（example/stream、example/membership 例外）。

## 11. 风险与取舍

- **gather 写入有一次 memcpy**：每条 ≤4KB，相对省下的 syscall 与 GC 开销可忽略；换来更简单的单缓冲路径与零稳态分配。
- **公开 API 破坏**：已与作者确认可接受；下游服务（vra-service 等）如引用 clustergo 需跟随更新 `rpcgo.*` → `rpc.*` 类型。
- **删除断连快速失败路径**：node 现状未使用（pending 走 32 分片 map + 各自 deadline/ctx 超时），删除不影响行为。
- **分帧逻辑从 codec 迁到 socket**：`codec/receiver.go` 删除后，若未来有非 socket 消费者需要分帧，需重新引入；当前无此消费者。
