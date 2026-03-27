# Commit 验收标准

本文档定义代码提交前必须满足的所有验收条件，通过 pre-commit 自动化门禁执行。

> 本地环境搭建、测试执行方式详见 [ENVIRONMENT.md](ENVIRONMENT.md)。

## 1. 单元测试覆盖率

| 指标 | 要求 |
|------|------|
| 全局行覆盖率 | ≥ 80% |
| 关键调度路径（balancer / selector / resolver / storage） | ≥ 90% |

> 覆盖率由 `git-hooks/run-tests.sh` 在测试完成后自动解析，低于阈值时脚本以非零状态退出，pre-commit 拒绝本次提交。
> 阈值常量定义在脚本顶部（`COVERAGE_GLOBAL_MIN` / `COVERAGE_CRITICAL_MIN`），如需调整直接修改脚本。

## 2. 测试通过率

| 指标 | 要求 |
|------|------|
| 单元测试通过率 | 100%（零失败） |
| 竞态检测（-race） | 必须通过，零数据竞争 |
| 测试重复次数（-count） | `count=1`，禁止依赖缓存结果 |

执行命令（由 `git-hooks/run-tests.sh` 完整实现）：

```bash
# 测试 + race 检测 + 覆盖率采集 + 阈值校验 一次性完成
go test -race -count=1 -coverprofile=coverage.out -covermode=atomic ./...
go tool cover -func=coverage.out   # 查看各函数覆盖率明细
```

## 3. 集成测试

集成测试**纳入 pre-push 门禁**，由 `git-hooks/run-integration-tests.sh` 在每次 `git push` 前自动执行。

**行为**：

- Docker 可用时：自动 `make env-up` 启动依赖 → 运行集成测试 → 始终 `make env-down` 清理容器（无论成功失败）
- Docker 不可用时：打印警告并放行（软性降级），需开发者手动补跑

**激活方式**（需在 `pre-commit install` 基础上额外执行）：

```bash
pre-commit install --hook-type pre-push
```

**手动执行**：

```bash
# 本地：启动依赖 → 运行 → 清理（使用 Makefile 封装）
make test-all

# 手动执行（支持环境变量注入）
MYSQL_DSN="acpool:acpool123@tcp(localhost:3306)/lumin_acpool_test?parseTime=true" \
REDIS_ADDR="localhost:6379" \
go test -tags=integration -race -count=1 -v ./...
```

**覆盖范围**：集成测试文件以 `_integration_test.go` 结尾，使用 `//go:build integration` 构建标签与单元测试隔离，不计入单元测试覆盖率统计。

## 4. 基准测试性能确认

基准测试**不纳入自动门禁**（不同机器硬件差异导致误报率高），但在以下场景**必须手动确认无性能回归**：

- 修改 `balancer/`、`selector/`、`resolver/` 核心调度路径后
- **push 前须手动跑基准，并将对比结果附到 PR 描述**，由代码审查确认

**性能回归阈值**：核心路径（Pick、Selector、OccupancyController）**性能不得下降超过 5%**。

**执行方式**：

```bash
# 修改代码前保存基线
make bench-pkg PKG=./balancer/... > before.txt
# 修改代码...
make bench-pkg PKG=./balancer/... > after.txt
benchstat before.txt after.txt   # go install golang.org/x/perf/cmd/benchstat@latest
```

**PR 描述要求**：修改核心路径时，PR 描述中必须包含 `benchstat` 对比输出，回归超过 5% 需在描述中说明原因并获得审查者明确批准。

**已验证基准值**（Memory 后端，作为性能回归基线，详见 [ENVIRONMENT.md](ENVIRONMENT.md)）：

| 路径 | 基准名 | ns/op 基线 |
|------|--------|-----------|
| Pick 全链路 | `BenchmarkPick_AutoMode` | ~6,100 |
| Pick 精确模式 | `BenchmarkPick_ExactMode` | ~3,900 |
| ReportSuccess 热路径 | `BenchmarkReportSuccess_NoStatsStore` | ~6 |

## 5. Pre-commit 各项检查通过率

所有 pre-commit hook 必须 **100% 通过**，不得绕过（禁止使用 `--no-verify`）。

### 5.1 Go 代码质量

| Hook | 工具 | 要求 |
|------|------|------|
| `pretty-format-golang` | go fmt | 代码格式化，自动修复后无 diff |
| `golangci-lint` | golangci-lint v1.64.8 | 零 lint 错误，超时上限 5 分钟 |
| `go-test` | `git-hooks/run-tests.sh` | 测试全部通过（见第 2 节） |
| `go-integration-test` | `git-hooks/run-integration-tests.sh` | 集成测试全部通过（pre-push，见第 3 节） |
| `go-mod-tidy` | go mod tidy | `go.mod` / `go.sum` 整洁，无多余依赖 |

> `golangci-lint` 不检查 `_test.go`、`_mock.go` 及文档文件，但测试文件本身须通过 `go-test`。

### 5.2 通用文件质量

| Hook | 要求 |
|------|------|
| `trailing-whitespace` | 无行尾空白 |
| `end-of-file-fixer` | 文件末尾保留一个换行符 |
| `check-added-large-files` | 单文件 ≤ 500 KB |
| `check-yaml` | YAML 文件语法合法 |
| `check-json` | JSON 文件语法合法 |
| `check-merge-conflict` | 无残留 merge conflict 标记 |

### 5.3 文档质量

| Hook | 要求 |
|------|------|
| `pymarkdown` | Markdown 格式合法，自动修复后无 diff |

## 6. 代码审查要求

提交前需满足以下编码规范（详见 [CONVENTIONS.md](CONVENTIONS.md)）：

- 命名遵循规范（包名小写、接口行为命名、实现以 `default` 前缀）
- 新增配置项使用 `With*` Functional Options 模式
- 测试文件与被测文件同包，不引入 testify
- Mock 存储统一使用 `storage/memory.NewStore()`
- 修改核心调度路径（`balancer/`、`selector/`、`resolver/`）时，PR 描述须附基准对比结果（见第 4 节）

## 7. 覆盖率排除范围

以下包**不计入**全局覆盖率统计（在 `git-hooks/run-tests.sh` 的 `EXCLUDE_PKG_PREFIXES` 中配置）：

```
cli/    — 纯 CLI 工具，依赖交互式终端，无法单测
```

> `storage/mysql`、`storage/redis`、`storage/sqlite` 已通过 go-sqlmock / miniredis / SQLite `:memory:` 完成单测，**不再排除**，覆盖率纳入全局统计。

## 8. 跳过检查范围

以下目录/文件**免除** pre-commit 检查（在 `.pre-commit-config.yaml` 中配置）：

```
testdata/
.codebuddy/
docs/references/
*_generated.go
```

## 9. 本地执行

```bash
# 安装 pre-commit
pip install pre-commit

# 安装 pre-commit hook（每次 commit 前触发）
pre-commit install

# 安装 pre-push hook（每次 push 前触发，含集成测试）
pre-commit install --hook-type pre-push

# 手动对所有文件执行全部 pre-commit hook
pre-commit run --all-files

# 手动触发集成测试 hook
pre-commit run go-integration-test --hook-stage pre-push --all-files

# 仅执行单元测试 hook
pre-commit run go-test --all-files

# 仅执行 lint hook
pre-commit run golangci-lint --all-files
```

## 10. 违规处理

| 违规类型 | 处理方式 |
|----------|----------|
| 测试失败 | 提交被拒绝，必须修复后重新提交 |
| 覆盖率不足 | 提交被拒绝，补充测试用例后重新提交 |
| Lint 报错 | 提交被拒绝，修复代码问题后重新提交 |
| 格式问题 | Hook 自动修复，重新 `git add` 后提交 |
| 绕过 `--no-verify` | 视为违规，代码审查阶段强制回滚 |
