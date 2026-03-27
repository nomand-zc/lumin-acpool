# lumin-acpool

**AI 账号池 SDK**：为多平台 AI 模型账号提供负载均衡、熔断冷却、用量追踪和健康检查，实现高可用账号调度。

## 快速开始

- 📐 **架构设计** → [ARCHITECTURE.md](ARCHITECTURE.md)
- 📋 **编码规范** → [docs/CONVENTIONS.md](docs/CONVENTIONS.md)
- 🔍 **核心原则** → [docs/design-docs/core-beliefs.md](docs/design-docs/core-beliefs.md)
- 🎯 **内置策略** → [docs/references/strategies.md](docs/references/strategies.md)

## 项目结构

```
account/          Account 聚合根 + Status + Stats + TrackedUsage
balancer/         Pick 选号 + Report 上报 + 占用控制
selector/         两级选号策略（Provider级 + Account级）
resolver/         服务发现（解析可用 Provider/Account）
storage/          6 个子接口存储体系 + 4 种后端（内存/SQLite/MySQL/Redis）
health/           健康检查编排 + 6 种内置检查项
circuitbreaker/   熔断器（连续失败 + 动态阈值）
cooldown/         冷却管理
usagetracker/     用量追踪（本地乐观 + 远端校准）
cli/              命令行工具
docs/             设计文档和参考资料
```

## 验收标准

### 代码质量门禁
- ✅ **go fmt** - 代码格式化
- ✅ **golangci-lint** - 综合检查（goimports, go vet, deadcode 等）
- ✅ **go test -race** - 单元测试 + 竞态检测（覆盖率 ≥80%）
- ✅ **go mod tidy** - 依赖整理
- ✅ Markdown 格式检查、大文件检查

### 工程约束
1. **文件大小** ≤ 200 行（测试除外）- 超过则按单一职责拆分
2. **函数长度** ≤ 80 行 - 超过则拆分为有意义的子步骤
3. **一个文件一个核心类型** - `account.go` 只定义 Account
4. **接口优先于实现** - 所有核心组件接口优先
5. **零侵入扩展** - 新增策略/后端/检查项不改核心代码

### 四大工程支柱

| 支柱 | 要求 | 验证方式 |
|------|------|---------|
| **高性能** | Hot path 零分配、原子操作替代锁 | Pick 流程 <100ms 无堆分配 |
| **高扩展** | 面向接口、策略可插拔 | 新功能无需改核心代码 |
| **高可读性** | 命名即文档、函数短小 | 代码自解释，无需冗长注释 |
| **单一职责** | 包不越界、接口不臃肿 | 存储 6 个子接口、Account 只持数据 |

## Pre-commit 检查

```bash
# 安装 pre-commit
pip install pre-commit
pre-commit install

# 检查所有文件
pre-commit run --all-files

# 手动触发特定检查
pre-commit run go-vet --all-files
```

详见 [.pre-commit-config.yaml](.pre-commit-config.yaml)

## 核心 API

### Balancer - 调度中枢

```go
type Balancer interface {
    Pick(ctx context.Context, req *PickRequest) (*PickResult, error)
    ReportSuccess(ctx context.Context, accountID string) error
    ReportFailure(ctx context.Context, accountID string, callErr error) error
}
```

### 存储接口体系（6 个子接口）

```go
type Storage interface {
    account.AccountStorage         // Account CRUD + 字段更新
    account.ProviderStorage        // Provider CRUD
    account.StatsStore             // 统计数据（原子 Incr）
    usagetracker.UsageStore        // 用量追踪
    occupancy.OccupancyStore       // 并发占用控制
    affinity.AffinityStore         // 亲和绑定
}
```

## 关键文件

- **account/account.go** - Account 数据模型 + 7 种状态
- **balancer/balancer.go** - Balancer 接口定义
- **selector/selector.go** - 选号策略接口
- **resolver/resolver.go** - 服务发现接口
- **storage/interface.go** - 6 个子接口定义
- **health/checker.go** - 健康检查编排

## 测试

```bash
# 运行所有测试
go test ./... -race -cover

# 运行特定包的测试
go test ./balancer -race -v

# 生成覆盖率报告
go test ./... -race -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## 许可

MIT
