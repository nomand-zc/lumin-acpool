# 编码规范

## 命名

| 类别 | 规范 | 示例 |
|------|------|------|
| 包名 | 小写单词 | `balancer`, `selector`, `usagetracker` |
| 接口 | 行为命名，无 I 前缀 | `Balancer`, `Selector`, `CircuitBreaker` |
| 实现 | 小写私有 + default 前缀 | `defaultBalancer`, `defaultChecker` |
| 常量 | CamelCase | `StatusAvailable`, `UpdateFieldStatus` |
| 文件名 | 小写下划线 | `default_balancer.go`, `filter_cond.go` |

## 设计模式

| 模式 | 使用场景 |
|------|---------|
| Functional Options | Balancer / CircuitBreaker / Cooldown / UsageTracker 构造器 |
| Strategy | Selector / GroupSelector / OccupancyController |
| Builder | filtercond.Builder |
| 位掩码 | UpdateField 部分更新 |
| 乐观锁 | Account.Version 并发控制 |
| 聚合根 | Account 作为领域聚合根 |

## 测试

- 测试文件 `_test.go`，与被测文件同包
- 使用 `github.com/stretchr/testify` 断言（go.sum 中未直接依赖，部分测试使用标准库）
- 运行：`go test ./...`
