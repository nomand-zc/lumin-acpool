# 编码规范

## 命名规范

| 类别 | 规范 | 示例 |
|------|------|------|
| 包名 | 小写单词 | `balancer`, `selector` |
| 接口 | 行为命名，无 I 前缀 | `Balancer`, `Selector` |
| 实现 | 小写私有 + `default` 前缀 | `defaultBalancer` |
| 常量 | CamelCase | `StatusAvailable`, `UpdateFieldStatus` |
| 文件名 | 小写下划线 | `default_balancer.go` |

## 架构原则

- 遵循 SOLID、GRASP 和 YAGNI 原则
- 接口约束高于实现约束
- 配置使用 `With*` Functional Options，新增 Option 不修改已有签名
- `Account` 只持数据，`Balancer` 只编排流程，算法封装在 `Selector`
- 非必要不导出

---

> 测试规范（单元测试 / 集成测试 / 基准测试）见 [docs/TESTING.md](TESTING.md)。
