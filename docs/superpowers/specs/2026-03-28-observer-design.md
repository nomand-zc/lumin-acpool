# 账号池可观测性模块设计文档（observer）

**日期**：2026-03-28
**状态**：待实现
**关联需求**：ARCHITECTURE.md — 设计与实现关键约束 · 可监控要素

---

## 一、背景与目标

### 1.1 现状

`lumin-acpool` 目前满足了高稳定性与高性能的核心调度需求，但在可监控维度存在以下空缺：

- 无法实时获取账号池中各 Provider 的可用账号数量
- 无法得知每个账号的真实可用状态（仅状态为 Available 不够，还需排除限流、熔断、并发已满等情况）
- 缺少"是否需要补充账号"的判断依据，资源枯竭时只能被动感知

### 1.2 目标

1. **账号池可用资源可监控**：上游（lumin-proxy / lumin-admin）可查询全局和 Provider 维度的指标快照，快速判断是否需要补充账号或新增 Provider
2. **账号状态实时性**：快照数据通过 `SnapshotStore` 持久化，多实例部署时各实例读到一致数据，快照刷新间隔默认 30s（可配置）
3. **零侵入调度流程**：`PoolObserver` 完全独立于 `Balancer.Pick()` 流程，不影响选号性能

### 1.3 设计约束

- 遵循 CONVENTIONS.md 中的 SOLID、GRASP、YAGNI 原则
- 配置使用 `With*` Functional Options
- 接口命名遵循行为命名，无 `I` 前缀
- 实现类型使用 `default` 前缀私有类型
- 不导出非必要类型

---

## 二、架构概览

### 2.1 模块位置

```
lumin-acpool/
├── observer/               # 新增：可观测性模块
│   ├── observer.go         # PoolObserver 接口 + NewPoolObserver 构造
│   ├── snapshot.go         # BaseMetrics / ProviderMetrics / GlobalMetrics 数据结构
│   ├── metric.go           # StatusDist / QuotaHealth / PoolHealth 枚举与类型
│   ├── collector.go        # 从 storage 层聚合计算指标（内部，不导出）
│   ├── option.go           # With* Functional Options + Options 结构
│   ├── leader.go           # LeaderElector 接口定义
│   └── observer_test.go    # 单元测试
│
└── storage/
    └── interface.go        # 新增第 7 个子接口：SnapshotStore
    └── memory/
        └── snapshot.go     # SnapshotStore Memory 实现
    └── redis/
        └── snapshot.go     # SnapshotStore Redis 实现
    └── mysql/
        └── snapshot.go     # SnapshotStore MySQL 实现
    └── sqlite/
        └── snapshot.go     # SnapshotStore SQLite 实现
```

### 2.2 数据流

```
[后台 goroutine（仅 leader 执行）]
  │
  ├─ AccountStorage.SearchAccounts()   → 获取全量账号
  ├─ UsageStore.GetCurrentUsages()     → 判断限流
  ├─ OccupancyStore.GetOccupancies()   → 判断并发占用
  ├─ StatsStore.GetStats()             → 计算成功率
  ├─ ProviderStorage.SearchProviders() → 枚举 Provider
  │
  └─ [计算 EffectiveAvailable、StatusDist、SuccessRate、QuotaHealth、Health]
       │
       └─ SnapshotStore.SaveProviderMetrics() / SaveGlobalMetrics()

[上游 admin/proxy 查询侧]
  │
  └─ PoolObserver.GetAllProviderMetrics() / GetGlobalMetrics()
       │
       └─ SnapshotStore.GetAllProviderMetrics() / GetGlobalMetrics()
```

---

## 三、数据结构定义

### 3.1 指标枚举类型（`observer/metric.go`）

```go
// StatusDist 账号状态分布
type StatusDist struct {
    Available   int
    CoolingDown int
    CircuitOpen int
    Expired     int
    Invalidated int
    Banned      int
    Disabled    int
}

// QuotaHealth 额度健康度
type QuotaHealth int

const (
    QuotaHealthy   QuotaHealth = iota // 额度充足
    QuotaLow                          // 部分账号额度不足（< 20% 的账号剩余比例超过一半）
    QuotaExhausted                    // 有账号额度已耗尽
)

// PoolHealth 综合健康等级
// 注意：具体阈值由业务层根据 EffectiveAvailable / TotalAccounts 自行判断，
// 此字段由 collector 按以下规则计算：
//   - TotalAccounts == 0                         → Critical
//   - EffectiveAvailable == 0 且 TotalAccounts > 0 → Critical
//   - EffectiveAvailable > 0                     → Healthy
// 业务层如需更细粒度（如 < 30% 为 Warning），在读到快照后自行对比。
type PoolHealth int

const (
    PoolHealthy  PoolHealth = iota
    PoolWarning
    PoolCritical
)
```

### 3.2 指标快照结构（`observer/snapshot.go`）

```go
// BaseMetrics 是 ProviderMetrics 和 GlobalMetrics 的公共字段集，保证两者完全对齐。
type BaseMetrics struct {
    TotalAccounts      int         // 账号总数
    EffectiveAvailable int         // 真实可用账号数（三重过滤后）
    StatusDist         StatusDist  // 状态分布
    OccupancyTotal     int64       // 当前总并发占用数
    SuccessRate        float64     // 近期成功率（0.0 - 1.0）
    QuotaHealth        QuotaHealth // 额度健康度
    Health             PoolHealth  // 综合健康状态
    GeneratedAt        time.Time   // 快照生成时间
}

// ProviderMetrics 单个 Provider 维度的指标快照。
// ProviderType + ProviderName 是存储层的唯一索引（拆分存储，便于 SQL/非SQL 检索）。
type ProviderMetrics struct {
    BaseMetrics
    ProviderType string
    ProviderName string
}

// GlobalMetrics 全局维度的指标快照，字段与 ProviderMetrics 完全对齐（都是 BaseMetrics），
// 额外包含 SuggestNewProvider 字段供业务层参考（不存储，由业务层根据 Provider 列表状态判断）。
// 注意：SuggestNewProvider 不由 collector 写入，始终为零值，
//       业务层在读到快照后根据 []ProviderMetrics 中 Health == Critical 的数量自行判断。
type GlobalMetrics struct {
    BaseMetrics
}
```

> **"真实可用"定义**（与 Pick 流程对齐）：
> 1. `account.Status == StatusAvailable`
> 2. `UsageTracker.IsQuotaAvailable() == true`（未触发限流）
> 3. 当前并发占用 < OccupancyController 上限（通过 `OccupancyStore.GetOccupancies` 批量查询）

---

## 四、存储接口（`storage.SnapshotStore`）

```go
// SnapshotStore 是第 7 个存储子接口，负责持久化账号池指标快照。
//
// 存储语义：upsert（覆盖写），同一 ProviderType+ProviderName 只保留最新一条记录；
// 全局指标只保留一条记录。
// 生命周期：由 PoolObserver 后台周期性写入，查询侧频繁读取。
type SnapshotStore interface {
    // SaveProviderMetrics 保存单个 Provider 的指标快照（upsert）。
    SaveProviderMetrics(ctx context.Context, metrics *observer.ProviderMetrics) error

    // GetProviderMetrics 读取单个 Provider 的指标快照。
    // 不存在时返回 ErrNotFound。
    GetProviderMetrics(ctx context.Context, providerType, providerName string) (*observer.ProviderMetrics, error)

    // GetAllProviderMetrics 读取所有 Provider 的指标快照列表。
    // 无快照时返回空切片。
    GetAllProviderMetrics(ctx context.Context) ([]*observer.ProviderMetrics, error)

    // SaveGlobalMetrics 保存全局指标快照（upsert，只保留一条）。
    SaveGlobalMetrics(ctx context.Context, metrics *observer.GlobalMetrics) error

    // GetGlobalMetrics 读取全局指标快照。
    // 不存在时返回 ErrNotFound。
    GetGlobalMetrics(ctx context.Context) (*observer.GlobalMetrics, error)

    // ClearMetrics 清空所有快照数据（仅用于测试）。
    ClearMetrics(ctx context.Context) error
}
```

各后端存储实现要点：

| 后端   | SaveProviderMetrics 实现方式 | SaveGlobalMetrics 实现方式 |
|--------|----------------------------|-----------------------------|
| Memory | `sync.RWMutex` + map，key 为 `providerType:providerName` | `sync.RWMutex` + 单字段 |
| Redis  | `HSET lumin:snapshot:provider:{type}:{name} ...` JSON 序列化 | `SET lumin:snapshot:global` JSON 序列化 |
| MySQL  | `INSERT ... ON DUPLICATE KEY UPDATE`，联合唯一索引 `(provider_type, provider_name)` | 单行 upsert，固定 key |
| SQLite | 同 MySQL，使用 `INSERT OR REPLACE` | 同 MySQL |

聚合接口 `storage.Storage` 同步扩展：

```go
type Storage interface {
    AccountStorage
    ProviderStorage
    StatsStore
    UsageStore
    OccupancyStore
    AffinityStore
    SnapshotStore  // 新增第 7 个
}
```

---

## 五、`PoolObserver` 接口与选项

### 5.1 LeaderElector（`observer/leader.go`）

设计与 `health.LeaderElector` 完全对齐：

```go
// LeaderElector 用于集群部署时选举 leader，确保同一时刻只有一个实例执行指标计算与写入。
// 未注入时，当前实例默认为 leader（兼容单机部署）。
type LeaderElector interface {
    IsLeader(ctx context.Context, key string) bool
}
```

### 5.2 PoolObserver 接口（`observer/observer.go`）

```go
type PoolObserver interface {
    // Start 启动后台指标刷新 goroutine。
    // 集群部署时仅 leader 实例执行计算与写入，非 leader 实例可直接调用 Get* 读取。
    Start(ctx context.Context) error

    // Stop 停止后台刷新并等待 goroutine 退出。
    Stop()

    // GetGlobalMetrics 从 SnapshotStore 读取最新全局指标快照。
    GetGlobalMetrics(ctx context.Context) (*GlobalMetrics, error)

    // GetProviderMetrics 从 SnapshotStore 读取指定 Provider 的指标快照。
    GetProviderMetrics(ctx context.Context, providerType, providerName string) (*ProviderMetrics, error)

    // GetAllProviderMetrics 从 SnapshotStore 读取所有 Provider 的指标快照。
    GetAllProviderMetrics(ctx context.Context) ([]*ProviderMetrics, error)

    // RefreshNow 立即触发一次全量指标计算与写入（同步）。
    // 用于测试或紧急刷新场景。
    RefreshNow(ctx context.Context) error
}
```

### 5.3 Options 与 With* 配置（`observer/option.go`）

```go
type Options struct {
    // 后端存储（必填）
    SnapshotStore   storage.SnapshotStore
    AccountStorage  storage.AccountStorage
    ProviderStorage storage.ProviderStorage
    StatsStore      storage.StatsStore
    UsageStore      storage.UsageStore
    OccupancyStore  storage.OccupancyStore

    // 后台任务配置
    RefreshInterval time.Duration // 默认 30s

    // 指标计算参数
    SafetyRatio float64 // 限流安全阈值，用于判断 IsQuotaAvailable，默认 0.95

    // 集群选举（可选）
    LeaderElector    LeaderElector
    LeaderElectorKey string
}

// 对外暴露的 With* 函数（举例）：
func WithSnapshotStore(s storage.SnapshotStore) Option
func WithAccountStorage(s storage.AccountStorage) Option
func WithProviderStorage(s storage.ProviderStorage) Option
func WithStatsStore(s storage.StatsStore) Option
func WithUsageStore(s storage.UsageStore) Option
func WithOccupancyStore(s storage.OccupancyStore) Option
func WithRefreshInterval(d time.Duration) Option
func WithSafetyRatio(ratio float64) Option
func WithLeaderElector(key string, le LeaderElector) Option
```

---

## 六、核心计算逻辑（`observer/collector.go`）

### 6.1 EffectiveAvailable 三重过滤

```
① 过滤 Status != Available
② 从 UsageStore.GetCurrentUsages() 获取用量，
   计算 estimatedUsed / rule.Total >= safetyRatio 则排除
③ 从 OccupancyStore.GetOccupancies() 批量获取并发数，
   当前占用 >= account.MaxConcurrency 则排除
   （MaxConcurrency 为 0 时视为无上限，不过滤）
```

### 6.2 PoolHealth 计算规则

```
TotalAccounts == 0              → PoolCritical
EffectiveAvailable == 0         → PoolCritical
EffectiveAvailable > 0          → PoolHealthy
```

> 业务层如需 Warning 等级，在读取 `EffectiveAvailable / TotalAccounts` 后自行与配置阈值对比。

### 6.3 QuotaHealth 计算规则

```
任意账号有 IsExhausted() == true → QuotaExhausted
Available 账号中 RemainRatio() < 0.2 的数量 > totalAccounts / 2 → QuotaLow
其他 → QuotaHealthy
```

### 6.4 SuccessRate 计算规则

```
遍历 Provider 下所有账号，从 StatsStore.GetStats() 汇总 TotalCalls 与 SuccessCalls
SuccessRate = sum(SuccessCalls) / sum(TotalCalls)
TotalCalls == 0 时返回 1.0（无历史调用，默认健康）
```

### 6.5 RefreshNow 执行顺序

```
1. ProviderStorage.SearchProviders(nil)         // 枚举所有 Provider
2. 对每个 Provider 执行 collectProviderMetrics()
3. SnapshotStore.SaveProviderMetrics()           // 逐个写入
4. 全量账号 collectGlobalMetrics()
5. SnapshotStore.SaveGlobalMetrics()
6. 单个 Provider 计算失败时 continue，不中断整体流程
```

---

## 七、多实例一致性策略

| 场景 | 策略 |
|------|------|
| **单机部署** | 不注入 `LeaderElector`，默认 `isLeader()` 返回 true，单实例执行计算 |
| **集群 + Memory 后端** | 不适用（Memory 不跨进程共享），此组合下每实例独立计算 |
| **集群 + Redis/MySQL 后端** | 注入基于分布式锁的 `LeaderElector`，仅 leader 实例计算并写入，其他实例直接读 `SnapshotStore` |
| **leader 宕机** | `LeaderElector.IsLeader()` 约定：分布式锁不可用时返回 `true`，保证至少有一个实例继续执行 |
| **并发写入** | `SnapshotStore` 采用 upsert 语义，last-write-wins，无版本冲突问题 |

---

## 八、包结构与文件规划

```
observer/
├── observer.go          // PoolObserver 接口定义 + NewPoolObserver 构造函数
├── default_observer.go  // defaultPoolObserver 实现
├── snapshot.go          // BaseMetrics / ProviderMetrics / GlobalMetrics
├── metric.go            // StatusDist / QuotaHealth / PoolHealth
├── collector.go         // collector（内部类型，不导出）
├── option.go            // Options + With* Functional Options
├── leader.go            // LeaderElector 接口
└── observer_test.go     // 单元测试（Table-Driven，覆盖率 ≥ 90%）

storage/
├── interface.go         // 新增 SnapshotStore 第 7 个子接口 + Storage 聚合接口扩展
├── memory/
│   └── snapshot.go      // Memory 实现
├── redis/
│   └── snapshot.go      // Redis 实现（JSON 序列化，HSET/GET/HGETALL）
├── mysql/
│   └── snapshot.go      // MySQL 实现（upsert，联合唯一索引）
│   └── migrations/
│       └── xxx_create_snapshot_tables.sql
└── sqlite/
    └── snapshot.go      // SQLite 实现（INSERT OR REPLACE）
```

---

## 九、验收标准

### AC-1：EffectiveAvailable 准确性

**目标**：快照中的 `EffectiveAvailable` 必须与 Pick 流程中可被选出的账号数量一致。

| # | 验收条件 | 验证方式 |
|---|----------|---------|
| AC-1.1 | `Status != Available` 的账号不计入 `EffectiveAvailable` | 单元测试：构造含 CoolingDown/CircuitOpen 账号的列表，验证过滤结果 |
| AC-1.2 | `UsageTracker.IsQuotaAvailable() == false` 的账号不计入 | 单元测试：模拟 UsageStore 返回已超限额的 TrackedUsage，验证排除逻辑 |
| AC-1.3 | 并发占用 >= MaxConcurrency 的账号不计入（MaxConcurrency > 0 时） | 单元测试：模拟 OccupancyStore 返回满占用，验证排除逻辑 |
| AC-1.4 | MaxConcurrency == 0（无上限）的账号不受并发过滤影响 | 单元测试：MaxConcurrency=0 场景下，有占用记录的账号仍计入 |
| AC-1.5 | `EffectiveAvailable + 非可用账号数 == TotalAccounts`（StatusDist 总和一致） | 单元测试：混合状态账号列表，验证 StatusDist 各字段之和等于 TotalAccounts |

---

### AC-2：PoolHealth 与 QuotaHealth 判断准确性

| # | 验收条件 | 验证方式 |
|---|----------|---------|
| AC-2.1 | `TotalAccounts == 0` 时 `Health == PoolCritical` | 单元测试：空 Provider 场景 |
| AC-2.2 | `EffectiveAvailable == 0` 且 `TotalAccounts > 0` 时 `Health == PoolCritical` | 单元测试：所有账号均不可用（均在冷却中）场景 |
| AC-2.3 | `EffectiveAvailable > 0` 时 `Health == PoolHealthy` | 单元测试：至少一个账号可用场景 |
| AC-2.4 | 任意账号 `IsExhausted() == true` 时 `QuotaHealth == QuotaExhausted` | 单元测试：一个账号 EstimatedRemain <= 0 |
| AC-2.5 | 超过一半 Available 账号 `RemainRatio() < 0.2` 时 `QuotaHealth == QuotaLow` | 单元测试：3 个账号中 2 个剩余 < 20% |
| AC-2.6 | 额度充足时 `QuotaHealth == QuotaHealthy` | 单元测试：所有账号剩余比例 > 0.5 |

---

### AC-3：SnapshotStore 各后端实现正确性

| # | 验收条件 | 验证方式 |
|---|----------|---------|
| AC-3.1 | `SaveProviderMetrics` 后 `GetProviderMetrics` 能读到相同数据 | 各后端单元测试（Memory 用 mock，Redis 用 miniredis，MySQL 用 sqlmock，SQLite 用内存库） |
| AC-3.2 | 相同 `ProviderType + ProviderName` 第二次写入覆盖第一次（upsert 语义） | 单元测试：写入两次，第二次 GeneratedAt 更新，`GetProviderMetrics` 返回最新 |
| AC-3.3 | `GetAllProviderMetrics` 返回所有已保存的 Provider 快照 | 单元测试：写入 3 个 Provider，验证返回列表长度为 3 |
| AC-3.4 | `SaveGlobalMetrics` 后 `GetGlobalMetrics` 返回一致数据 | 各后端单元测试 |
| AC-3.5 | `GetProviderMetrics` / `GetGlobalMetrics` 对不存在的数据返回 `ErrNotFound` | 单元测试：空 store 直接 Get |
| AC-3.6 | `ClearMetrics` 后所有 Get 方法返回 `ErrNotFound` | 单元测试：写入后清空，再读取验证 |

---

### AC-4：多实例一致性（SnapshotStore 共享后端）

| # | 验收条件 | 验证方式 |
|---|----------|---------|
| AC-4.1 | 两个 `defaultPoolObserver` 实例共享同一 Redis/MySQL `SnapshotStore`，实例 A 写入后实例 B 读取到相同数据 | 集成测试（Redis 用 miniredis，MySQL 用 sqlmock） |
| AC-4.2 | 注入非 leader 的 `LeaderElector` 时，`Start()` 后后台 goroutine 不执行写入（`RefreshNow` 不触发） | 单元测试：mock `LeaderElector.IsLeader()` 返回 false，验证 `SnapshotStore.SaveGlobalMetrics` 未被调用 |
| AC-4.3 | 注入 leader `LeaderElector` 时，后台 goroutine 正常触发写入 | 单元测试：mock `IsLeader()` 返回 true，等待一次 tick，验证 `SaveGlobalMetrics` 被调用 |
| AC-4.4 | 未注入 `LeaderElector` 时（nil），`isLeader()` 默认返回 true | 单元测试：验证 nil leaderElector 时执行写入 |

---

### AC-5：PoolObserver 生命周期

| # | 验收条件 | 验证方式 |
|---|----------|---------|
| AC-5.1 | `Start()` 后立即调用 `Stop()` 不会 panic，goroutine 正常退出 | 单元测试：`Start(); Stop()` 快速序列 |
| AC-5.2 | `Start()` 重复调用返回错误，不启动多个后台 goroutine | 单元测试：两次 `Start()` 验证第二次返回 error |
| AC-5.3 | `RefreshNow()` 在 `SnapshotStore` 写入失败时返回错误，不 panic | 单元测试：mock `SnapshotStore.SaveGlobalMetrics` 返回 error |
| AC-5.4 | 单个 Provider 指标计算失败时，`RefreshNow` 继续处理其他 Provider，不中断整体流程 | 单元测试：mock `AccountStorage.SearchAccounts` 对某 Provider 返回 error，验证其他 Provider 正常写入 |
| AC-5.5 | `NewPoolObserver` 缺少必填 storage 时返回 error，不返回 nil observer | 单元测试：逐一缺省各必填 storage 参数 |

---

### AC-6：性能要求

| # | 验收条件 | 验证方式 |
|---|----------|---------|
| AC-6.1 | `GetGlobalMetrics` / `GetAllProviderMetrics` 的 P99 延迟 < 10ms（Memory 后端） | 基准测试：`BenchmarkGetGlobalMetrics`，验证 ns/op |
| AC-6.2 | `RefreshNow` 对 100 个账号（10 个 Provider，每 Provider 10 账号）的全量计算耗时 < 500ms（Memory 后端） | 基准测试：`BenchmarkRefreshNow_100Accounts` |
| AC-6.3 | `RefreshNow` 不在 Pick 流程（100ms 约束）的关键路径上执行（后台独立 goroutine） | 代码审查：确认 `Balancer.Pick()` 中无对 `PoolObserver` 的调用 |

---

### AC-7：代码质量要求

| # | 验收条件 | 验证方式 |
|---|----------|---------|
| AC-7.1 | `observer/` 包单元测试覆盖率 ≥ 90% | `go test -cover ./observer/...` |
| AC-7.2 | `storage/memory/snapshot.go`、`storage/redis/snapshot.go` 等各后端实现覆盖率 ≥ 90% | `go test -cover ./storage/...` |
| AC-7.3 | 并发相关测试（`Start/Stop`、`SaveProviderMetrics` 并发写）通过 `-race` 检测 | `go test -race ./observer/... ./storage/...` |
| AC-7.4 | 所有测试使用 Table-Driven 模式，禁止 testify，使用原生 `testing` 包 | 代码审查 |
| AC-7.5 | `collector`、`defaultPoolObserver` 等实现类型不导出 | 代码审查：确认无 `export` 的实现类型 |

---

## 十、不在本次范围内

以下内容明确排除在本次实现之外：

- **补账号/补 Provider 的判断逻辑**：上游业务层根据 `EffectiveAvailable`、`Health` 等指标自定义阈值判断，本模块只提供原始指标
- **指标历史时序存储**：本模块只保留最新快照，历史趋势由上游监控系统（Prometheus/Grafana）负责
- **HTTP / Prometheus exporter**：本模块是 SDK 库，不提供网络服务端点，由上游 lumin-proxy / lumin-admin 自行暴露
- **告警触发**：不在本模块处理，由业务层订阅快照后决定
- **SnapshotStore 的 LeaderElector 实现**：业务层或上游项目自行提供基于 Redis/MySQL 的实现

---

## 十一、后续可扩展方向（YAGNI，当前不实现）

- `ProviderMetrics` 增加 `P99Latency` 等延迟分布字段（需要 StatsStore 扩展）
- `GlobalMetrics` 增加模型维度的分组统计
- `SnapshotStore` 增加历史快照版本保留，支持时序查询
