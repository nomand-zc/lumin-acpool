# 内置选号策略一览

## Account 级策略

位于 `selector/strategies/account/`

| 策略 | 说明 |
|------|------|
| **RoundRobin** | 轮询选号（默认策略） |
| **Priority** | 按 Account.Priority 降序 |
| **Weighted** | 按权重加权随机 |
| **LeastUsed** | 最少使用，优先选配额充裕的账号 |
| **Affinity** | 亲和绑定，同一 UserID 绑定到同一 Account |

## Group 级策略

位于 `selector/strategies/group/`

| 策略 | 说明 |
|------|------|
| **GroupPriority** | 按 Priority 降序（默认策略） |
| **GroupRoundRobin** | Provider 轮询 |
| **GroupWeighted** | 按 Weight 加权随机 |
| **GroupMostAvailable** | 选可用账号数最多的 Provider |
| **GroupAffinity** | 亲和绑定，同一 UserID 绑定到同一 Provider |

## 默认配置

- Account 级：RoundRobin
- Group 级：GroupPriority
- OccupancyController：Unlimited

## Occupancy Controller

位于 `balancer/occupancy/`

| 控制器 | 说明 |
|--------|------|
| **Unlimited** | 不限制并发（默认） |
| **FixedLimit** | 固定并发上限 |
| **AdaptiveLimit** | 基于配额动态调整上限 |
