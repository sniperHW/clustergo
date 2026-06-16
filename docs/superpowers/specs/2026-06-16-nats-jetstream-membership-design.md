# NATS JetStream Membership 实现设计

**日期**：2026-06-16
**作者**：clustergo 维护者
**状态**：已批准，待实现

## 背景与目标

`membership` 包当前已有两个后端实现：

- `membership/redis`：Lua 脚本原子操作 + Pub/Sub 通知 + 单独的 `CheckTimeout`
- `membership/etcd`：lease 自动到期保活 + Watch 监听前缀变更

本设计新增第三个后端 `membership/natsjet`，使用 NATS JetStream KV 实现同一套 `membership.Membership` 接口，为已有 NATS 基础设施的部署场景提供开箱即用的成员发现能力。

## 关键技术约束

NATS JetStream KV bucket 的 **TTL 是 bucket 级别**的——不像 etcd lease 或 redis hash field 那样可以 per-key/per-lease 设置。

后果：`KeepAlive(addr.LogicAddr, second int)` 接口中的 `second` 参数无法真的用来逐 key 控制 TTL。所有 alive key 共享同一个 TTL，由 `Membership.AliveTTL` 配置项决定。**每次 `Put` 都会重置该 key 的过期计时**，这正好与 etcd lease 的"心跳续期"语义吻合。

**关于 per-key TTL**：nats.go 较新版本提供 `KeyTTL()` option，但仅能在 `Create()` 调用时设置，而 `Create()` 在 key 已存在时会返回 `nats: key exists`。心跳场景下每次都需要"刷新 TTL"，必须先 `Delete` 再 `Create`，这破坏了 put-if-absent 语义并引入竞态——不实用。因此本设计坚持 bucket 级 TTL。

## 接口契约

严格遵循 `membership.Membership` 接口，**不**额外暴露 `CheckTimeout`（与 etcd 实现保持一致）：

```go
type Membership struct {
    Servers   string        // NATS 服务器地址，如 "nats://127.0.0.1:4222"
    Prefix    string        // bucket 名前缀，默认 ""
    AliveTTL  time.Duration // alive bucket 的 TTL，默认 10s
    Logger    clustergo.Logger
    // 私有字段略
}

func (s *Membership) Subscribe(cb func(membership.MemberInfo)) (func(), error)
func (s *Membership) UpdateMember(n membership.Node) error
func (s *Membership) RemoveMember(la addr.LogicAddr) error
func (s *Membership) KeepAlive(la addr.LogicAddr, _ int) error  // second 参数被忽略
```

## 架构

```
membership/natsjet/
├── nats.go          // Membership 结构体定义、init()、EnsureBucket、GetNatsError 辅助
├── admin.go         // UpdateMember / RemoveMember / KeepAlive
├── subscribe.go     // Subscribe / watch goroutine / handleConfigEntry / handleAliveEntry
├── client_test.go   // 集成测试
└── start.sh         // 启动单节点 nats-server，开启 JetStream
```

**职责边界**：
- `nats.go` 只负责连接 + bucket 元数据；不含业务逻辑
- `admin.go` 是写入面（CRUD），无状态
- `subscribe.go` 是读取面，持有 `alive`/`members` 缓存与 watcher 生命周期

**两个 KV bucket**：

| bucket 名 | 用途 | TTL | History | Storage |
|---|---|---|---|---|
| `<Prefix>members`（默认 `members`） | 成员配置 | 0（不过期） | 1 | Memory |
| `<Prefix>alive`（默认 `alive`） | 心跳/存活 | `AliveTTL`（默认 10s） | 1 | Memory |

key 都是 `logicAddr.String()`。value 都是 `Node.Marshal()` 后的 JSON 字节。

## 初始化

`init()` 是私有懒初始化，`UpdateMember`/`RemoveMember`/`KeepAlive`/`Subscribe` 都先调用它：

1. `nats.Connect(Servers, nats.ReconnectWait(2s), nats.MaxReconnects(-1))` 建立连接
2. `nc.JetStream()` 获取 JetStreamContext
3. `ensureBucket("members", 0)` 和 `ensureBucket("alive", AliveTTL)` 创建两个 bucket
4. 绑定到 `s.membersKV` / `s.aliveKV`

`ensureBucket(name, ttl)`：先 `CreateKeyValue`，若返回 `ErrStreamNameAlreadyInUse` 则降级为 `KeyValue(name)` 取出已存在 bucket——以运行中的 bucket 为准。

`cleanupConn()`：init 中途失败时关闭连接并置空。

`GetNatsError(err)`：屏蔽 `ErrKeyNotFound` / `ErrBucketNotFound` / `ErrNoKeysFound` 等非错误返回。

## Admin 方法（admin.go）

```go
func (s *Membership) UpdateMember(n membership.Node) error {
    // init -> Marshal -> membersKV.Put(logicAddr, jsonBytes)
}

func (s *Membership) RemoveMember(la addr.LogicAddr) error {
    // init -> membersKV.Delete(la)（幂等，不存在时 GetNatsError 归一化为 nil）
}

func (s *Membership) KeepAlive(la addr.LogicAddr, _ int) error {
    // init -> aliveKV.Put(la, []byte("1"))
    // 第二个 int 参数被忽略：bucket 级 TTL 由 AliveTTL 决定
}
```

## Subscribe 与 Watch（subscribe.go）

**`Subscribe(cb)`**：

1. `sync.Once` 保护幂等：二次调用直接返回已有 close 函数
2. `init()` 建立连接和 bucket
3. 全量同步：先 `getMembers(ctx)` 后 `getAlives(ctx)`，顺序与 etcd/redis 一致
4. 启动 `watch(ctx)` goroutine
5. 返回 `s.close`

**`getMembers(ctx)` / `getAlives(ctx)`**：用 `kv.Keys()` 列举并 diff 出 `Add` / `Remove` / `Update`。`getAlives` 收到 alive key 后，只有该 key 同时在 `members` 中且 `Node.Available == true` 才推 `Update(Available=true)`。

**`watch(ctx)`**：

```go
membersW, _ := s.membersKV.WatchAll(nats.Context(ctx))
aliveW, _   := s.aliveKV.WatchAll(nats.Context(ctx))
membersCh := membersW.Updates()
aliveCh   := aliveW.Updates()

ticker := time.NewTicker(5 * time.Second)
hadError := false

for {
    select {
    case entry, ok := <-membersCh:
        if !ok { return }
        s.handleConfigEntry(entry)
    case entry, ok := <-aliveCh:
        if !ok { return }
        s.handleAliveEntry(entry)
    case <-ticker.C:
        err1 := s.getMembers(ctx)
        err2 := s.getAlives(ctx)
        if err1 != nil || err2 != nil {
            hadError = true
        } else if hadError {
            // 重置 watcher 以确保不丢事件
            membersW.Stop(); aliveW.Stop()
            membersW, _ = s.membersKV.WatchAll(nats.Context(ctx))
            aliveW, _   = s.aliveKV.WatchAll(nats.Context(ctx))
            membersCh = membersW.Updates(); aliveCh = aliveW.Updates()
            hadError = false
        }
    case <-ctx.Done():
        membersW.Stop(); aliveW.Stop()
        return
    }
}
```

**`WatchAll` 语义要点**：NATS KV watcher 启动后会先发送当前快照（每个现存 key 一条 `KeyValuePutOp` entry），然后是 `nil` 标记快照结束，最后是后续增量。因此 `Subscribe` 中的显式全量拉取是"权威"的初始通知来源；watcher 推送的初始快照因为 key 已在 `members` map 中，会被判断为已存在，只触发 `Update` 而不会重复触发 `Add`——符合预期。

**`handleConfigEntry` / `handleAliveEntry`**：mirror etcd 单事件版的逻辑，按 `entry.Operation()` 区分 `KeyValuePutOp` / `KeyValueDeleteOp`，`nil` 标记直接 skip。

**`close()`**：`CompareAndSwap` 保护下，cancel context（触发 watch goroutine 退出并 Stop watcher）+ `nc.Close()`。

## 与现有实现的差异

| 维度 | natsjet | etcd | redis |
|---|---|---|---|
| 保活机制 | KV bucket TTL | per-lease TTL | Lua 脚本 deadline |
| `KeepAlive` 的 `second` 参数 | **被忽略** | 实际生效 | 实际生效 |
| 超时清理 | 自动（TTL） | 自动（lease） | 手动 `CheckTimeout()` |
| 变更通知 | KV Watch | Watch | Pub/Sub + Lua version |
| 文件划分 | 3 文件（仿 redis） | 1 文件 | 3 文件 |

## 测试计划

`start.sh` 启动单节点 nats-server（store_dir 必须通过配置文件指定，命令行不支持）：

```bash
#!/bin/bash
# 在当前目录启动单节点 nats-server，启用 JetStream
DIR="$(cd "$(dirname "$0")" && pwd)"
JSDIR="${DIR}/js"
mkdir -p "${JSDIR}"

cat > "${DIR}/nats-server.conf" <<EOF
jetstream {
  store_dir: "${JSDIR}"
}
EOF

exec nats-server \
  -c "${DIR}/nats-server.conf" \
  -a 127.0.0.1 \
  -p 4222
```

`client_test.go` 直接复刻 redis/etcd 的测试结构（`memberCollector` / `waitUntil` / `makeAddr` / `makeLogicAddr`）。

`setupTestDB(t)` 接受 `aliveTTL time.Duration` 参数，由测试用例按需指定：

```go
func setupTestDB(t *testing.T, aliveTTL time.Duration) *Membership {
    t.Helper()
    nc, err := nats.Connect("nats://127.0.0.1:4222")
    if err != nil { t.Fatal(err) }
    js, _ := nc.JetStream()
    // 清空旧 bucket（无视错误，可能不存在）
    _ = js.DeleteKeyValue("members")
    _ = js.DeleteKeyValue("alive")
    t.Cleanup(func() {
        _ = js.DeleteKeyValue("members")
        _ = js.DeleteKeyValue("alive")
        nc.Close()
    })
    return &Membership{
        Servers:  "nats://127.0.0.1:4222",
        AliveTTL: aliveTTL,
    }
}
```

测试用例需要短 TTL 的（如 `TestAdmin_AliveTTL` / `TestSubscribe_TimeoutChange`）传 `1*time.Second`；不需要的传默认值即可。

| 测试 | 验证点 |
|---|---|
| `TestGetNatsError` | 错误归一化 |
| `TestAdmin_UpdateMember` | Put 成功 |
| `TestAdmin_UpdateMemberTwice` | 同 key 二次 Put 是更新 |
| `TestAdmin_MultipleMembers` | 多 key 写入 |
| `TestAdmin_RemoveMember` | Delete 后再读得到 ErrKeyNotFound |
| `TestAdmin_RemoveMemberNonExistent` | 删不存在的 key 不报错 |
| `TestAdmin_KeepAlive` | Put 进 alive bucket 能 Get 到 |
| `TestAdmin_KeepAlive_MultipleNodes` | 多 key alive 写入 |
| `TestAdmin_AliveTTL` | 写入 alive 后 sleep > TTL，Get 返回 ErrKeyNotFound |
| `TestSubscribe_EmptyDB` | 空 bucket 订阅无回调 |
| `TestSubscribe_InitialMembers` | 已有成员 → 初始 Add |
| `TestSubscribe_InitialMembersWithAlive` | members + alive 已存在 → 1 Add + 1 Update(Available=true) |
| `TestSubscribe_NewMemberNotification` | 订阅后 Put → 收到 Add |
| `TestSubscribe_UpdateNotification` | 已有 key 再 Put → 收到 Update |
| `TestSubscribe_RemoveNotification` | 已有 key Delete → 收到 Remove |
| `TestSubscribe_AddAfterRemove` | 删除后 Put 同 key → Remove + Add |
| `TestSubscribe_AliveChange` | alive Put → Update(Available=true) |
| `TestSubscribe_TimeoutChange` | alive 过期 → Update(Available=false) |

`setupTestDB(t)`：连接 `nats://127.0.0.1:4222`，`DeleteKeyValue` + `CreateKeyValue` 重建两个 bucket 保证干净状态。

## 依赖

引入 `github.com/nats-io/nats.go` 最新稳定版：

```bash
go get github.com/nats-io/nats.go@latest
go mod tidy
go build ./...
```

## 文档更新

在 `doc/membership.md` 中追加 `## natsjet 实现` 章节，包含数据结构、工作流程、KeepAlive 机制、与 etcd/redis 的差异表（同上表）。
