# lumin-acpool
lumin-acpool 是 LUMIN 生态的**核心账号池与调度引擎**，为整个 lumin 技术栈提供统一的 AI 账号资源管理和高可用调度能力。

**核心作用**:
- **统一账号管理**: 聚合多平台 AI 模型账号（Kiro、GeminiCLI、Codex、iFlow 等）
- **智能调度引擎**: 提供负载均衡、熔断保护、冷却机制和健康检查
- **资源池化**: 将分散的账号资源统一池化管理，提高利用率和可靠性
- **韧性保障**: 内置完整的故障恢复和状态管理机制

**服务对象**:
- `lumin-proxy`: 代理网关层，处理 OpenAI/Anthropic 协议请求
- `lumin-desktop`: 本地代理应用，提供 API 接口
- `lumin-admin`: 管理控制台，提供池管理功能

## 技术栈
Golang 1.24 + mysql + redis + sqlite

## 核心文档

| 文档 | 路径 | 内容 |
|------|------|------|
| 架构设计 | [ARCHITECTURE.md](ARCHITECTURE.md) | 系统定位、调度流程、组件依赖图、状态机 |
| 编码规范 | [docs/CONVENTIONS.md](docs/CONVENTIONS.md) | 强制约束、命名规范 |
| 验收标准 | [docs/COMMIT_ACCEPTANCE.md](docs/COMMIT_ACCEPTANCE.md) | 覆盖率、通过率、pre-commit 门禁的完整验收条件 |
| Code Review 规范 | [docs/CODE_REVIEW.md](docs/CODE_REVIEW.md) | pre-push AI 代码审查机制、三级 checklist、报告格式 |
| 本地环境 | [docs/ENVIRONMENT.md](docs/ENVIRONMENT.md) | Docker 依赖启动、集成测试、基准测试、Makefile 命令速览 |

## 目录结构（一句话速览）

```
account/          聚合根：Account + ProviderInfo + Status + TrackedUsage + AccountStats
balancer/         负载均衡器：Pick（三种模式）+ ReportSuccess/Failure + 占用控制
  occupancy/      并发占用控制器（Unlimited / FixedLimit / AdaptiveLimit）
selector/         选号策略接口 + 内置策略（account 级 + group 级）
resolver/         服务发现层：从存储解析可用 Provider 和 Account
storage/          定义存储接口，以及后端存储驱动的接口实现（Memory / SQLite / MySQL / Redis）
  filtercond/     通用过滤条件表达式树
health/           健康检查编排器 + 内置检查项 + ReportHandler
  checks/         内置检查项（credential / probe / recovery / refresh / usage / usage_rules）
circuitbreaker/   熔断器：连续失败阈值 + 动态计算
cooldown/         冷却管理器：限流后暂停选号
usagetracker/     账号额度用量追踪管理, 同时也是做主动用量限流控制的依据，避免在使用账号时候被动发现账号的限流情况
cli/              命令行管理工具，主要用于方便快速测试账号管理的各个功能
```
