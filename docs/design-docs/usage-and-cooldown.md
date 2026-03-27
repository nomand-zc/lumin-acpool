# 用量追踪与冷却机制

## UsageTracker 核心设计

**本地乐观计数 + 远端定期校准**，在不频繁请求远端 API 的情况下实时估算账号配额使用情况。

### TrackedUsage 数据模型

```
TrackedUsage
├── Rule          — 关联的用量规则（来自 lumin-client 的 UsageRule）
├── LocalUsed     — 本地乐观计数（自上次校准以来的本地累加量）
├── RemoteUsed    — 远端快照已用量
├── RemoteRemain  — 远端快照剩余量
├── WindowStart   — 窗口起始时间
├── WindowEnd     — 窗口结束时间
└── LastSyncAt    — 上次校准时间
```

**估算公式：**
- `EstimatedRemain = RemoteRemain - LocalUsed`
- `EstimatedUsed = RemoteUsed + LocalUsed`
- `IsExhausted = EstimatedRemain <= 0`

### 工作流程

```
1. RecordUsage(amount)
   └─ UsageStore.IncrLocalUsed(ruleIndex, amount)  ← 原子递增
   └─ 若触及安全阈值 → 回调 → CooldownManager.StartCooldown

2. IsQuotaAvailable(accountID)  ← Resolver 层选号前调用
   └─ 遍历所有规则，检查 EstimatedRemain > 0

3. Calibrate(accountID, remoteStats)  ← HealthChecker 定期调用
   └─ UsageStore.CalibrateRule: 原子设置 remote_used, remote_remain, 重置 local_used=0
   └─ 避免全量 Save 覆盖丢失并发 IncrLocalUsed 的增量

4. CalibrateFromResponse(accountID)  ← ReportFailure 收到 429 时调用
   └─ 将对应规则标记为已耗尽（RemoteRemain=0）
```

### 安全阈值回调

当 `RecordUsage` 检测到某条规则配额达到安全阈值时，触发回调：
- Balancer 自动创建的 UsageTracker 内置冷却回调
- 回调逻辑：获取 Account → 状态设为 CoolingDown → 持久化

## CooldownManager

管理因限流或配额耗尽而需要冷却的账号。

| 配置 | 默认值 | 说明 |
|------|--------|------|
| `DefaultDuration` | 30s | 无 Retry-After 时的默认冷却时长 |

### 冷却时间来源（优先级）

1. 请求响应中的 `Retry-After`（由 `extractRetryAfter(err)` 提取）
2. CooldownManager 的 `DefaultDuration`

### 冷却恢复

由 `HealthChecker` 的 `RecoveryCheck` 负责检测冷却到期，建议恢复为 Available。
`RecoveryCheck` 无网络开销，应以 5~10s 的短间隔注册。

## CircuitBreaker

基于**连续失败次数**触发熔断。

### 阈值计算

```
if 账号有 UsageRules:
    threshold = max(rule.Total * ThresholdRatio, MinThreshold)
else:
    threshold = DefaultThreshold
```

| 配置 | 默认值 | 说明 |
|------|--------|------|
| `DefaultThreshold` | 5 | 无 UsageRules 时的阈值 |
| `DefaultTimeout` | 60s | 熔断恢复窗口 |
| `ThresholdRatio` | 0.5 | 动态阈值 = Total × Ratio |
| `MinThreshold` | 3 | 动态计算后的最小值 |

### 半开探测

`ShouldAllow(acct)` 检查 `CircuitOpenUntil` 是否已过期，过期则允许半开探测。
