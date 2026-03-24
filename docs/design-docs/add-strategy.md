# 新增选号策略指南

## 两级策略体系

| 级别 | 接口 | 职责 |
|------|------|------|
| **Group 级** | `GroupSelector` | 从候选 Provider 中选一个 |
| **Account 级** | `Selector` | 从候选 Account 中选一个 |

## 新增 Account 级策略

在 `selector/strategies/account/` 下创建文件，实现 `Selector` 接口：

```go
type MyStrategy struct{}

func (s *MyStrategy) Name() string { return "my_strategy" }

func (s *MyStrategy) Select(candidates []*account.Account, req *selector.SelectRequest) (*account.Account, error) {
    if len(candidates) == 0 {
        return nil, selector.ErrEmptyCandidates
    }
    // 策略逻辑
    return candidates[0], nil
}
```

注入：`balancer.WithSelector(accountstrategies.NewMyStrategy())`

## 新增 Group 级策略

在 `selector/strategies/group/` 下创建文件，实现 `GroupSelector` 接口。
注入：`balancer.WithGroupSelector(groupstrategies.NewMyGroupStrategy())`

## 亲和策略

亲和策略依赖 `AffinityStore` 维护 UserID → targetID 绑定。
单机用内存实现，集群部署需注入 Redis 实现共享绑定关系。

## 检查清单

- [ ] 实现 Selector 或 GroupSelector 接口
- [ ] Name() 返回唯一名称
- [ ] 空候选返回 ErrEmptyCandidates / ErrNoAvailableProvider
- [ ] 编写单元测试
