# Commit 验收标准

本文档定义代码提交前必须满足的所有验收条件，通过 pre-commit 自动化门禁执行。

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

## 3. Pre-commit 各项检查通过率

所有 pre-commit hook 必须 **100% 通过**，不得绕过（禁止使用 `--no-verify`）。

### 3.1 Go 代码质量

| Hook | 工具 | 要求 |
|------|------|------|
| `pretty-format-golang` | go fmt | 代码格式化，自动修复后无 diff |
| `golangci-lint` | golangci-lint v1.64.8 | 零 lint 错误，超时上限 5 分钟 |
| `go-test` | `git-hooks/run-tests.sh` | 测试全部通过（见第 2 节） |
| `go-mod-tidy` | go mod tidy | `go.mod` / `go.sum` 整洁，无多余依赖 |

> `golangci-lint` 不检查 `_test.go`、`_mock.go` 及文档文件，但测试文件本身须通过 `go-test`。

### 3.2 通用文件质量

| Hook | 要求 |
|------|------|
| `trailing-whitespace` | 无行尾空白 |
| `end-of-file-fixer` | 文件末尾保留一个换行符 |
| `check-added-large-files` | 单文件 ≤ 500 KB |
| `check-yaml` | YAML 文件语法合法 |
| `check-json` | JSON 文件语法合法 |
| `check-merge-conflict` | 无残留 merge conflict 标记 |

### 3.3 文档质量

| Hook | 要求 |
|------|------|
| `pymarkdown` | Markdown 格式合法，自动修复后无 diff |

## 4. 代码审查要求

提交前需满足以下编码规范（详见 [CONVENTIONS.md](CONVENTIONS.md)）：

- 命名遵循规范（包名小写、接口行为命名、实现以 `default` 前缀）
- 新增配置项使用 `With*` Functional Options 模式
- 测试文件与被测文件同包，不引入 testify
- Mock 存储统一使用 `storage/memory.NewStore()`

## 5. 覆盖率排除范围

以下包**不计入**全局覆盖率统计（在 `git-hooks/run-tests.sh` 的 `EXCLUDE_PKG_PREFIXES` 中配置）：

```
cli/    — 纯 CLI 工具，依赖交互式终端，无法单测
```

> `storage/mysql`、`storage/redis`、`storage/sqlite` 已通过 go-sqlmock / miniredis / SQLite `:memory:` 完成单测，**不再排除**，覆盖率纳入全局统计。

## 6. 跳过检查范围

以下目录/文件**免除** pre-commit 检查（在 `.pre-commit-config.yaml` 中配置）：

```
testdata/
.codebuddy/
docs/references/
*_generated.go
```

## 7. 本地执行

```bash
# 安装 pre-commit
pip install pre-commit
pre-commit install

# 手动对所有文件执行全部 hook
pre-commit run --all-files

# 仅执行测试 hook
pre-commit run go-test --all-files

# 仅执行 lint hook
pre-commit run golangci-lint --all-files
```

## 8. 违规处理

| 违规类型 | 处理方式 |
|----------|----------|
| 测试失败 | 提交被拒绝，必须修复后重新提交 |
| 覆盖率不足 | 提交被拒绝，补充测试用例后重新提交 |
| Lint 报错 | 提交被拒绝，修复代码问题后重新提交 |
| 格式问题 | Hook 自动修复，重新 `git add` 后提交 |
| 绕过 `--no-verify` | 视为违规，代码审查阶段强制回滚 |
