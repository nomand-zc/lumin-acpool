# 集群部署注意事项

## 需要共享存储的组件

| 组件 | 单机实现 | 集群实现 | 原因 |
|------|---------|---------|------|
| `OccupancyStore` | Memory | Redis | 跨实例共享占用计数 |
| `AffinityStore` | Memory | Redis | 跨实例共享亲和绑定 |
| `StatsStore` | Memory | Redis/MySQL | 跨实例共享统计数据 |
| `UsageStore` | Memory | Redis | 跨实例共享用量追踪 |
| `AccountStorage` | Memory/SQLite | MySQL | 跨实例共享账号数据 |
| `ProviderStorage` | Memory/SQLite | MySQL | 跨实例共享供应商数据 |

## LeaderElector

健康检查后台任务需要选主，避免多实例同时执行 API 探活导致请求放大。

- 注入 `health.WithLeaderElector(leaderKey, impl)`（leaderKey 为分布式锁键名）
- 推荐基于 Redis 分布式锁实现
- TTL 设为检查间隔的 2~3 倍
- 锁服务不可用时 `IsLeader` 应返回 true（宁可重复也不停止）

## 乐观锁

`Account.Version` 每次 UpdateAccount 递增，存储层 WHERE 检查 version。
冲突返回 `ErrVersionConflict`，上层静默忽略（幂等设计）。

## 典型部署架构

```
实例 A ──┐
实例 B ──┤──→ MySQL（持久化）+ Redis（高频操作）
实例 C ──┘
```
