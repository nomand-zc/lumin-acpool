# 核心设计原则

> 四大工程支柱：**高性能** · **高扩展** · **高可读性** · **单一职责**
> 以下每条原则都服务于这四根支柱中的一根或多根。

---

## 高性能

### P1. 热路径零分配

Pick 选号路径是最高频操作。在此路径上：
- 避免不必要的堆分配（返回指针而非值拷贝，复用 slice）
- 使用 `sync.Pool` 缓存临时对象（如 PickResult）
- 随机打散候选列表用 `rand.Shuffle` 原地操作，不创建新 slice

### P2. 原子操作替代互斥锁

StatsStore 和 UsageStore 的 `Incr` 系列方法使用原子操作（CAS / INCR），而非 Mutex 锁。
Redis 后端天然原子；Memory 后端使用 `sync/atomic`。

理由：每次 ReportSuccess/Failure 都触发统计更新，锁竞争会成为吞吐瓶颈。

### P3. 高低频数据分离

高频变更数据（Stats / Usage / Occupancy）从低频数据（Account / Provider）独立存储，
避免全量读写放大。详见原则 2。

### P4. 本地乐观计数减少远端请求

UsageTracker 在本地维护 `LocalUsed` 计数，仅在健康检查时校准远端数据。
避免每次调用都请求远端 API 查询配额，极大降低延迟和外部依赖压力。

---

## 高扩展

### E1. 接口驱动

所有核心组件（Selector / GroupSelector / Storage / OccupancyController / HealthCheck / CircuitBreaker / Cooldown / LeaderElector）
均面向接口编程。新增实现只需满足接口约定，无需修改核心调度逻辑。

### E2. Strategy 模式

Selector 和 GroupSelector 采用策略模式，选号算法可热插拔：
- 内置 5 种 Account 级策略、5 种 Group 级策略
- 新增策略只需实现接口 + 注入，零行核心代码改动

### E3. Functional Options

所有组件构造器使用 `With*` 选项函数，提供合理默认值 + 灵活覆盖：
- 新增配置项只需新增一个 Option 函数，不破坏已有调用方
- 测试中可精确注入 mock 依赖

### E4. 多后端存储

存储层定义统一接口，4 种后端各自实现。新增存储后端（如 PostgreSQL / DynamoDB）
只需实现对应子接口，无需改动任何上层代码。

---

## 高可读性

### R1. 一个文件做一件事

- `account.go` 只定义 Account 结构体
- `status.go` 只定义状态枚举和判断方法
- `provider.go` 只定义 ProviderInfo 和 ProviderKey
- 不在一个文件中混杂多个不相关的类型定义

### R2. 函数短小聚焦

- 公开方法不超过 80 行，复杂流程拆分为清晰的子步骤函数
- 私有辅助函数以动词开头（`filterAvailable`、`acquireFromAccounts`）
- 大段逻辑用空行 + 注释分隔成视觉段落

### R3. 命名即文档

- 类型名、方法名、变量名应自解释，不需要额外注释就能理解意图
- 注释只解释 **为什么（Why）**，不解释 **做什么（What）**
- 每个包提供 `doc.go` 说明包级职责和核心概念

---

## 单一职责

### S1. 包级职责清晰

每个包只承担一个核心职责：

| 包 | 唯一职责 |
|---|--------|
| `balancer/` | 调度编排（Pick / Report） |
| `selector/` | 选号策略（从候选中选一个） |
| `resolver/` | 候选解析（从存储中查询可用列表） |
| `storage/` | 数据持久化接口定义 |
| `health/` | 健康检查编排 |
| `circuitbreaker/` | 熔断判定 |
| `cooldown/` | 冷却管理 |
| `usagetracker/` | 用量追踪 |

### S2. 接口精简不臃肿

- 存储层拆分为 6 个子接口，而非一个巨型 Storage 接口
- 每个子接口方法数控制在 3~6 个
- 消费端只依赖需要的子接口（接口隔离原则）

### S3. 结构体不兼职

- `Account` 只持有数据，不包含行为逻辑
- `defaultBalancer` 只编排流程，不包含选号算法
- 选号算法封装在 Selector，状态判定封装在 CircuitBreaker/Cooldown

---

## 具体实践清单

以下是四大支柱在本仓库中的具体实践：

| # | 实践 | 服务支柱 |
|---|------|----------|
| 1 | Account 是聚合根，位掩码部分更新 | 高性能 · 单一职责 |
| 2 | 高频统计（Stats/Usage）独立于 Account 存储 | 高性能 |
| 3 | 乐观锁（Version）防并发覆盖 | 高性能 |
| 4 | 存储层 6 个细粒度子接口 | 高扩展 · 单一职责 |
| 5 | 单机/集群共享接口，后端可替换 | 高扩展 |
| 6 | Pick 返回 Clone 深拷贝 | 高可读性（调用方无 side effect 心智负担） |
| 7 | Report 是必须的（释放占用 + 驱动状态机） | 单一职责（调用方只管 Report） |
| 8 | Functional Options 构造器 | 高扩展 · 高可读性 |

> 任何新代码都应对照四大支柱自查——高性能 / 高扩展 / 高可读性 / 单一职责。
