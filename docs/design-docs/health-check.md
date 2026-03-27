# 健康检查系统

## 架构

```
HealthChecker（编排器）
  │
  ├─ Register(CheckSchedule) — 注册检查项 + 执行间隔
  ├─ Unregister(checkName) — 移除检查项
  ├─ ListChecks() — 列出所有已注册检查项
  ├─ Start(ctx) — 启动后台周期任务
  │   └─ 每个检查项按自己的 Interval 独立执行
  │   └─ LeaderElector.IsLeader() — 集群模式下选主
  ├─ Stop() — 停止后台检查
  │
  ├─ RunAll(ctx, target) — 对指定账号执行所有检查
  │   └─ 按 DependsOn 拓扑排序执行
  │   └─ 依赖失败 → 自动 Skipped
  ├─ RunOne(ctx, target, checkName) — 执行单个检查项
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

| 检查项 | 名称常量 | 源文件 | 严重级别 | 依赖 | 说明 |
|--------|---------|--------|---------|------|------|
| `RecoveryCheck` | `recovery` | `checks/recovery.go` | Critical | 无 | 冷却/熔断到期恢复（纯本地时间判断，无网络） |
| `CredentialValidityCheck` | `credential_validity` | `checks/credential.go` | Critical | 无 | 凭证格式和过期状态验证（本地，无网络） |
| `CredentialRefreshCheck` | `credential_refresh` | `checks/refresh.go` | Critical | `credential_validity` | Token 过期刷新（调用 Provider.Refresh） |
| `ProbeCheck` | `probe` | `checks/probe.go` | Warning | `credential_refresh` | 探活请求（实际调用 API 验证可用性） |
| `UsageQuotaCheck` | `usage_quota` | `checks/usage.go` | Critical | `credential_refresh` | 获取远端用量统计（校准 UsageTracker，配额耗尽触发冷却） |
| `UsageRulesRefreshCheck` | `usage_rules_refresh` | `checks/usage_rules_refresh.go` | Info | `credential_refresh` | 动态刷新用量规则 |
| `ModelDiscoveryCheck` | `model_discovery` | `checks/model_discovery.go` | Info | `credential_validity` | 动态发现支持的模型列表 |

### 依赖链

```
credential_validity → credential_refresh → probe
                                        → usage_quota
                                        → usage_rules_refresh
credential_validity → model_discovery
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

`NewDefaultReportCallback(deps)` 消费 HealthReport，对每条 CheckResult 依次执行：

1. **UsageStats 校准**：`Data["usage_stats"]` → `UsageTracker.Calibrate()`（不触发 Account 持久化）
2. **SupportedModels 更新**：`Data["supported_models"]` → `ProviderStorage.UpdateProvider()`
3. **UsageRules 刷新**：`Data["usage_rules"]` → 更新 `Account.UsageRules` + `UsageTracker.InitRules()`（触发持久化）
4. **凭证刷新标记**：`Data["credential_refreshed"]=true` → 标记凭证字段需要持久化
5. **状态变更**：`SuggestedStatus` 非 nil → 调整 Account 状态（`CoolingDown` 经由 `CooldownManager.StartCooldown`）
6. **持久化**：`AccountStorage.UpdateAccount(fields)` 按实际变更字段掩码写入（`UpdateFieldStatus|UpdateFieldUsageRules|UpdateFieldCredential`）

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
