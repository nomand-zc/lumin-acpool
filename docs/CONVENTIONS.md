# 编码规范

## 四大工程支柱

所有代码必须同时满足：

| 支柱 | 要求 | 检查清单 |
|------|------|----------|
| **高性能** | 热路径零分配、原子操作替代锁、避免不必要的 IO | ✅ Pick 路径无堆分配 ✅ Stats/Usage 用原子操作 ✅ 无阻塞式远端调用 |
| **高扩展** | 面向接口、策略可插拔、Options 可叠加 | ✅ 新功能不改核心代码 ✅ 新后端只需实现接口 ✅ 配置项用 With* 函数 |
| **高可读性** | 命名即文档、函数 ≤80 行、一个文件一个类型 | ✅ 无需注释即可理解意图 ✅ 无超长函数 ✅ 无混杂文件 |
| **单一职责** | 一个包/接口/函数只做一件事 | ✅ 包不越界 ✅ 接口不臃肿 ✅ 函数不兼职 |

> 详细原则解读 → [core-beliefs.md](design-docs/core-beliefs.md)

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

## 性能规范

| 规则 | 说明 |
|------|------|
| 热路径零分配 | Pick / Report 路径避免堆分配，用指针传递，复用 slice |
| 原子操作优先 | StatsStore / UsageStore 的 Incr 用原子操作，不用 Mutex |
| 本地优先 | 可本地计算的不走远端（UsageTracker 本地乐观计数） |
| 减少序列化 | 内存存储直接引用，避免不必要的 JSON marshal/unmarshal |

## 扩展规范

| 规则 | 说明 |
|------|------|
| 接口在消费端定义 | 存储子接口定义在 `storage/` 包，不在实现包 |
| 新功能零侵入 | 新增策略/后端/检查项只需实现接口 + 注入，不改核心代码 |
| 配置向后兼容 | 新增 Option 函数，不修改已有 Option 签名 |

## 可读性规范

| 规则 | 说明 |
|------|------|
| 函数 ≤ 80 行 | 超过则拆分为有意义的子步骤 |
| 一个文件一个核心类型 | `account.go` 只定义 Account，`status.go` 只定义 Status |
| 命名自解释 | 优先用清晰的命名代替注释，注释只解释 Why 不解释 What |
| 包级 doc.go | 每个包提供 `doc.go` 说明包的职责和核心概念 |

## 单一职责规范

| 规则 | 说明 |
|------|------|
| 包不越界 | 每个包只承担一个核心职责（balancer 调度、selector 选号、storage 存储） |
| 接口不臃肿 | 存储拆 6 个子接口，每个 3~6 个方法 |
| 结构体不兼职 | Account 只持数据，Balancer 只编排流程，算法封装在 Selector |

## 测试

- 测试文件 `_test.go`，与被测文件同包
- 运行：`go test ./...`
