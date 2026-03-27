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
2. **账号状态实时性**：快照数据通过 `MetricStore` 持久化，多实例部署时各实例读到一致数据，快照刷新间隔默认 30s（可配置）
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
├── election/               # 新增：公共选举接口
│   └── election.go         # LeaderElector 接口定义
│
├── observer/               # 新增：可观测性模块
│   ├── observer.go         # PoolObserver 接口 + NewPoolObserver 构造
│   ├── collector.go        # 从 storage 层聚合计算指标（内部，不导出）
│   ├── option.go           # With* Functional Options + Options 结构
│   └── observer_test.go    # 单元测试 + 集成测试 + 基准测试
│
├── account/
│   ├── ...                 # 现有文件
│   ├── metrics.go          # 新增：Metrics / StatusDist / QuotaMetrics / GlobalProviderType / GlobalProviderName
│   └── tracked_usage.go    # 扩展：新增 UsedRatio() / IsThrottled() 方法
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
  ├─ UsageStore.GetCurrentUsagesBatch()  → 批量判断限流（一次调用获取所有账号数据）
  ├─ OccupancyStore.GetOccupancies()   → 判断并发占用
  ├─ StatsStore.GetStatsBatch()        → 批量计算成功率
  ├─ ProviderStorage.SearchProviders() → 枚举 Provider
  │
  └─ [计算 EffectiveAvailable、StatusDist、TokenQuota/RequestQuota、SuccessRate]
       │
       └─ MetricStore.SaveMetrics()  // Provider 维度 + 全局维度各写一条

[上游 admin/proxy 查询侧]
  │
  └─ PoolObserver.GetMetrics() / ListMetrics()
       │
       └─ MetricStore.GetMetrics() / ListMetrics()
```

---

## 三、数据结构定义

### 3.1 `account/tracked_usage.go` 扩展（修正现有缺陷 + 消除重复逻辑）

**背景**：observer 的「双重过滤」和 `usagetracker` 的 `IsQuotaAvailable` 都需要判断某条规则是否触发限流，公式完全相同（`EstimatedUsed() / rule.Total >= safetyRatio`）。将该公式下沉到 `TrackedUsage` 本身，是消除重复的最小侵入方案，同时不引入任何新的跨模块依赖。

同步修正现有缺陷：`usagetracker/default_tracker.go:96` 使用 `u.EstimatedUsed()` 而非正确的估算值；实际上 `EstimatedUsed()` 方法定义为 `RemoteUsed + LocalUsed`，但 `usagetracker` 中的触发回调路径（第 69 行）使用了相同公式，两处保持一致即可，核心问题是 `rule.Total <= 0` 时未跳过，需统一处理。

```go
// account/tracked_usage.go 新增方法

// UsedRatio 返回已用量占总额度的比例。
// rule.Total <= 0 时返回 0.0（视为无限额）。
func (t *TrackedUsage) UsedRatio() float64 {
    if t.Rule == nil || t.Rule.Total <= 0 {
        return 0.0
    }
    return t.EstimatedUsed() / t.Rule.Total
}

// IsThrottled 判断该规则是否已触发限流（达到安全阈值）。
// rule.Total <= 0 时始终返回 false（无限额规则不参与限流判断）。
// usagetracker 的 IsQuotaAvailable 和 observer 的 EffectiveAvailable 过滤均调用此方法，
// 确保两处语义严格一致。
func (t *TrackedUsage) IsThrottled(safetyRatio float64) bool {
    return t.UsedRatio() >= safetyRatio
}
```

**同步变更**：`usagetracker/default_tracker.go` 中以下两处内联公式替换为 `u.IsThrottled(safetyRatio)`：
- 第 66-70 行（`RecordUsage` 的回调触发路径）
- 第 93-97 行（`IsQuotaAvailable`）

替换后同时消除了 `rule.Total <= 0` 未跳过的缺陷。

---

### 3.2 数据类型定义（`account/metrics.go`）

所有指标相关类型统一定义在 `account` 包，与其他账号数据结构（`Account`、`TrackedUsage` 等）保持一致，同时避免 `storage` 包与 `observer` 包之间的循环依赖。

```go
// package account

// GlobalProviderType / GlobalProviderName 是全局指标在 MetricStore 中的特殊标识。
// Metrics.ProviderType == GlobalProviderType 且 ProviderName == GlobalProviderName 时代表全局维度。
const (
    GlobalProviderType = "__global__"
    GlobalProviderName = "__global__"
)

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
    // EstimatedUsed 当前估算已用量（各账号 TrackedUsage.EstimatedUsed() 之和）
    EstimatedUsed float64
    // EstimatedRemain 当前估算剩余量（各账号 TrackedUsage.EstimatedRemain() 之和）
    EstimatedRemain float64
    // RemainRatio 剩余比例（0.0 ~ 1.0）。
    // 计算规则：
    //   TotalQuota <= 0 时为 1.0
    //   否则 = max(0.0, EstimatedRemain / TotalQuota)
    RemainRatio float64
}

// Metrics 账号池指标快照，统一表示 Provider 维度和全局维度。
//
// 全局维度：ProviderType = GlobalProviderType，ProviderName = GlobalProviderName
// Provider 维度：ProviderType/ProviderName 为实际的 Provider 标识
//
// ProviderType + ProviderName 是 MetricStore 的唯一存储键。
// 所有字段均为原始指标，健康状态评估由业务层根据自身阈值自行判断。
type Metrics struct {
    // ---- 标识 ----
    // ProviderType Provider 类型。全局维度时为 GlobalProviderType（"__global__"）。
    ProviderType string
    // ProviderName Provider 名称。全局维度时为 GlobalProviderName（"__global__"）。
    ProviderName string

    // ---- 账号数量统计 ----
    // TotalAccounts 该 Provider（或全局）下的账号总数。
    // 统计范围：全量，含所有状态（Available / CoolingDown / CircuitOpen / Expired / Invalidated / Banned / Disabled）。
    TotalAccounts int
    // EffectiveAvailable 当前真正可被 Pick 流程选出的账号数。
    // 统计范围：在 TotalAccounts 基础上经过双重过滤：
    //   ① Status == Available
    //   ② 所有 UsageRule 对应的 TrackedUsage.IsThrottled(safetyRatio) 均返回 false（未触发限流）
    //      IsThrottled 内部处理 rule.Total <= 0 的规则（视为无限额，不参与限流判断）
    // 说明：并发占用在 OccupancyTotal 中单独统计暴露，业务层可结合两者自行评估。
    EffectiveAvailable int
    // StatusDist 各状态的账号数量分布。
    // 统计范围：全量，StatusDist 各字段之和 == TotalAccounts。
    StatusDist StatusDist

    // ---- 并发占用 ----
    // OccupancyTotal 当前池内所有账号的并发占用槽总数。
    // 统计范围：全量，直接从 OccupancyStore 汇总，不按账号状态过滤。
    // 说明：CoolingDown/CircuitOpen 账号在进入该状态前可能已有请求在途，
    // 其残留占用同样计入，待请求完成后自然归零。
    OccupancyTotal int64

    // ---- 质量指标 ----
    // SuccessRate 近期请求成功率（0.0～1.0）。
    // 统计范围：Status ∈ {Available, CoolingDown, CircuitOpen} 的账号，
    // 排除已永久退出服务的账号（Expired / Invalidated / Banned / Disabled）。
    // 特殊值：TotalCalls == 0 时为 1.0（无历史调用，默认健康）。
    // 计算：sum(SuccessCalls) / sum(TotalCalls)，数据来自 StatsStore.GetStatsBatch()。
    SuccessRate float64
    // TokenQuota Token/积分维度的额度用量汇总。
    // 统计范围：同 SuccessRate，Status ∈ {Available, CoolingDown, CircuitOpen}。
    // 说明：CoolingDown/CircuitOpen 为临时状态，恢复后额度仍可用，故纳入统计，
    // 以反映池子当前真实可用的额度容量而非资产总量。
    // 数据来源：UsageStore.GetCurrentUsagesBatch()，取 SourceType == Token 的规则聚合。
    TokenQuota QuotaMetrics
    // RequestQuota 请求次数维度的额度用量汇总。
    // 统计范围、说明同 TokenQuota；数据来源：UsageStore.GetCurrentUsagesBatch()，取 SourceType == Request 的规则聚合。
    RequestQuota QuotaMetrics

    // ---- 元数据 ----
    // GeneratedAt 本次快照的生成时间（UTC）。
    GeneratedAt time.Time
}

// IsGlobal 判断当前 Metrics 是否为全局维度。
func (m *Metrics) IsGlobal() bool {
    return m.ProviderType == GlobalProviderType && m.ProviderName == GlobalProviderName
}
```

**说明**：
- `QuotaMetrics` 存储 Token/Request 两个维度的原始用量指标，由业务层自行判断是否告警
- `RemainRatio` 方便业务层快速判断，无需再次计算；完整钳制逻辑见第六章 6.2 节
- `QuotaMetrics` 统计范围为「服务生命周期内」的账号（Available + CoolingDown + CircuitOpen），见下方指标统计口径对照表

> **"真实可用"定义**（与 Pick 流程对齐）：
> 1. `account.Status == StatusAvailable`
> 2. 所有 `UsageRule` 对应的 `TrackedUsage.IsThrottled(safetyRatio)` 均返回 false（未触发限流）
>
> **说明**：并发占用限制由 `balancer/occupancy.Controller` 管理（与 Account 解耦），observer 无法感知各账号的并发上限配置，故不做并发过滤。业务层可结合 `EffectiveAvailable` + `OccupancyTotal` 进行综合评估。

### 3.3 指标统计口径对照表

| 指标 | 统计范围 | 说明 |
|------|----------|------|
| `TotalAccounts` | 全量（含所有状态） | 池内账号总数，反映"资产"规模 |
| `StatusDist` | 全量（含所有状态） | 各状态计数之和 == TotalAccounts |
| `EffectiveAvailable` | Available → 双重过滤 | 当前真正可被 Pick 的账号数（不含并发过滤） |
| `OccupancyTotal` | 全量（不按状态过滤） | 直接汇总 OccupancyStore，含 CoolingDown/CircuitOpen 残留占用 |
| `SuccessRate` | Available + CoolingDown + CircuitOpen | 服务生命周期内账号；排除 Expired / Invalidated / Banned / Disabled |
| `TokenQuota` | Available + CoolingDown + CircuitOpen | 同 SuccessRate；CoolingDown/CircuitOpen 恢复后额度仍可用 |
| `RequestQuota` | Available + CoolingDown + CircuitOpen | 同 SuccessRate |

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
    SaveMetrics(ctx context.Context, metrics *account.Metrics) error

    // GetMetrics 读取指定 ProviderType + ProviderName 的指标快照。
    // 不存在时返回 ErrNotFound。
    GetMetrics(ctx context.Context, providerType, providerName string) (*account.Metrics, error)

    // ListMetrics 读取所有已存储的指标快照列表（含全局维度和所有 Provider 维度）。
    // 无快照时返回空切片。
    ListMetrics(ctx context.Context) ([]*account.Metrics, error)

    // ClearMetrics 清空所有快照数据（仅用于测试）。
    ClearMetrics(ctx context.Context) error
}
```

各后端存储实现要点：

| 后端   | SaveMetrics 实现方式 | GetMetrics 实现方式 |
|--------|--------------------|--------------------|
| Memory | `sync.RWMutex` + `map[string]*Metrics`，key 为 `providerType:providerName` | map 直接读取 |
| Redis  | `HSET lumin:metrics:{type}:{name}` JSON 序列化 + `SADD lumin:metrics:index {type}:{name}`（参照 ProviderStorage 模式）| `GET lumin:metrics:{type}:{name}` |
| MySQL  | `INSERT ... ON DUPLICATE KEY UPDATE`，联合唯一索引 `(provider_type, provider_name)` | `SELECT WHERE provider_type=? AND provider_name=?` |
| SQLite | `INSERT OR REPLACE`，联合唯一索引同 MySQL | 同 MySQL |

聚合接口 `storage.Storage` 同步扩展（加入第 7 个子接口）：

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

### 5.1 LeaderElector（`election/election.go`）

`LeaderElector` 是跨模块公共接口，独立放置于 `election/` 包，供 `health`、`observer` 等多个包共同引用，避免各自定义导致的语义分散。

```go
// election/election.go
package election

import "context"

// LeaderElector 用于集群部署时选举 leader，确保同一时刻只有一个实例执行后台任务。
// 未注入时，当前实例默认为 leader（兼容单机部署）。
//
// 实现注意事项：
//   - 锁应设置合理的 TTL，防止实例宕机后锁无法释放。推荐 TTL 为任务 tick 间隔的 2~3 倍。
//   - 分布式锁服务不可用时，建议返回 true（宁可重复执行，也不要全部停止）。
type LeaderElector interface {
    // IsLeader 判断当前实例在指定 key 下是否为 leader。
    // key 用于区分不同的后台任务，每个 key 独立选举，互不影响。
    IsLeader(ctx context.Context, key string) bool
}
```

**迁移说明**：`health` 包中现有的 `LeaderElector` 定义迁移至 `election/` 包，`health` 包改为 `import election` 并将自身类型替换为 `election.LeaderElector`，对外 API（`WithLeaderElector`）参数类型同步更新，无向后兼容问题（该接口由业务方注入，类型变更需业务方同步更新 import 路径）。

**依赖方向**：`health` → `election`，`observer` → `election`，两者均不互相依赖，无循环风险。

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
global, _ := obs.GetMetrics(ctx, account.GlobalProviderType, account.GlobalProviderName)

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
    LeaderElector    election.LeaderElector
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
func WithLeaderElector(key string, le election.LeaderElector) Option
```

---

## 六、核心计算逻辑（`observer/collector.go`）

### 前置：存储层扩展需求

为减少网络往返，需在 `storage.UsageStore` 和 `storage.StatsStore` 接口各增加一个批量接口：

```go
// UsageStore 新增：
// GetCurrentUsagesBatch 批量获取多个账号的用量追踪数据。
// 返回 accountID → []*TrackedUsage 映射，未找到的账号不包含（视为无规则）。
GetCurrentUsagesBatch(ctx context.Context, accountIDs []string) (map[string][]*account.TrackedUsage, error)

// StatsStore 新增：
// GetStatsBatch 批量获取多个账号的运行统计。
// 返回 accountID → *AccountStats 映射，未找到的账号不包含（视为零值统计）。
GetStatsBatch(ctx context.Context, accountIDs []string) (map[string]*account.AccountStats, error)
```

collector 中 EffectiveAvailable / QuotaMetrics 计算调用 `GetCurrentUsagesBatch`，SuccessRate 计算调用 `GetStatsBatch`，各一次性获取所有账号数据。

### 6.1 EffectiveAvailable 双重过滤

```
① 过滤 Status != Available
② 调用 UsageStore.GetCurrentUsagesBatch(所有账号 ID) 一次性获取全部用量数据，
   遍历 Available 账号的所有规则，判断是否触发限流：
     - 调用 TrackedUsage.IsThrottled(safetyRatio) 判断（见 account/tracked_usage.go）
     - IsThrottled 内部：rule.Total <= 0 时返回 false（视为无限额，不排除账号）；
       否则判断 EstimatedUsed() / rule.Total >= safetyRatio
     - 任意规则 IsThrottled 返回 true：该账号排除
   - 批量查询整体失败（GetCurrentUsagesBatch 返回 error）：整次 collectMetrics 返回 error，
     该 Provider 跳过（continue），不更新其快照，保留上次存储的值
   - 单账号在 batch 结果中不存在（map 中无该 key）：视为「无用量规则」，账号通过双重过滤（不排除）

注：并发占用信息通过 OccupancyTotal 单独暴露，业务层可结合 EffectiveAvailable 进行综合评估。
```

### 6.2 QuotaMetrics 聚合计算

```
统计范围：Status ∈ {Available, CoolingDown, CircuitOpen} 的账号
（排除 Expired / Invalidated / Banned / Disabled）

TokenQuota：
  遍历范围内账号，从 GetCurrentUsagesBatch 的结果中取 SourceType == Token 的规则
  （GetCurrentUsagesBatch 已过滤掉 WindowEnd < 当前时间的过期规则，返回当前有效窗口数据）
  TotalQuota      = sum(rule.Total)
  EstimatedUsed   = sum(TrackedUsage.EstimatedUsed())
  EstimatedRemain = sum(TrackedUsage.EstimatedRemain())
  RemainRatio 钳制规则：
    if TotalQuota <= 0:
        RemainRatio = 1.0
    else:
        RemainRatio = max(0.0, EstimatedRemain / TotalQuota)

RequestQuota：同上，取 SourceType == Request 的规则

无规则的账号贡献 0，不影响比例。
```

### 6.3 SuccessRate 计算规则

```
统计范围：Status ∈ {Available, CoolingDown, CircuitOpen} 的账号
（排除 Expired / Invalidated / Banned / Disabled）

调用 StatsStore.GetStatsBatch(范围内账号 ID 列表) 批量获取统计数据
SuccessRate = sum(SuccessCalls) / sum(TotalCalls)
TotalCalls == 0 时返回 1.0（无历史调用，默认健康）
batch 结果中不存在的账号视为零值（TotalCalls=0, SuccessCalls=0），不影响整体计算
```

### 6.4 RefreshNow 执行顺序

```
1. ProviderStorage.SearchProviders(nil)  → 枚举所有 Provider（nil 返回全量）
   失败时直接返回 error，终止本次刷新。
2. 初始化全局累加器 globalMetrics（字段全部为零值）
3. 对每个 Provider 执行 collectMetrics(providerType, providerName)：
   a. AccountStorage.SearchAccounts(按 Provider 过滤)
   b. 计算该 Provider 的各项指标
   c. 若 collectMetrics 返回 error：
      - continue，跳过该 Provider，不调用 SaveMetrics，不累加到 globalMetrics
      - 保留该 Provider 上次写入的快照
   d. 若 collectMetrics 成功：
      - MetricStore.SaveMetrics(providerMetrics)  → 写入 Provider 维度指标
      - 将 providerMetrics 各字段累加到 globalMetrics
4. 全部 Provider 处理完毕后，写入全局维度：
   MetricStore.SaveMetrics(globalMetrics)
   失败时返回 error。
5. RefreshNow 返回 error 的条件：
   - SearchProviders 失败
   - 全局维度 SaveMetrics 失败
   - 单个 Provider 失败只 log，不影响返回值

说明：全局指标直接由各成功 Provider 指标累加得出，不发起额外的全量 SearchAccounts 调用。
      失败的 Provider 不累加到 globalMetrics，全局维度反映的是本次成功收集到的数据之和。
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
election/
└── election.go          // LeaderElector 接口（公共，供 health / observer 共同引用）

account/
├── metrics.go           // 新增：Metrics / StatusDist / QuotaMetrics / GlobalProviderType / GlobalProviderName
└── tracked_usage.go     // 扩展：新增 UsedRatio() / IsThrottled() 方法（同时修正现有实现缺陷）

observer/
├── observer.go          // PoolObserver 接口定义 + NewPoolObserver 构造函数
├── default_observer.go  // defaultPoolObserver 实现
├── collector.go         // collector（内部类型，不导出）
├── option.go            // Options + With* Functional Options（LeaderElector 使用 election.LeaderElector）
└── observer_test.go     // 单元测试（Table-Driven，覆盖率 ≥ 90%）

storage/
├── interface.go         // 新增 MetricStore 第 7 个子接口 + UsageStore/StatsStore 批量方法 + Storage 聚合接口扩展
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

## 九、验收标准与测试策略

> 测试规范遵循 [docs/TESTING.md](../../TESTING.md)：单元测试必须 Table-Driven、禁止 testify；集成测试文件名以 `_integration_test.go` 结尾并添加 `//go:build integration` 构建标签；基准测试文件名以 `_benchmark_test.go` 结尾。

### 测试 Mock 策略

根据 TESTING.md 规范，不同存储后端的 mock 方式如下：

| 存储后端 | Mock 库 | 说明 |
|--------|--------|------|
| Memory | 无需 mock（直接使用 memory store 实现） | memory store 自身无外部依赖，直接创建实例 |
| Redis | `miniredis` | 轻量级 Redis 内存实现，用于单元测试 |
| MySQL | `sqlmock` | SQL 模拟库，拦截数据库调用 |
| SQLite | `os.TempDir()` 内存库 | SQLite 支持 `:memory:` URI 创建内存库 |

PoolObserver 逻辑测试（如 LeaderElector、RefreshNow 流程）使用**手写 mock struct**（不依赖具体存储实现），接口示例：

```go
type mockMetricStore struct {
    saveMetricsErr error
    getMetricsErr  error
    savedMetrics   []*account.Metrics
}
func (m *mockMetricStore) SaveMetrics(ctx context.Context, metrics *account.Metrics) error {
    if m.saveMetricsErr != nil { return m.saveMetricsErr }
    m.savedMetrics = append(m.savedMetrics, metrics)
    return nil
}
// ... 其他接口实现
```

---

## 十、验收标准

### AC-0：`account.TrackedUsage` 新增方法正确性（单元测试）

**测试文件**：`account/tracked_usage_test.go`，函数前缀 `TestTrackedUsage_`

**执行命令**：`go test ./account/...`

| # | 验收条件 | 测试用例场景 |
|---|----------|------------|
| AC-0.1 | `UsedRatio()` = `EstimatedUsed() / rule.Total` | RemoteUsed=700, LocalUsed=200, Total=1000，验证 UsedRatio=0.9 |
| AC-0.2 | `rule.Total <= 0` 时 `UsedRatio()` 返回 0.0 | Total=0 / Total=-1，验证 UsedRatio==0.0 |
| AC-0.3 | `IsThrottled(safetyRatio)` 在 UsedRatio >= safetyRatio 时返回 true | UsedRatio=0.95，safetyRatio=0.95，返回 true；UsedRatio=0.94，返回 false |
| AC-0.4 | `rule.Total <= 0` 时 `IsThrottled` 始终返回 false | Total=0，任意 safetyRatio，返回 false |
| AC-0.5 | `usagetracker.IsQuotaAvailable` 语义与 observer 双重过滤严格一致 | 构造相同账号数据，分别调用 IsQuotaAvailable 和 IsThrottled，验证结论相同 |

---

### AC-1：EffectiveAvailable 双重过滤准确性（单元测试）

**目标**：指标中的 `EffectiveAvailable` 必须与 Pick 流程中可被选出的账号数量严格一致。

**测试文件**：`observer/observer_test.go`，函数前缀 `TestCollector_EffectiveAvailable`

| # | 验收条件 | 测试用例场景 |
|---|----------|------------|
| AC-1.1 | `Status != Available` 的账号不计入 `EffectiveAvailable` | 构造含 CoolingDown/CircuitOpen/Expired/Banned 账号的混合列表，验证只有 Available 的账号参与后续过滤 |
| AC-1.2 | `TrackedUsage.IsThrottled(safetyRatio)` 返回 true 的账号不计入；`rule.Total <= 0` 时 IsThrottled 返回 false 不排除账号 | 模拟 UsageStore 返回 RemoteUsed=900, LocalUsed=50, Total=1000（safetyRatio=0.95），EstimatedUsed()=950，验证该账号被排除；再构造 Total=0 的规则，验证账号不被排除 |
| AC-1.3 | StatusDist 各字段之和 == TotalAccounts | 混合 7 种状态各 2 个账号（共 14 个），验证 StatusDist 总和 == 14，EffectiveAvailable 仅计 Available 中通过双重过滤的数量 |
| AC-1.4 | `GetCurrentUsagesBatch` 整体失败时，该 Provider 的 collectMetrics 返回 error，本次快照不更新，也不累加到 globalMetrics | 模拟 `GetCurrentUsagesBatch` 返回 error，验证 `SaveMetrics` 未被调用，上次快照保留，全局维度不含该 Provider 数据 |
| AC-1.4b | batch 结果中某账号 ID 不存在时（无该 key），视为无用量规则，账号通过双重过滤（不排除） | 模拟 `GetCurrentUsagesBatch` 返回空 map，验证所有 Available 账号均计入 `EffectiveAvailable` |
| AC-1.5 | OccupancyTotal 正确汇总所有账号的并发占用 | 构造 5 个账号各有不同占用数，验证 OccupancyTotal == 汇总值，与 EffectiveAvailable 无关联 |

---

### AC-2：指标计算准确性（单元测试）

**测试文件**：`observer/observer_test.go`，函数前缀 `TestCollector_Metrics`

#### AC-2a：QuotaMetrics 聚合

| # | 验收条件 | 测试用例场景 |
|---|----------|------------|
| AC-2.4 | `TokenQuota.TotalQuota = sum(rule.Total)` for Token 规则 | 3 个账号各有 Token 规则 Total=100/200/300，验证 TotalQuota=600 |
| AC-2.5 | `TokenQuota.EstimatedRemain` = 各账号 EstimatedRemain() 之和，`RemainRatio = EstimatedRemain / TotalQuota` | 已用一半额度，验证 RemainRatio ≈ 0.5（精度 1e-6） |
| AC-2.6 | `RequestQuota` 独立按 `SourceType == Request` 规则汇总，与 `TokenQuota` 不干扰 | 账号同时有 Token 和 Request 规则，验证两个维度分别正确，数值互不影响 |
| AC-2.7 | 无任何用量规则时 `TotalQuota == 0`，`RemainRatio == 1.0` | 无 UsageRule 账号，QuotaMetrics 零值且 RemainRatio=1.0 |
| AC-2.8 | `EstimatedRemain < 0` 时 `RemainRatio` 钳制为 0.0，不返回负值 | EstimatedRemain=-10，验证 RemainRatio == 0.0 |

#### AC-2c：SuccessRate 计算

| # | 验收条件 | 测试用例场景 |
|---|----------|------------|
| AC-2.9 | `SuccessRate = sum(SuccessCalls) / sum(TotalCalls)` | 账号 A 成功 80/100，账号 B 成功 60/100，验证 SuccessRate = 0.7 |
| AC-2.10 | `TotalCalls == 0` 时 `SuccessRate == 1.0` | 新建账号无历史调用 |

---

### AC-3：MetricStore 存储后端正确性（单元测试）

**测试文件**：各后端 `storage/{backend}/metric_test.go`

**执行命令**：`go test ./storage/...`（Memory 直接测，Redis 用 miniredis，MySQL 用 sqlmock，SQLite 用内存库）

| # | 验收条件 | 测试用例场景 |
|---|----------|------------|
| AC-3.1 | `SaveMetrics` 后 `GetMetrics` 读到完全相同的结构（含所有数值字段，含 QuotaMetrics） | 构造完整 Metrics，写入后读取，逐字段比对 |
| AC-3.2 | 相同 `ProviderType+ProviderName` 第二次写入覆盖第一次（upsert 语义） | 写入两次，第二次修改 EffectiveAvailable 和 GeneratedAt，`GetMetrics` 返回第二次的值 |
| AC-3.3 | `ListMetrics` 返回全局 + 所有 Provider 的完整列表 | 写入全局维度 + 3 个 Provider，验证返回 4 条，且可通过 `IsGlobal()` 区分 |
| AC-3.4 | 全局维度与 Provider 维度同 key 不冲突 | 分别写入全局 `(__global__, __global__)` 和 Provider `(kiro, kiro-prod)`，互不覆盖 |
| AC-3.5 | `GetMetrics` 对不存在的 key 返回 `ErrNotFound` | 空 store 直接 Get 任意 key |
| AC-3.6 | `ClearMetrics` 后 `ListMetrics` 返回空切片，`GetMetrics` 返回 `ErrNotFound` | 写入若干条后清空，全量验证 |
| AC-3.7 | 4 种后端（Memory/Redis/MySQL/SQLite）均通过 AC-3.1～AC-3.6 全部用例 | 同一测试逻辑分别在各后端运行 |

---

### AC-4：PoolObserver 生命周期（单元测试）

**测试文件**：`observer/observer_test.go`，函数前缀 `TestPoolObserver_`

**执行命令**：`go test -race ./observer/...`（并发相关用例强制加 -race）

| # | 验收条件 | 测试用例场景 |
|---|----------|------------|
| AC-4.1 | `Start()` 后立即 `Stop()` 不 panic，goroutine 正常退出 | `Start(); Stop()`，通过 `done` channel 或 WaitGroup 确认退出 |
| AC-4.2 | `Start()` 重复调用返回 error，不启动多个 goroutine | 连续两次 `Start()`，第二次 error 不为 nil |
| AC-4.3 | `Stop()` 可安全多次调用，不 panic | 连续调用两次 `Stop()` |
| AC-4.4 | 未注入 `LeaderElector`（nil）时默认为 leader，`RefreshNow` 正常触发写入 | mock MetricStore，验证 SaveMetrics 被调用 |
| AC-4.5 | 注入非 leader 的 `LeaderElector` 时，tick 触发时不执行写入 | mock `IsLeader()` 返回 false，等待两个 tick，验证 `MetricStore.SaveMetrics` 调用次数为 0 |
| AC-4.6 | 注入 leader 的 `LeaderElector` 时，tick 触发时正常写入 | mock `IsLeader()` 返回 true，等待一个 tick，验证 `MetricStore.SaveMetrics` 被调用 |
| AC-4.7 | `RefreshNow()` 在全局维度 `SaveMetrics` 失败时返回 error，不 panic | mock 全局维度 `SaveMetrics` 返回 error，验证返回值不为 nil |
| AC-4.8 | 单个 Provider 指标计算失败时 `RefreshNow` 仍继续处理其余 Provider，失败 Provider 不累加到全局 | mock `AccountStorage.SearchAccounts` 对 Provider A 返回 error，Provider B/C 正常，验证 B/C 写入成功，全局维度只含 B+C 数据，RefreshNow 返回 nil |
| AC-4.9 | `NewPoolObserver` 缺少任一必填 storage 时返回 error，不返回 nil | 逐一缺省 MetricStore/AccountStorage/ProviderStorage/StatsStore/UsageStore/OccupancyStore，各自返回 error |

---

### AC-5：多实例一致性（集成测试）

**测试文件**：`observer/observer_integration_test.go`

**构建标签**：`//go:build integration`

**执行命令**：`go test -tags=integration -v ./observer/...`

| # | 验收条件 | 测试场景 |
|---|----------|---------|
| AC-5.1 | 两个 `defaultPoolObserver` 共享同一 Redis（miniredis）`MetricStore`，实例 A `RefreshNow` 后实例 B `GetMetrics` 读到相同数据 | 实例 A 写入，实例 B 独立读取，对比全部字段 |
| AC-5.2 | 两个实例共享同一 MySQL（sqlmock）`MetricStore`，行为与 AC-5.1 相同 | 同上，换 sqlmock 后端 |
| AC-5.3 | leader 实例写入后，非 leader 实例通过 `ListMetrics` 可读到最新快照 | 实例 A leader 写入，实例 B IsLeader=false 只读，验证读取结果与写入一致 |
| AC-5.4 | 两个实例并发 `RefreshNow` 时（upsert 竞争），最终 MetricStore 中数据为其中一次写入的合法值，不出现部分写/混合数据 | goroutine A、B 同时调用 `RefreshNow`，写入后读取，验证结构合法无混乱 |

---

### AC-6：性能（基准测试）

**测试文件**：`observer/observer_benchmark_test.go`

**执行命令**：`go test -bench=. -benchmem -count=5 ./observer/...`

| # | 验收条件 | 基准函数 | 通过标准 |
|---|----------|---------|---------|
| AC-6.1 | Memory 后端 `GetMetrics` 单次调用耗时 | `BenchmarkGetMetrics_Memory` | ns/op < 10,000（即 P99 < 10µs） |
| AC-6.2 | Memory 后端 `ListMetrics`（10 个 Provider + 全局）单次调用耗时 | `BenchmarkListMetrics_Memory_10Providers` | ns/op < 50,000 |
| AC-6.3 | Memory 后端 `RefreshNow` 对 100 个账号（10 Provider × 10 账号）全量计算耗时 | `BenchmarkRefreshNow_100Accounts` | 单次 < 500ms（即 ns/op < 500,000,000） |
| AC-6.4 | Memory 后端 `SaveMetrics` 并发写（100 goroutine）吞吐量 | `BenchmarkSaveMetrics_Concurrent` | 无 data race（配合 `-race` 运行） |
| AC-6.5 | `RefreshNow` 全程不在 `Balancer.Pick()` 调用栈内（零侵入调度流程） | 代码审查 | 确认 `balancer/` 目录下无 `observer` 包导入 |

---

### AC-7：代码质量（执行命令汇总）

| # | 验收条件 | 执行命令 | 通过标准 |
|---|----------|---------|---------|
| AC-7.0 | `account/` 包 `TrackedUsage` 新增方法测试通过 | `go test ./account/...` | 全部 PASS |
| AC-7.1 | `observer/` 包单元测试覆盖率 ≥ 90% | `go test -cover ./observer/...` | coverage ≥ 90% |
| AC-7.2 | `storage/` 各后端 MetricStore 实现覆盖率 ≥ 90% | `go test -cover ./storage/...` | coverage ≥ 90% |
| AC-7.3 | `observer/`、`storage/`、`account/` 并发测试通过 `-race` 检测 | `go test -race ./observer/... ./storage/... ./account/...` | 无 data race 报告 |
| AC-7.4 | 集成测试（需 `-tags=integration`）通过 | `go test -tags=integration -v ./observer/...` | 全部 PASS |
| AC-7.5 | 所有测试使用 Table-Driven 模式，禁止 testify，使用原生 `testing` 包 | 代码审查 | 无 `github.com/stretchr/testify` 导入 |
| AC-7.6 | `collector`、`defaultPoolObserver` 等实现类型不导出 | 代码审查 | `observer/` 包中无以大写开头的实现类型 |
| AC-7.7 | `balancer/` 目录下无 `observer` 包导入（零侵入） | `grep -r "observer" balancer/` | 输出为空 |

---

## 十、不在本次范围内

以下内容明确排除在本次实现之外：

- **补账号/补 Provider 的判断逻辑**：上游业务层根据 `EffectiveAvailable`、`TokenQuota`、`SuccessRate` 等原始指标自定义阈值判断，本模块只提供原始指标
- **指标历史时序存储**：本模块只保留最新快照，历史趋势由上游监控系统（Prometheus/Grafana）负责
- **HTTP / Prometheus exporter**：本模块是 SDK 库，不提供网络服务端点，由上游 lumin-proxy / lumin-admin 自行暴露
- **告警触发**：不在本模块处理，由业务层订阅快照后决定
- **MetricStore 的 LeaderElector 实现**：业务层或上游项目自行提供基于 Redis/MySQL 的实现

---

## 十一、后续可扩展方向（YAGNI，当前不实现）

- `Metrics` 增加 `P99Latency` 等延迟分布字段（需要 StatsStore 扩展）
- 全局维度 `Metrics` 增加模型维度的分组统计
- `MetricStore` 增加历史快照版本保留，支持时序查询
