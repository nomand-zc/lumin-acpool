# 健康检查系统

## 架构

```
HealthChecker（编排器）
  │
  ├─ Register(CheckSchedule) — 注册检查项 + 执行间隔
  ├─ Start(ctx) — 启动后台周期任务
  │   └─ 每个检查项按自己的 Interval 独立执行
  │   └─ LeaderElector.IsLeader() — 集群模式下选主
  │
  ├─ RunAll(ctx, target) — 对指定账号执行所有检查
  │   └─ 按 DependsOn 拓扑排序执行
  │   └─ 依赖失败 → 自动 Skipped
  │
  └─ ReportCallback — 消费 HealthReport 驱动状态变更
```

## CheckTarget 接口

封装被检查对象的信息：

```go
CheckTarget interface {
    Credential() credentials.Credential  // 账号凭证
    Client() providers.Provider          // Provider 客户端（用于 API 调用）
    Account() *account.Account           // 完整账号对象
}
```

## 内置检查项

| 检查项 | 包 | 严重级别 | 依赖 | 说明 |
|--------|---|---------|------|------|
| `RecoveryCheck` | `checks/recovery.go` | Critical | 无 | 冷却/熔断到期恢复（纯本地时间判断，无网络） |
| `CredentialCheck` | `checks/credential.go` | Critical | 无 | 凭证有效性验证（Validate） |
| `RefreshCheck` | `checks/refresh.go` | Critical | credential | Token 过期刷新（调用 Provider.Refresh） |
| `ProbeCheck` | `checks/probe.go` | Critical | refresh | 探活请求（实际调用 API 验证可用性） |
| `UsageCheck` | `checks/usage.go` | Warning | refresh | 获取远端用量统计（校准 UsageTracker） |
| `UsageRulesRefresh` | `checks/usage_rules_refresh.go` | Info | refresh | 刷新用量规则 |
| `ModelDiscovery` | `checks/model_discovery.go` | Info | refresh | 动态发现支持的模型列表 |

### 依赖链

```
credential → refresh → probe
                    → usage
                    → usage_rules_refresh
                    → model_discovery
```

## HealthCheck 接口

```go
HealthCheck interface {
    Name() string                                         // 唯一标识
    Severity() CheckSeverity                              // Info / Warning / Critical
    Check(ctx, target) *CheckResult                       // 执行检查
    DependsOn() []string                                  // 前置依赖
}
```

**约定**：即使检查过程出错，也返回 `CheckResult(Status=CheckError)` 而非 error。
确保 HealthReport 总能收集所有检查项结果。

## CheckResult

```go
CheckResult {
    Status          CheckStatus     // Passed / Warning / Failed / Skipped / Error
    Severity        CheckSeverity   // Info / Warning / Critical
    SuggestedStatus *account.Status // 建议的状态变更（nil = 无建议）
    Data            any             // 附加数据（如 UsageStats / CooldownUntil / SupportedModels）
}
```

## ReportHandler

`NewDefaultReportCallback(deps)` 消费 HealthReport，执行：

1. **UsageStats 校准** → `UsageTracker.Calibrate()`
2. **SupportedModels 更新** → `ProviderStorage.UpdateProvider()`
3. **UsageRules 刷新** → 更新 Account.UsageRules + `UsageTracker.InitRules()`
4. **状态变更** → 根据 `SuggestedStatus` 更新 Account 状态
5. **持久化** → `AccountStorage.UpdateAccount(fields)`

## LeaderElector

集群部署时，避免多个实例同时执行健康检查（API 请求放大）。

```go
LeaderElector interface {
    IsLeader(ctx, key string) bool
}
```

- 单机部署：不注入，所有实例默认执行
- 集群部署：注入基于 Redis/MySQL 分布式锁的实现
- 推荐 TTL = 检查间隔的 2~3 倍
- 分布式锁不可用时，`IsLeader` 应返回 true（宁可重复也不停止）
