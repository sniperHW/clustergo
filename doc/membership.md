# 成员发现系统

## 接口定义

```go
// membership/membership.go
type Membership interface {
    Subscribe(func(MemberInfo)) (func(), error)  // 订阅成员变更，返回取消函数
    UpdateMember(Node) error                      // 更新/添加节点
    RemoveMember(addr.LogicAddr) error            // 移除节点
    KeepAlive(addr.LogicAddr, int) error          // 保活（参数：逻辑地址、TTL 秒数）
}

type MemberInfo struct {
    Add    []Node    // 新增节点
    Remove []Node    // 移除节点
    Update []Node    // 更新节点
}

type Node struct {
    Addr      addr.Addr
    Export    bool    // 是否暴露给其他 cluster
    Available bool    // 是否可用（配置可用 且 保活有效）
}
```

## 节点可见性规则

一个节点对其他节点可见需满足以下条件之一：
1. `Export == true`（显式导出）
2. 两个节点属于同一 cluster
3. 双方都是 Harbor 节点

在 `nodeCache.onNodeInfoUpdate` 中判断。

## Redis 实现

**文件**：`membership/redis/`

### 数据结构
- 使用 Redis Hash 存储成员列表，每个成员一个 field（逻辑地址字符串）
- 使用 Lua 脚本保证原子性（`membership/redis/script.go`）
- 版本号机制：`memberVersion` / `aliveVersion` 用于增量同步

### 工作流程
1. `Subscribe` 时先全量拉取 members 和 alive 状态
2. 启动 `watch` goroutine 订阅 Redis Pub/Sub channel（`"members"` 和 `"alive"`）
3. 收到通知后通过 Lua 脚本做版本比对，只处理变更部分
4. 每 5 秒 ticker 兜底轮询，防止 Pub/Sub 事件丢失
5. 从错误恢复时重置 version 为 0 强制全量同步

### KeepAlive
通过 Lua 脚本设置带 TTL 的 key，cluster 节点每 2 秒调用一次 `KeepAlive(addr, 5)`。

### 管理操作
- `UpdateMember`：Lua 脚本 insert_update
- `RemoveMember`：Lua 脚本 delete
- `CheckTimeout`：Lua 脚本检查并清理过期节点

## etcd 实现

**文件**：`membership/etcd/`

### 数据结构
- 配置数据：`PrefixConfig` 前缀的 KV（key 为逻辑地址字符串，value 为 JSON 编码的 Node）
- 存活数据：`PrefixAlive` 前缀的 KV（带 lease，TTL 到期自动删除）

### 工作流程
1. `Subscribe` 时先 `Get` 全量 members 和 alives
2. 启动 `watch` goroutine 同时 watch config 和 alive 前缀
3. 处理 `EventTypePut` / `EventTypeDelete` 事件
4. 每 5 秒 ticker 兜底全量同步
5. 错误恢复逻辑与 redis 实现类似

### KeepAlive
使用 etcd lease：`cli.Grant(ctx, seconds)` 创建 lease，然后 `Put` alive key with lease。

## 自定义实现示例

`example/membership/` 提供了一个基于 TCP server/client 的简单实现：
- `memberShipSvr`：中央配置服务器，客户端通过 TCP 连接订阅
- `memberShipCli`：客户端，连接到配置服务器获取全量节点列表并做 diff
- 通过 `AddNode` / `RemNode` / `ModNode` 命令管理节点

这个实现不提供 `UpdateMember`、`RemoveMember`、`KeepAlive`，仅用于本地测试。
