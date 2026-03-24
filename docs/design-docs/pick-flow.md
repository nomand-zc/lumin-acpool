# Pick 选号流程

## 三种调度模式

由 `PickRequest.ProviderKey` 决定：

| 模式 | ProviderKey 状态 | 行为 |
|------|-----------------|------|
| **精确供应商**（Mode 1） | Type + Name 非空 | 直接从指定 Provider 选账号 |
| **按类型过滤**（Mode 2） | 仅 Type 非空 | GroupSelector 从该类型的 Provider 中选 |
| **全自动**（Mode 3） | nil | GroupSelector 从所有 Provider 中选 |

## 完整流程

```
Pick(model, providerKey?, tags?)
  │
  ├─ Mode 1: pickExact
  │   ├─ Resolver.ResolveProvider(key, model) → 验证存在/活跃/支持模型
  │   └─ selectAccountFromProvider(provider)
  │
  ├─ Mode 2/3: pickAuto
  │   ├─ Resolver.ResolveProviders(model, type?) → 候选 Provider 列表
  │   ├─ rand.Shuffle(candidates) → 随机打散（分散竞争热点）
  │   ├─ GroupSelector.Select(candidates) → 选一个 Provider
  │   └─ selectAccountFromProvider(provider)
  │
  └─ selectAccountFromProvider(provider):
      ├─ Resolver.ResolveAccounts(key, tags, excludeIDs)
      ├─ OccupancyController.FilterAvailable → 排除并发已满账号
      ├─ rand.Shuffle(accounts) → 随机打散
      └─ acquireFromAccounts:
          ├─ 排除 ExcludeAccountIDs
          ├─ Selector.Select(filtered) → 选一个 Account
          ├─ OccupancyController.Acquire → 获取占用槽位
          │   ├─ 成功 → 返回 PickResult（Clone）
          │   └─ 失败（竞态） → 排除此账号，重试
          └─ 重试直到 maxRetries 耗尽
```

## Failover 机制

当 `EnableFailover=true` 且当前 Provider 下无可用账号时：

- **pickAuto**：排除当前 Provider，重新 GroupSelector.Select，循环直到成功或候选耗尽
- **pickExact**：精确供应商失败后，降级到 pickAuto（保留 ProviderKey.Type 作为类型约束），结果标记 `Fallback=true`

## Retry 机制

当 `MaxRetries > 0` 时：
- 选号失败后，将失败账号 ID 加入 `ExcludeAccountIDs`
- 重新从剩余候选中选取
- 直到成功或重试次数耗尽（`ErrMaxRetriesExceeded`）

## 错误类型

| 错误 | 含义 |
|------|------|
| `ErrModelRequired` | 未指定 Model |
| `ErrNoAvailableProvider` | 无可用 Provider |
| `ErrNoAvailableAccount` | Provider 下无可用 Account |
| `ErrModelNotSupported` | 无 Provider 支持该 Model |
| `ErrProviderNotFound` | 指定 Provider 不存在 |
| `ErrMaxRetriesExceeded` | 重试次数耗尽 |
| `ErrOccupancyFull` | 所有账号并发已满 |
