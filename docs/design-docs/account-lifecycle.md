# 账号状态机

## 7 种状态

| 状态 | 值 | 可选号 | 可恢复 | 含义 |
|------|---|--------|--------|------|
| `Available` | 1 | ✅ | — | 正常可用 |
| `CoolingDown` | 2 | ❌ | ✅ 自动 | 限流冷却中，等待到期 |
| `CircuitOpen` | 3 | ❌ | ✅ 自动 | 熔断中，连续失败过多 |
| `Expired` | 4 | ❌ | ✅ 刷新 | Token 过期，需 Refresh |
| `Invalidated` | 5 | ❌ | ❌ | 永久失效（refresh token 无效） |
| `Banned` | 6 | ❌ | ❌ | 被平台封禁 |
| `Disabled` | 7 | ❌ | ❌ | 管理员手动禁用 |

## 状态转换图

```
                    ┌────────────────────┐
                    │    Available (1)   │ ← 初始 / 恢复目标
                    └──┬──┬──┬──┬───────┘
                       │  │  │  │
          ┌────────────┘  │  │  └────────────┐
          ▼               ▼  ▼               ▼
   ┌──────────────┐ ┌──────────┐  ┌─────────────────┐
   │ CoolingDown  │ │CircuitOpen│ │    Expired      │
   │     (2)      │ │   (3)    │  │     (4)         │
   └──────┬───────┘ └────┬─────┘  └───────┬─────────┘
          │               │                │
   到期自动恢复      到期自动恢复       Token 刷新成功
   (RecoveryCheck)  (RecoveryCheck)    (RefreshCheck)
          │               │                │
          └───────┬───────┘────────────────┘
                  ▼
            → Available

   失败路径（不可自动恢复）：
   ┌──────────────┐  ┌──────────┐  ┌──────────┐
   │ Invalidated  │  │  Banned  │  │ Disabled │
   │    (5)       │  │   (6)    │  │   (7)    │
   └──────────────┘  └──────────┘  └──────────┘
    需人工移除/替换    需人工处理     管理员手动启用
```

## 触发状态变更的来源

| 来源 | 触发场景 | 目标状态 |
|------|---------|---------|
| `ReportFailure` | 限流错误（429） | → CoolingDown |
| `ReportFailure` | 连续失败超阈值 | → CircuitOpen |
| `ReportSuccess` | CircuitOpen 账号成功 | → Available |
| `HealthChecker` | RecoveryCheck 冷却/熔断到期 | → Available |
| `HealthChecker` | RefreshCheck Token 刷新成功 | → Available |
| `HealthChecker` | CredentialCheck 凭证无效 | → Invalidated |
| `HealthChecker` | ProbeCheck 探活失败（封禁） | → Banned |
| `HealthChecker` | UsageCheck 配额耗尽 | → CoolingDown |
| 管理员 API | 手动禁用 | → Disabled |

## 可用性判断

- `Status.IsSelectable()` — 仅 `Available` 返回 true
- `Status.IsRecoverable()` — `CoolingDown`、`CircuitOpen`、`Expired` 返回 true
- Resolver 层只返回 `Status == Available` 的账号参与选号
