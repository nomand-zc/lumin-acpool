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
    └── interface.go        # 新增第 7 个子接口：MetricStore
    └── memory/
        └── metric.go       # MetricStore Memory 实现
    └── redis/
        └── metric.go       # MetricStore Redis 实现
    └── mysql/
        └── metric.go       # MetricStore MySQL 实现
    └── sqlite/
        └── metric.go       # MetricStore SQLite 实现
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

### 3.1 指标类型定义（`observer/metric.go`）

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

// QuotaMetrics 额度用量指标（按 SourceType 维度聚合）
// 业务层根据这些原始指标自定义阈值判断是否告警或补额度
type QuotaMetrics struct {
    // TotalQuota 该维度的总额度（各账号 UsageRule.Total 之和）
    TotalQuota float64
    // EstimatedUsed 当前估算已用量（各账号 EstimatedUsed 之和）
    EstimatedUsed float64
    // EstimatedRemain 当前估算剩余量（各账号 EstimatedRemain 之和）
    EstimatedRemain float64
    // RemainRatio 剩余比例（0.0 ~ 1.0），= EstimatedRemain / TotalQuota；TotalQuota 为 0 时值为 1.0
    RemainRatio float64
}

// PoolHealth 综合健康等级（基于 EffectiveAvailable 计算，业务层可自定义更细粒度）
type PoolHealth int

const (
    PoolHealthy  PoolHealth = iota // EffectiveAvailable > 0
    PoolWarning                    // EffectiveAvailable == 0 但 TotalAccounts > 0（可扩展）
    PoolCritical                   // TotalAccounts == 0 或 EffectiveAvailable == 0
)
```

**说明**：
- `QuotaMetrics` 存储 Token/Request 两个维度的原始用量指标，由业务层自行判断是否告警
- `RemainRatio` 方便业务层快速判断，无需再次计算

### 3.2 指标快照结构（`observer/snapshot.go`）

```go
// GlobalProviderType / GlobalProviderName 是全局指标在 MetricStore 中的特殊标识。
// Metrics.ProviderType == GlobalProviderType 且 ProviderName == GlobalProviderName 时代表全局维度。
const (
    GlobalProviderType = "__global__"
    GlobalProviderName = "__global__"
)

// Metrics 账号池指标快照，统一表示 Provider 维度和全局维度。
//
// 全局维度：ProviderType = GlobalProviderType，ProviderName = GlobalProviderName
// Provider 维度：ProviderType/ProviderName 为实际的 Provider 标识
//
// ProviderType + ProviderName 是 MetricStore 的唯一存储键。
type Metrics struct {
    // 标识
    ProviderType string // Provider 类型；全局时为 GlobalProviderType
    ProviderName string // Provider 名称；全局时为 GlobalProviderName

    // 账号统计
    TotalAccounts      int        // 账号总数
    EffectiveAvailable int        // 真实可用账号数（三重过滤后）
    StatusDist         StatusDist // 状态分布

    // 并发情况
    OccupancyTotal int64 // 当前总并发占用数

    // 质量指标
    SuccessRate  float64      // 近期成功率（0.0 - 1.0）；无历史调用时为 1.0
    TokenQuota   QuotaMetrics // Token/积分维度的额度汇总
    RequestQuota QuotaMetrics // 请求次数维度的额度汇总
    Health       PoolHealth   // 综合健康状态

    // 元数据
    GeneratedAt time.Time // 快照生成时间
}

// IsGlobal 判断当前 Metrics 是否为全局维度。
func (m *Metrics) IsGlobal() bool {
    return m.ProviderType == GlobalProviderType && m.ProviderName == GlobalProviderName
}
```

> **"真实可用"定义**（与 Pick 流程对齐）：
> 1. `account.Status == StatusAvailable`
> 2. 所有 `UsageRule` 的 `estimatedUsed / rule.Total < safetyRatio`（未触发限流）
> 3. 当前并发占用 < `account.MaxConcurrency`（通过 `OccupancyStore.GetOccupancies` 批量查询，`MaxConcurrency == 0` 时视为无上限）

---

## 四、存储接口（`storage.MetricStore`）

```go
// MetricStore 是第 7 个存储子接口，负责持久化账号池指标快照。
//
// 存储键：ProviderType + ProviderName 的组合，全局维度使用
// GlobalProviderType / GlobalProviderName 常量标识。
// 存储语义：upsert（覆盖写），同一 ProviderType+ProviderName 只保留最新一条记录。
// 生命周期：由 PoolObserver 后台周期性写入，查询侧频繁读取。
type MetricStore interface {
    // SaveMetrics 保存指标快照（upsert）。
    // 相同的 ProviderType + ProviderName 将覆盖之前的记录。
    SaveMetrics(ctx context.Context, metrics *observer.Metrics) error

    // GetMetrics 读取指定 ProviderType + ProviderName 的指标快照。
    // 不存在时返回 ErrNotFound。
    GetMetrics(ctx context.Context, providerType, providerName string) (*observer.Metrics, error)

    // ListMetrics 读取所有已存储的指标快照列表（含全局维度和所有 Provider 维度）。
    // 无快照时返回空切片。
    ListMetrics(ctx context.Context) ([]*observer.Metrics, error)

    // ClearMetrics 清空所有快照数据（仅用于测试）。
    ClearMetrics(ctx context.Context) error
}
```

各后端存储实现要点：

| 后端   | SaveMetrics 实现方式 | GetMetrics 实现方式 |
|--------|--------------------|--------------------|
| Memory | `sync.RWMutex` + `map[string]*Metrics`，key 为 `providerType:providerName` | map 直接读取 |
| Redis  | `SET lumin:metrics:{type}:{name}` JSON 序列化，TTL 可配置 | `GET lumin:metrics:{type}:{name}` |
| MySQL  | `INSERT ... ON DUPLICATE KEY UPDATE`，联合唯一索引 `(provider_type, provider_name)` | `SELECT WHERE provider_type=? AND provider_name=?` |
| SQLite | `INSERT OR REPLACE`，联合唯一索引同 MySQL | 同 MySQL |

聚合接口 `storage.Storage` 同步扩展：

```go
type Storage interface {
    AccountStorage
    ProviderStorage
    StatsStore
    UsageStore
    OccupancyStore
    AffinityStore
    MetricStore  // 新增第 7 个
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

    // GetMetrics 获取指定 ProviderType + ProviderName 的指标快照。
    // 传入 GlobalProviderType / GlobalProviderName 获取全局维度快照。
    // 从 MetricStore 直接读取，无阻塞。
    GetMetrics(ctx context.Context, providerType, providerName string) (*Metrics, error)

    // ListMetrics 获取所有已存储的指标快照（含全局维度和所有 Provider 维度）。
    // 从 MetricStore 直接读取，无阻塞。
    ListMetrics(ctx context.Context) ([]*Metrics, error)

    // RefreshNow 立即触发一次全量指标计算与写入（同步）。
    // 用于测试或紧急刷新场景。
    RefreshNow(ctx context.Context) error
}
```

**调用惯例**：

```go
// 获取全局指标
global, _ := obs.GetMetrics(ctx, observer.GlobalProviderType, observer.GlobalProviderName)

// 获取指定 Provider 指标
pm, _ := obs.GetMetrics(ctx, "kiro", "kiro-prod")

// 获取所有指标（含全局 + 所有 Provider）
all, _ := obs.ListMetrics(ctx)
```

### 5.3 Options 与 With* 配置（`observer/option.go`）

```go
type Options struct {
    // 后端存储（必填）
    MetricStore     storage.MetricStore
    AccountStorage  storage.AccountStorage
    ProviderStorage storage.ProviderStorage
    StatsStore      storage.StatsStore
    UsageStore      storage.UsageStore
    OccupancyStore  storage.OccupancyStore

    // 后台任务配置
    RefreshInterval time.Duration // 默认 30s

    // 指标计算参数
    SafetyRatio float64 // 限流安全阈值，用于判断账号是否触发限流，默认 0.95

    // 集群选举（可选）
    LeaderElector    LeaderElector
    LeaderElectorKey string
}

// 对外暴露的 With* 函数：
func WithMetricStore(s storage.MetricStore) Option
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
② 从 UsageStore.GetCurrentUsages() 获取各账号用量追踪数据，
   遍历所有规则：estimatedUsed / rule.Total >= safetyRatio 则该账号排除
③ 从 OccupancyStore.GetOccupancies() 批量获取并发占用数，
   当前占用 >= account.MaxConcurrency 则排除
   （MaxConcurrency == 0 时视为无上限，不过滤）
```

### 6.2 QuotaMetrics 聚合计算

```
TokenQuota：
  遍历所有账号，从 UsageStore.GetCurrentUsages() 取 SourceType == Token 的规则
  TotalQuota      = sum(rule.Total)
  EstimatedUsed   = sum(EstimatedUsed())
  EstimatedRemain = sum(EstimatedRemain())
  RemainRatio     = EstimatedRemain / TotalQuota（TotalQuota == 0 时为 1.0）

RequestQuota：同上，取 SourceType == Request 的规则

无规则的账号贡献 0，不影响比例。
```

### 6.3 PoolHealth 计算规则

```
TotalAccounts == 0              → PoolCritical
EffectiveAvailable == 0         → PoolCritical
EffectiveAvailable > 0          → PoolHealthy
```

> 业务层如需 Warning 等级，读取 `EffectiveAvailable / TotalAccounts` 后自行与自定义阈值对比。

### 6.4 SuccessRate 计算规则

```
从 StatsStore.GetStats() 汇总 Provider（或全局）下所有账号的 TotalCalls / SuccessCalls
SuccessRate = sum(SuccessCalls) / sum(TotalCalls)
TotalCalls == 0 时返回 1.0（无历史调用，默认健康）
```

### 6.5 RefreshNow 执行顺序

```
1. ProviderStorage.SearchProviders(nil)       // 枚举所有 Provider
2. 对每个 Provider 执行 collectMetrics(providerType, providerName)
3. MetricStore.SaveMetrics(providerMetrics)   // 逐个写入
4. 全量账号 collectMetrics(GlobalProviderType, GlobalProviderName)
5. MetricStore.SaveMetrics(globalMetrics)
6. 单个 Provider 计算失败时 continue，不中断整体流程
```

---

## 七、多实例一致性策略

| 场景 | 策略 |
|------|------|
| **单机部署** | 不注入 `LeaderElector`，默认 `isLeader()` 返回 true，单实例执行计算 |
| **集群 + Memory 后端** | 不适用（Memory 不跨进程共享），此组合下每实例独立计算 |
| **集群 + Redis/MySQL 后端** | 注入基于分布式锁的 `LeaderElector`，仅 leader 实例计算并写入，其他实例直接读 `MetricStore` |
| **leader 宕机** | `LeaderElector.IsLeader()` 约定：分布式锁不可用时返回 `true`，保证至少有一个实例继续执行 |
| **并发写入** | `MetricStore` 采用 upsert 语义，last-write-wins，无版本冲突问题 |

---

## 八、包结构与文件规划

```
observer/
├── observer.go          // PoolObserver 接口定义 + NewPoolObserver 构造函数
├── default_observer.go  // defaultPoolObserver 实现
├── snapshot.go          // Metrics / GlobalProviderType / GlobalProviderName 常量
├── metric.go            // StatusDist / QuotaMetrics / PoolHealth
├── collector.go         // collector（内部类型，不导出）
├── option.go            // Options + With* Functional Options
├── leader.go            // LeaderElector 接口
└── observer_test.go     // 单元测试（Table-Driven，覆盖率 ≥ 90%）

storage/
├── interface.go         // 新增 MetricStore 第 7 个子接口 + Storage 聚合接口扩展
├── memory/
│   └── metric.go        // MetricStore Memory 实现
├── redis/
│   └── metric.go        // MetricStore Redis 实现（JSON 序列化，SET/GET/SCAN）
├── mysql/
│   ├── metric.go        // MetricStore MySQL 实现（upsert，联合唯一索引）
│   └── migrations/
│       └── xxx_create_metrics_table.sql
└── sqlite/
    └── metric.go        // MetricStore SQLite 实现（INSERT OR REPLACE）
```

---

## 九、验收标准

### AC-1：EffectiveAvailable 准确性

**目标**：快照中的 `EffectiveAvailable` 必须与 Pick 流程中可被选出的账号数量一致。

| # | 验收条件 | 验证方式 |
|---|----------|---------|
| AC-1.1 | `Status != Available` 的账号不计入 `EffectiveAvailable` | 单元测试：构造含 CoolingDown/CircuitOpen 账号的列表，验证过滤结果 |
| AC-1.2 | 用量超过 safetyRatio 阈值的账号不计入（estimatedUsed / rule.Total >= safetyRatio） | 单元测试：模拟 UsageStore 返回已超限额的 TrackedUsage，验证排除逻辑 |
| AC-1.3 | 并发占用 >= MaxConcurrency 的账号不计入（MaxConcurrency > 0 时） | 单元测试：模拟 OccupancyStore 返回满占用，验证排除逻辑 |
| AC-1.4 | MaxConcurrency == 0（无上限）的账号不受并发过滤影响 | 单元测试：MaxConcurrency=0 场景下，有占用记录的账号仍计入 |
| AC-1.5 | `EffectiveAvailable + 非可用账号数 == TotalAccounts`（StatusDist 总和一致） | 单元测试：混合状态账号列表，验证 StatusDist 各字段之和等于 TotalAccounts |

---

### AC-2：PoolHealth 与 QuotaMetrics 计算准确性

| # | 验收条件 | 验证方式 |
|---|----------|---------|
| AC-2.1 | `TotalAccounts == 0` 时 `Health == PoolCritical` | 单元测试：空 Provider 场景 |
| AC-2.2 | `EffectiveAvailable == 0` 且 `TotalAccounts > 0` 时 `Health == PoolCritical` | 单元测试：所有账号均不可用（均在冷却中）场景 |
| AC-2.3 | `EffectiveAvailable > 0` 时 `Health == PoolHealthy` | 单元测试：至少一个账号可用场景 |
| AC-2.4 | `TokenQuota.TotalQuota` = 各账号 Token 规则的 `rule.Total` 之和 | 单元测试：3 个账号各有不同 Total，验证汇总值 |
| AC-2.5 | `TokenQuota.EstimatedRemain` = 各账号 `EstimatedRemain()` 之和，`RemainRatio = EstimatedRemain / TotalQuota` | 单元测试：构造已用一半额度的账号，验证 RemainRatio ≈ 0.5 |
| AC-2.6 | `RequestQuota` 独立按 `SourceType == Request` 规则汇总，与 `TokenQuota` 互不干扰 | 单元测试：账号同时有 Token 和 Request 规则，验证两个维度分别正确 |
| AC-2.7 | 账号无任何用量规则时，`TokenQuota` 和 `RequestQuota` 的 `TotalQuota == 0`，`RemainRatio == 1.0` | 单元测试：无规则账号场景 |

---

### AC-3：MetricStore 各后端实现正确性

| # | 验收条件 | 验证方式 |
|---|----------|---------|
| AC-3.1 | `SaveMetrics` 后 `GetMetrics` 能读到相同数据（含 QuotaMetrics 数值字段） | 各后端单元测试（Memory 直接测，Redis 用 miniredis，MySQL 用 sqlmock，SQLite 用内存库） |
| AC-3.2 | 相同 `ProviderType + ProviderName` 第二次写入覆盖第一次（upsert 语义） | 单元测试：写入两次，第二次 GeneratedAt 更新，`GetMetrics` 返回最新 |
| AC-3.3 | `ListMetrics` 返回所有已保存的快照（含全局维度 + 所有 Provider 维度） | 单元测试：写入全局 + 3 个 Provider，验证返回列表长度为 4 |
| AC-3.4 | 全局维度通过 `GlobalProviderType / GlobalProviderName` 读写，与 Provider 维度隔离不冲突 | 单元测试：全局写入后读取 Provider 维度，验证不互相覆盖 |
| AC-3.5 | `GetMetrics` 对不存在的 key 返回 `ErrNotFound` | 单元测试：空 store 直接 Get |
| AC-3.6 | `ClearMetrics` 后 `ListMetrics` 返回空切片，`GetMetrics` 返回 `ErrNotFound` | 单元测试：写入后清空，再验证 |

---

### AC-4：多实例一致性（MetricStore 共享后端）

| # | 验收条件 | 验证方式 |
|---|----------|---------|
| AC-4.1 | 两个 `defaultPoolObserver` 实例共享同一 Redis/MySQL `MetricStore`，实例 A 写入后实例 B 读取到相同数据 | 集成测试（Redis 用 miniredis，MySQL 用 sqlmock） |
| AC-4.2 | 注入非 leader 的 `LeaderElector` 时，`Start()` 后后台 goroutine 不执行写入 | 单元测试：mock `LeaderElector.IsLeader()` 返回 false，验证 `MetricStore.SaveMetrics` 未被调用 |
| AC-4.3 | 注入 leader `LeaderElector` 时，后台 goroutine 正常触发写入 | 单元测试：mock `IsLeader()` 返回 true，等待一次 tick，验证 `MetricStore.SaveMetrics` 被调用 |
| AC-4.4 | 未注入 `LeaderElector` 时（nil），`isLeader()` 默认返回 true | 单元测试：验证 nil leaderElector 时执行写入 |

---

### AC-5：PoolObserver 生命周期

| # | 验收条件 | 验证方式 |
|---|----------|---------|
| AC-5.1 | `Start()` 后立即调用 `Stop()` 不会 panic，goroutine 正常退出 | 单元测试：`Start(); Stop()` 快速序列 |
| AC-5.2 | `Start()` 重复调用返回错误，不启动多个后台 goroutine | 单元测试：两次 `Start()` 验证第二次返回 error |
| AC-5.3 | `RefreshNow()` 在 `MetricStore` 写入失败时返回错误，不 panic | 单元测试：mock `MetricStore.SaveMetrics`（全局维度）返回 error |
| AC-5.4 | 单个 Provider 指标计算失败时，`RefreshNow` 继续处理其他 Provider，不中断整体流程 | 单元测试：mock `AccountStorage.SearchAccounts` 对某 Provider 返回 error，验证其他 Provider 正常写入 |
| AC-5.5 | `NewPoolObserver` 缺少必填 storage 时返回 error，不返回 nil observer | 单元测试：逐一缺省各必填 storage 参数 |

---

### AC-6：性能要求

| # | 验收条件 | 验证方式 |
|---|----------|---------|
| AC-6.1 | `GetMetrics` / `ListMetrics` 的 P99 延迟 < 10ms（Memory 后端） | 基准测试：`BenchmarkGetMetrics`、`BenchmarkListMetrics`，验证 ns/op |
| AC-6.2 | `RefreshNow` 对 100 个账号（10 个 Provider，每 Provider 10 账号）的全量计算耗时 < 500ms（Memory 后端） | 基准测试：`BenchmarkRefreshNow_100Accounts` |
| AC-6.3 | `RefreshNow` 不在 Pick 流程（100ms 约束）的关键路径上执行（后台独立 goroutine） | 代码审查：确认 `Balancer.Pick()` 中无对 `PoolObserver` 的调用 |

---

### AC-7：代码质量要求

| # | 验收条件 | 验证方式 |
|---|----------|---------|
| AC-7.1 | `observer/` 包单元测试覆盖率 ≥ 90% | `go test -cover ./observer/...` |
| AC-7.2 | `storage/memory/metric.go`、`storage/redis/metric.go` 等各后端实现覆盖率 ≥ 90% | `go test -cover ./storage/...` |
| AC-7.3 | 并发相关测试（`Start/Stop`、`SaveMetrics` 并发写）通过 `-race` 检测 | `go test -race ./observer/... ./storage/...` |
| AC-7.4 | 所有测试使用 Table-Driven 模式，禁止 testify，使用原生 `testing` 包 | 代码审查 |
| AC-7.5 | `collector`、`defaultPoolObserver` 等实现类型不导出 | 代码审查：确认无 `export` 的实现类型 |

---

## 十、不在本次范围内

以下内容明确排除在本次实现之外：

- **补账号/补 Provider 的判断逻辑**：上游业务层根据 `EffectiveAvailable`、`Health` 等指标自定义阈值判断，本模块只提供原始指标
- **指标历史时序存储**：本模块只保留最新快照，历史趋势由上游监控系统（Prometheus/Grafana）负责
- **HTTP / Prometheus exporter**：本模块是 SDK 库，不提供网络服务端点，由上游 lumin-proxy / lumin-admin 自行暴露
- **告警触发**：不在本模块处理，由业务层订阅快照后决定
- **MetricStore 的 LeaderElector 实现**：业务层或上游项目自行提供基于 Redis/MySQL 的实现

---

## 十一、后续可扩展方向（YAGNI，当前不实现）

- `ProviderMetrics` 增加 `P99Latency` 等延迟分布字段（需要 StatsStore 扩展）
- `GlobalMetrics` 增加模型维度的分组统计
- `SnapshotStore` 增加历史快照版本保留，支持时序查询
