# 逻辑地址系统

## 编码格式

逻辑地址为 `uint32`，格式为 `cluster.type.server`（三段式字符串表示）：

```
bit 位:  [31      18] [17      10] [9       0]
         cluster(14)   type(8)      server(10)
```

| 字段 | 位数 | 范围 | 掩码常量 |
|------|------|------|----------|
| Cluster | 14 | 1–16383 | `ClusterMask = 0xFFFC0000` |
| Type | 8 | 1–254（255 保留为 Harbor） | `TypeMask = 0x0003FC00` |
| Server | 10 | 0–1023 | `ServerMask = 0x000003FF` |

## 特殊 Type

- **HarborType = 255**：Harbor 节点负责跨集群消息转发。必须通过 `MakeHarborAddr` / `MakeHarborLogicAddr` 创建，强制 type=255。

## Addr 结构

```go
type Addr struct {
    logicAddr LogicAddr      // 逻辑地址（uint32）
    netAddr   *net.TCPAddr   // 网络地址（可原子更新）
}
```

- `NetAddr()` 通过 `atomic.LoadPointer` 读取，保证并发安全
- `UpdateNetAddr()` 通过 `atomic.StorePointer` 更新，支持运行时网络地址变更

## 路由选择

### 同集群内发送
直接从 `nodeCache.allnodes` 查找目标逻辑地址对应的 node。

### 跨集群发送（通过 Harbor）
1. 如果当前节点是 Harbor，选择 harbor 集群中与 `to.Cluster()` 匹配的 Harbor 节点
2. 如果当前节点不是 Harbor，选择本 cluster 内的 Harbor 节点
3. Harbor 选择策略：`harbors[int(m) % len(harbors)]`，基于目标地址取模做负载均衡

### 按类型随机选择
`GetAddrByType(tt, n)` 从指定 type 的可用节点中选取：
- `n == 0`：随机选择（`rand.Int31() % len`）
- `n > 0`：取模选择（`n % len`）

## 地址构造函数

```go
// 普通节点（type: 1-254）
addr.MakeLogicAddr("1.2.3")    // cluster=1, type=2, server=3
addr.MakeAddr("1.2.3", "127.0.0.1:8080")

// Harbor 节点（type 强制 255）
addr.MakeHarborLogicAddr("1.255.0")
addr.MakeHarborAddr("1.255.0", "127.0.0.1:9090")
```
