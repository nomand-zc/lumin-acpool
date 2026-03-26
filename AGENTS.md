# AGENTS.md — lumin-acpool

> 这是一张地图，不是说明书。详细内容请跟随链接。

## 这是什么

AI 账号池（Account Pool）SDK，为多平台 AI 模型账号提供负载均衡、熔断冷却、用量追踪和健康检查能力。
模块路径: `github.com/nomand-zc/lumin-acpool` · Go 1.24.11 · 依赖 `lumin-client`

## 快速导航

| 你想做什么 | 去哪里看 |
|-----------|---------|
| 理解整体架构和数据流 | → [ARCHITECTURE.md](ARCHITECTURE.md) |
| 了解核心设计原则 | → [docs/design-docs/core-beliefs.md](docs/design-docs/core-beliefs.md) |
| 理解选号流程（Pick 三种模式） | → [docs/design-docs/pick-flow.md](docs/design-docs/pick-flow.md) |
| 理解账号状态机和恢复机制 | → [docs/design-docs/account-lifecycle.md](docs/design-docs/account-lifecycle.md) |
| 理解用量追踪和冷却机制 | → [docs/design-docs/usage-and-cooldown.md](docs/design-docs/usage-and-cooldown.md) |
| 理解健康检查系统 | → [docs/design-docs/health-check.md](docs/design-docs/health-check.md) |
| 理解存储层和多后端设计 | → [docs/design-docs/storage.md](docs/design-docs/storage.md) |
| 新增一种选号策略 | → [docs/design-docs/add-strategy.md](docs/design-docs/add-strategy.md) |
| 查看内置选号策略列表 | → [docs/references/strategies.md](docs/references/strategies.md) |
| 查看集群部署注意事项 | → [docs/references/cluster-deployment.md](docs/references/cluster-deployment.md) |
| 查看编码规范和约定 | → [docs/CONVENTIONS.md](docs/CONVENTIONS.md) |

## 目录结构（一句话速览）

```
account/          → 聚合根：Account + ProviderInfo + Status + TrackedUsage + AccountStats
balancer/         → 负载均衡器：Pick（三种模式）+ ReportSuccess/Failure + 占用控制
  occupancy/      → 并发占用控制器（Unlimited / FixedLimit / AdaptiveLimit）
selector/         → 选号策略接口 + 内置策略（account 级 + group 级）
resolver/         → 服务发现层：从存储解析可用 Provider 和 Account
storage/          → 存储接口（6 个子接口）+ 4 种后端（Memory / SQLite / MySQL / Redis）
  filtercond/     → 通用过滤条件表达式树
health/           → 健康检查编排器 + 内置检查项 + ReportHandler
  checks/         → 6 种内置检查项（credential / probe / recovery / refresh / usage / usage_rules）
circuitbreaker/   → 熔断器：连续失败阈值 + 动态计算
cooldown/         → 冷却管理器：限流后暂停选号
usagetracker/     → 用量追踪器：本地乐观计数 + 远端校准
cli/              → 命令行管理工具
```

## 工程原则（贯穿所有开发）

- **高性能**：热路径零分配、原子操作替代锁、高低频数据分离、本地乐观计数减少远端请求
- **高扩展**：接口驱动 + Strategy 模式 + Functional Options，新增策略/后端/检查项无需改动核心代码
- **高可读性**：一个文件做一件事、函数短小聚焦、命名即文档、包级 `doc.go` 说明职责
- **单一职责**：每个包/接口/结构体只承担一个职责 → 详见 [core-beliefs.md](docs/design-docs/core-beliefs.md)

## 关键约束（必须遵守）

1. **Account 是聚合根**，所有状态变更必须通过 `UpdateAccount(fields)` 按位掩码部分更新，**禁止全量覆盖**
2. **AccountStats 独立存储**，高频原子更新通过 `StatsStore`，不与 Account 全量写竞争
3. **Pick 返回的 Account 是深拷贝**，调用者持有的引用不影响池内状态
4. **ReportSuccess/Failure 必须在调用完成后调用**，它负责释放占用槽位 + 驱动状态机
5. **乐观锁**：`Account.Version` 每次 Update 递增，集群部署时防止并发覆盖

## 文档索引

```
docs/
├── design-docs/                     ← 设计文档
│   ├── index.md                     ← 设计文档总览
│   ├── core-beliefs.md              ← 核心设计原则
│   ├── pick-flow.md                 ← Pick 选号流程（三种模式 + Failover + Retry）
│   ├── account-lifecycle.md         ← 账号状态机（7 种状态 + 转换）
│   ├── usage-and-cooldown.md        ← 用量追踪 + 冷却机制
│   ├── health-check.md              ← 健康检查系统
│   ├── storage.md                   ← 存储层设计（6 个子接口 + 4 种后端）
│   └── add-strategy.md              ← 新增选号策略指南
├── references/                      ← 参考资料
│   ├── strategies.md                ← 内置策略一览
│   └── cluster-deployment.md        ← 集群部署注意事项
└── CONVENTIONS.md                   ← 编码规范
```

## Pre-commit 配置

项目使用 [pre-commit](https://pre-commit.com/) 进行代码提交前的自动化质量检查，确保代码符合项目的统一规范。

### 安装

```bash
# 检查是否已安装 pre-commit
pre-commit --version

# 如果未安装，使用以下命令安装：
# 方式一：使用 pip（推荐）
pip install pre-commit

# 方式二：使用 Homebrew（macOS）
brew install pre-commit

# 方式三：使用 apt（Ubuntu/Debian）
sudo apt install pre-commit

# 安装完成后，在项目根目录安装 git hooks
pre-commit install
```

### 检查项说明

| 类别 | 检查项 | 说明 |
|------|--------|------|
| **Go 代码** | `go fmt` | 格式化 Go 代码 |
| | `golangci-lint` | 综合代码检查（含 goimports, go vet） |
| | `go test` | **单元测试门禁**（-race 竞态检测 + 覆盖率） |
| | `go mod tidy` | 依赖整理 |
| **通用检查** | `trailing-whitespace` | 行尾空白检查 |
| | `end-of-file-fixer` | 文件末尾换行检查 |
| | `check-added-large-files` | 大文件检查（≤500KB） |
| | `check-yaml` / `check-json` | YAML/JSON 语法检查 |
| | `check-merge-conflict` | 合并冲突标记检查 |
| **文档** | `pymarkdown` | Markdown 格式化 |

### 使用方式

```bash
# 对所有文件运行检查
pre-commit run --all-files

# 只检查暂存的文件（推荐）
pre-commit run

# 手动触发特定检查
pre-commit run go-vet --all-files
```

### 跳过检查

**仅在紧急情况下使用**：

```bash
# 跳过所有检查提交（不推荐）
git commit --no-verify
```

### 配置文件

详细配置见 [.pre-commit-config.yaml](.pre-commit-config.yaml)
