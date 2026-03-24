# 核心设计原则

## 1. Account 是聚合根

`Account` 是核心领域模型，持有 Credential、Status、UsageRules 等所有账号相关状态。
所有状态变更通过 `AccountStorage.UpdateAccount(acct, fields)` 的位掩码部分更新，**禁止全量覆盖**。

理由：并发场景下 Balancer 和 HealthChecker 可能同时修改不同字段，全量覆盖会丢失对方的变更。

## 2. 高频统计与低频数据分离

`AccountStats`（调用次数、连续失败数等）从 Account 独立存储到 `StatsStore`，支持原子 Incr 操作。
`TrackedUsage`（用量追踪）独立存储到 `UsageStore`，支持 `IncrLocalUsed` 原子递增。

理由：每次 ReportSuccess/Failure 都需要更新统计，如果和 Account 同一个 Store，全量覆盖竞争会极其严重。

## 3. 乐观锁防止并发覆盖

`Account.Version` 每次 `UpdateAccount` 递增，存储层在 WHERE 条件中检查 version，
冲突时返回 `ErrVersionConflict`。上层对 version 冲突采取**静默忽略**策略（幂等设计）。

理由：集群部署时多个实例可能同时修改同一个 Account，乐观锁是轻量级的并发控制手段。

## 4. 接口隔离

存储层拆分为 6 个细粒度子接口（AccountStorage / ProviderStorage / StatsStore / UsageStore / OccupancyStore / AffinityStore），
消费端依赖具体子接口而非聚合接口 `Storage`。

理由：不同存储后端可能只实现部分接口（如 Redis 不适合做 AccountStorage 的全功能实现），
聚合接口 `Storage` 仅用于简化一体化后端的初始化传递。

## 5. 单机/集群共享接口

所有接口（Storage / OccupancyStore / AffinityStore / LeaderElector）统一定义，
单机用内存实现，集群用 Redis/MySQL 实现。调用方无感知切换。

理由：lumin-desktop（单机）和 lumin-proxy（集群）共享同一套 acpool 逻辑。

## 6. Pick 返回深拷贝

`Pick` 返回的 `Account` 是 `Clone()` 深拷贝，调用者对其的任何修改不影响池内状态。

理由：避免调用者误修改池内数据，保证 Balancer 内部数据一致性。

## 7. Report 是必须的

`ReportSuccess` / `ReportFailure` 不仅更新统计，还负责：
- 释放占用槽位（Acquire 的对称操作）
- 驱动状态机（熔断、冷却）
- 记录用量（配额管理）

**忘记调用 Report 会导致占用槽位泄漏。**

## 8. Functional Options

所有组件构造器使用 Functional Options 模式，提供合理默认值 + 灵活扩展。
