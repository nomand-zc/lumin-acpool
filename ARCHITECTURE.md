# ARCHITECTURE.md — lumin-acpool 架构文档

## 系统定位

lumin-acpool 是 LUMIN 生态的**账号池层**，位于 lumin-client（SDK 层）之上、lumin-proxy（代理层）之下。
它将多个 AI 平台账号聚合为一个资源池，通过负载均衡、熔断、冷却、健康检查等机制实现高可用调度。

```
lumin-proxy / lumin-desktop（消费端）
        │
        ▼
┌──────────────────────────────────┐
│       lumin-acpool（账号池）       │  ← 本仓库
│  Pick → 选号 → Report → 状态驱动  │
└──────────┬───────────────────────┘
           │
           ▼
┌──────────────────────────────────┐
│     lumin-client（统一 SDK）      │
│  Provider.GenerateContentStream  │
└──────────────────────────────────┘
```

## 核心流程：Pick → Use → Report

```
调用者
  │
  ├─ 1. Balancer.Pick(model, providerKey?, tags?)
  │     ├─ Resolver 解析候选 Provider + Account
  │     ├─ GroupSelector 选 Provider（group 级策略）
  │     ├─ OccupancyController.FilterAvailable 过滤已满账号
  │     ├─ Selector 选 Account（account 级策略）
  │     ├─ OccupancyController.Acquire 获取占用槽位
  │     └─ 返回 PickResult（Account 深拷贝 + ProviderKey）
  │
  ├─ 2. 使用 Account.Credential 调用 lumin-client
  │
  └─ 3a. Balancer.ReportSuccess(accountID)
  │       ├─ Release 占用槽位
  │       ├─ StatsStore.IncrSuccess
  │       ├─ UsageTracker.RecordUsage
  │       └─ CircuitBreaker.RecordSuccess（可能恢复 CircuitOpen → Available）
  │
  └─ 3b. Balancer.ReportFailure(accountID, err)
          ├─ Release 占用槽位
          ├─ StatsStore.IncrFailure（原子返回 consecutiveFailures）
          ├─ UsageTracker.RecordUsage
          ├─ 若限流错误 → CooldownManager.StartCooldown → CoolingDown
          └─ 若其他错误 → CircuitBreaker.RecordFailure → 可能 CircuitOpen
```

## Balancer 组件依赖

```
┌─────────────────────────────────────────────────┐
│                   Balancer                       │
│                                                  │
│  ┌──────────┐  ┌─────────────┐  ┌────────────┐  │
│  │ Resolver │  │GroupSelector│  │  Selector  │  │
│  └────┬─────┘  └─────────────┘  └────────────┘  │
│       │                                          │
│  ┌────▼──────────────────────────────────────┐   │
│  │           Storage（6 个子接口）             │   │
│  │ AccountStorage  ProviderStorage  StatsStore│   │
│  │ UsageStore  OccupancyStore  AffinityStore │   │
│  └───────────────────────────────────────────┘   │
│                                                  │
│  ┌──────────────┐ ┌───────────┐ ┌────────────┐  │
│  │CircuitBreaker│ │ Cooldown  │ │UsageTracker│  │
│  └──────────────┘ └───────────┘ └────────────┘  │
│                                                  │
│  ┌────────────────────┐                          │
│  │OccupancyController │                          │
│  └────────────────────┘                          │
└─────────────────────────────────────────────────┘
              ↕
┌─────────────────────────────────────────────────┐
│               HealthChecker                      │
│  ┌─────────┐ ┌────────┐ ┌──────┐ ┌───────────┐ │
│  │Recovery │ │Refresh │ │Probe │ │  Usage    │ │
│  │Check    │ │Check   │ │Check │ │  Check   │ │
│  └─────────┘ └────────┘ └──────┘ └───────────┘ │
│       → ReportHandler 消费结果 → 驱动状态变更     │
└─────────────────────────────────────────────────┘
```

## 存储接口体系

```
Storage（聚合接口 — 简化初始化，消费端仍依赖子接口）
├── AccountStorage     — Account CRUD（支持部分字段更新 UpdateField 位掩码）
├── ProviderStorage    — ProviderInfo CRUD
├── StatsStore         — 运行时统计（原子 IncrSuccess/IncrFailure）
├── UsageStore         — 用量追踪数据（IncrLocalUsed / CalibrateRule 原子操作）
├── OccupancyStore     — 并发占用计数（原子 Incr/Decr）
└── AffinityStore      — 亲和绑定关系（affinityKey → targetID）
```

4 种后端实现：`memory/`（单机）、`sqlite/`（单机持久化）、`mysql/`（集群）、`redis/`（集群高性能）

## 关键数据模型

| 模型 | 位置 | 角色 |
|------|------|------|
| `Account` | `account/account.go` | 聚合根，持有 Credential + Status + UsageRules |
| `ProviderInfo` | `account/provider.go` | Provider 元数据（SupportedModels / Weight / Priority） |
| `AccountStats` | `account/account_stats.go` | 运行时统计（独立于 Account，支持高频原子更新） |
| `TrackedUsage` | `account/tracked_usage.go` | 单条规则的用量追踪（远端快照 + 本地乐观计数） |
| `ProviderKey` | `account/provider.go` | Provider 复合标识（Type + Name） |
| `Status` | `account/status.go` | 7 种账号状态枚举 |

## 关键依赖

| 依赖 | 用途 |
|------|------|
| `lumin-client` | 统一 AI SDK（Credential / Provider / UsageRule） |
| `go-sql-driver/mysql` | MySQL 存储后端 |
| `mattn/go-sqlite3` | SQLite 存储后端 |
| `redis/go-redis/v9` | Redis 存储后端 |
| `spf13/cobra` | CLI 命令行框架 |
| `google/uuid` | UUID 生成 |
