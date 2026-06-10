# 消息编解码与节点间通信

## 线路格式（ss codec）

所有节点间 TCP 连接使用统一的 `SSCodec`，数据包格式：

```
[4字节 payload 长度] [1字节 flag] [4字节 to] [4字节 from] [2字节 cmd(可选)] [payload 数据]
```

### flag 字段

低 3 位表示消息类型：

| 值 | 常量 | 含义 |
|----|------|------|
| 0x1 | `PbMsg` | protobuf 消息，含 cmd 字段 |
| 0x2 | `BinMsg` | 二进制消息，含 cmd 字段 |
| 0x3 | `RpcReq` | RPC 请求，无 cmd |
| 0x4 | `RpcResp` | RPC 响应，无 cmd |

### 消息类型

- **`Message`**：正常消息，目标节点为当前节点时解码为此类型。Payload 可以是 `proto.Message`、`[]byte`、`*rpcgo.RequestMsg`、`*rpcgo.ResponseMsg`
- **`RelayMessage`**：透传消息，目标节点不是当前节点时解码为此类型，保留原始 wire 数据用于转发

## Protobuf 消息注册

`codec/pb` 提供命名空间隔离的 protobuf 消息注册表：

```go
// 注册（初始化阶段调用，非线程安全）
pb.Register("ss", &MyMessage{}, 1001)

// 获取 cmd ID
cmd := pb.GetCmd("ss", &MyMessage{})

// Marshal/Unmarshal（线程安全）
data, cmd, err := meta.Marshal(msg)
msg, err := meta.Unmarshal(cmd, data)
```

内部使用分段存储：
- ID < 65536：数组直接索引（O(1)）
- ID >= 65536：map 查找

## 消息发送流程

```
Node.SendPbMessage(to, msg, deadline)
├── to == self → Go(func() { dispatchProto(...) })
├── to != self
│   ├── getNodeByLogicAddr(to) → *node
│   ├── node.sendMessage(self, ss.NewMessage(...), deadline)
│   │   ├── socket != nil（已连接）→ socket.Send(msg, deadline)
│   │   └── socket == nil（未连接）
│   │       ├── pendingMsg.Len() >= MaxPendingMsgSize → 清理失败消息，满则返回 ErrPendingQueueFull
│   │       ├── 推入 pendingMsg 队列
│   │       └── 队列长度 == 1 → Go(func() { node.dial(self) })
```

### pending 队列机制

连接建立前，消息缓存在 `node.pendingMsg`（`list.List`）中：
- 最大容量 `MaxPendingMsgSize = 1024`
- `deadline.IsZero()` 的消息优先级最低，队列满时直接丢弃
- 连接建立后（`onEstablish`），按序投递所有未过期的 pending 消息
- dial 失败后清理过期消息，如果仍有 pending 消息则 1 秒后重试 dial

### deadline 语义

- `deadline.IsZero()`：最低优先级，队列满时丢弃，不等待连接建立
- `deadline != 0`：包含连接建立时间，到期消息被丢弃；连接建立后也会检查
- `SendChanSize = 256`：异步发送缓冲区大小
- `DefaultSendTimeout = 200ms`

## 消息接收流程

```
SSCodec.Decode(payload)
├── 解析 flag、to、from
├── isTarget(to) == true
│   ├── PbMsg → Unmarshal → *Message{payload: proto.Message}
│   ├── BinMsg → *Message{payload: []byte}
│   ├── RpcReq → DecodeRequest → *Message{payload: *rpcgo.RequestMsg}
│   └── RpcResp → DecodeResponse → *Message{payload: *rpcgo.ResponseMsg}
└── isTarget(to) == false → *RelayMessage（透传给 onRelayMessage 转发）
```

收到 `*Message` 后由 `node.onMessage` 分发：
- `proto.Message` → `msgManager.dispatchProto`
- `[]byte` → `msgManager.dispatchBinary`
- `*rpcgo.RequestMsg` → `rpcSvr.svr.OnMessage`
- `*rpcgo.ResponseMsg` → `rpcCli.cli.OnMessage`

## 握手协议

新 TCP 连接建立后执行加密握手：

1. 发送方：
   - 构造 `Handshake{LogicAddr, NetAddr, IsStream}` JSON
   - AES-CBC 加密（密钥 `cecret_key`）
   - 发送 `[4字节长度][加密数据]`

2. 接收方：
   - 读取长度 + 加密数据
   - AES-CBC 解密
   - JSON 反序列化得到 Handshake
   - 验证 LogicAddr 存在于 nodeCache 且 NetAddr 匹配
   - 发送 4 字节 `0x00000000` 表示成功

3. 非 Stream 连接：处理同时 dial 冲突（逻辑地址小的放弃 accept）

4. Stream 连接：在 TCP 连接上建立 smux session
