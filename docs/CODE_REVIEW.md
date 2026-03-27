# Code Review 规范

本文档定义 lumin-acpool 的代码审查机制，作为 pre-push 自动化门禁的规范依据。

> **与其他门禁的关系**：Code Review 是 pre-push 阶段的最后一道门禁，位于集成测试
> (`go-integration-test`) 之后执行。其目的是捕获自动化工具无法检测的问题——架构合规性、
> 业务逻辑正确性、设计意图符合度。

---

## 1. 机制说明

### 1.1 触发时机

| 阶段 | 触发方式 | 说明 |
|------|----------|------|
| `pre-push` | `git push` 时自动触发 | 由 `git-hooks/run-code-review.sh` 调用 review agent |
| 手动 | `pre-commit run go-code-review --hook-stage pre-push --all-files` | 主动触发完整审查 |

### 1.2 审查范围

- **仅审查差异代码**：`git diff origin/<branch>..HEAD` 产生的变更内容
- **不重复检查**：golangci-lint、格式化、测试覆盖率已由 pre-commit 其他 hook 保障，review 不重复覆盖
- **排除自动生成文件**：`*_generated.go`、`testdata/`、`docs/references/`

### 1.3 审查结果

Review agent 将结果写入项目根目录的 `.code-review-report.md`（已加入 `.gitignore`）。

| 字段 | 说明 |
|------|------|
| `verdict` | `PASS` 或 `FAIL`，决定 push 是否放行 |
| `critical_count` | Critical 级问题数量，任何 Critical 问题均导致 FAIL |
| `important_count` | Important 级问题数量，≥ 1 个 Important 问题导致 FAIL |
| `minor_count` | Minor 级问题数量，仅作提示，不阻断 push |

### 1.4 失败处理流程

```
push FAIL
  ├─ 打印报告摘要（Critical + Important 问题列表）
  ├─ 指出报告完整路径（.code-review-report.md）
  └─ 自动创建修复子任务（CodeBuddy agent 接管修复工作）
       └─ 修复完成后重新 push，再次触发 review
```

---

## 2. Review Checklist

### 2.1 Critical（任何一条触发 → FAIL）

这类问题会直接导致功能错误、数据损坏或安全漏洞，**必须在 push 前修复**。

#### 架构合规性

- [ ] **接口边界遵守**：`Account` 只持数据，`Balancer` 只编排流程，算法封装在 `Selector`，不得越层调用
- [ ] **存储接口使用**：业务逻辑只通过 `storage/` 定义的接口访问数据，不得直接引用后端实现（Memory/MySQL/Redis）
- [ ] **OccupancyController 原子性**：`FilterAvailable` 和 `Acquire` 必须保持原子语义，不得在二者之间插入其他逻辑
- [ ] **Pick 流程完整性**：修改 Pick 路径时（`balancer/`），六步流程（发现供应商→选取供应商→发现账号→并发过滤→选取账号→占用槽位）必须保持完整
- [ ] **状态机约束**：账号状态转换必须符合已定义的生命周期（Available/CoolingDown/CircuitOpen/HalfOpen/Expired/Invalidated/Banned/Disabled），不得引入非法状态转换
- [ ] **并发安全**：涉及共享状态的代码必须有正确的锁保护或使用 `sync/atomic`，不得引入数据竞争

#### 业务逻辑正确性

- [ ] **账号可用性判断**：选号前必须同时满足三个条件：状态为 `Available`、并发控制以内、无激活限流策略
- [ ] **错误传播**：关键路径的错误必须向上传播，不得静默吞掉（如 `Acquire` 失败必须返回错误）
- [ ] **资源释放**：`OccupancyController.Acquire()` 成功后，无论后续成功或失败均必须调用 `Release()`
- [ ] **熔断触发逻辑**：`CircuitBreaker` 阈值修改必须基于现有计算规则（连续失败次数 + 用量规则动态计算），不得绕过

#### 接口兼容性

- [ ] **存储接口变更**：修改 `storage/` 中任意子接口（AccountStorage/ProviderStorage 等）时，所有后端实现（Memory/SQLite/MySQL/Redis）必须同步更新
- [ ] **Functional Options 兼容性**：新增配置项必须以 `With*` 模式追加，不得修改已有函数签名
- [ ] **公开 API 兼容性**：对外暴露的接口类型修改无需向后兼容，按照最优方案执行即可

---

### 2.2 Important（累计 ≥ 1 条触发 → FAIL）

这类问题影响代码质量、可维护性或测试完整性，**应在 push 前修复**。

#### 代码设计

- [ ] **命名规范**：包名小写、接口行为命名（无 `I` 前缀）、实现使用 `default` 前缀，常量 CamelCase
- [ ] **非必要导出**：不需要跨包访问的类型/函数/变量应为小写私有，避免过度暴露
- [ ] **职责单一**：新增文件/函数应只承担一个明确的职责，发现"上帝函数"（>100行核心逻辑）须拆分
- [ ] **重复逻辑**：发现可复用的重复代码块（>15行相似逻辑出现 ≥2 处）须提取公共函数
- [ ] **魔法数字/字符串**：关键数值常量须定义为具名常量，不得在逻辑代码中硬编码

#### 错误处理

- [ ] **错误信息质量**：错误信息须携带上下文（账号ID、provider名称等），不得仅返回 `"error occurred"`
- [ ] **错误类型正确性**：区分 `可重试错误`（网络超时、临时限流）和 `不可重试错误`（凭证失效、永久封禁），不得混淆
- [ ] **Panic 禁止**：生产路径中禁止使用 `panic`，必须转换为错误返回

#### 测试覆盖

- [ ] **新增逻辑有对应测试**：新增业务函数必须有对应的表驱动单元测试，不得仅有 happy path
- [ ] **边界用例覆盖**：空列表、零值、最大值等边界场景必须有测试用例
- [ ] **Mock 使用规范**：存储层 mock 统一使用 `storage/memory.NewStore()`，不得自行构造 fake struct
- [ ] **测试文件同包**：测试文件与被测文件同目录同包，不使用 `_test` 包后缀（黑盒测试除外）

#### 性能影响

- [ ] **核心路径无不必要分配**：Pick 路径（`balancer/`、`selector/`、`resolver/`）不得引入无谓的堆分配
- [ ] **无 N+1 查询**：循环中不得出现针对存储的重复查询，应批量获取
- [ ] **网络调用优化**：在保证数据实时性的前提下，尽可能的较少网络调用的频率，能批量优先批量，避免在大循环中调用有网络请求的接口

---

### 2.3 Minor（仅提示，不阻断 push）

这类问题是改进建议，**记录在报告中，开发者可在后续迭代中处理**。

#### 可读性

- [ ] 复杂逻辑缺少注释说明意图（注意：不需要解释"做了什么"，而是"为什么这样做"）
- [ ] 函数参数过多（>5个）可考虑引入 Options 结构体
- [ ] 变量名过于简短（单字母除循环变量外）

#### 可观测性

- [ ] 关键状态变更缺少日志记录（账号状态转换、熔断触发/恢复）
- [ ] 错误路径缺少足够的上下文信息便于排查

#### 文档同步

- [ ] 修改了架构或行为但未更新对应设计文档（`docs/design-docs/`、`ARCHITECTURE.md`）
- [ ] 新增的公开接口缺少 godoc 注释

---

## 3. 报告格式

Review agent 输出的 `.code-review-report.md` 格式如下：

```markdown
# Code Review Report

**Commit Range:** <base_sha>..<head_sha>
**Reviewed Files:** <变更文件数>
**Branch:** <branch_name>
**Timestamp:** <ISO8601 时间>

## Verdict: PASS | FAIL

**Critical:** <n> | **Important:** <n> | **Minor:** <n>

---

## Critical Issues

### [C1] <问题标题>
- **File:** `path/to/file.go:line`
- **Problem:** <问题描述>
- **Impact:** <影响说明>
- **Fix:** <修复建议>

---

## Important Issues

### [I1] <问题标题>
- **File:** `path/to/file.go:line`
- **Problem:** <问题描述>
- **Impact:** <影响说明>
- **Fix:** <修复建议>

---

## Minor Issues

### [M1] <问题标题>
- **File:** `path/to/file.go:line`
- **Suggestion:** <改进建议>

---

## Strengths

- <做得好的地方，具体到文件/行>

---

## Summary

<1-3句话的技术评估>
```

---

## 4. 豁免与例外

以下情况可绕过 code review 门禁（仍须满足其他所有 pre-commit 检查）：

| 场景 | 方式 | 要求 |
|------|------|------|
| 纯文档修改（仅 `.md` 文件变更） | 自动豁免 | 脚本检测到无 `.go` 文件变更时跳过 |
| 紧急热修复 | `SKIP_CODE_REVIEW=1 git push` | 需在 PR 描述中注明原因，事后补 review |
| 自动生成文件（`*_generated.go`） | 自动排除 | 不计入 review 范围 |

> 禁止将 `SKIP_CODE_REVIEW=1` 作为常态，仅限真正的紧急情况。滥用视为违规。

---

## 5. 与现有门禁的关系

```
git commit → pre-commit hooks
               ├─ pretty-format-golang  (格式化)
               ├─ golangci-lint          (静态分析)
               ├─ go-test                (单元测试 + 覆盖率)
               └─ go-mod-tidy            (依赖整理)

git push   → pre-push hooks
               ├─ go-integration-test    (集成测试)
               ├─ go-code-review ◄──── 本门禁
               └─ go-doc-sync            (文档同步，最后执行)
```

Code Review 在集成测试通过后执行，专注于**人类视角的问题**：
架构符合度、业务逻辑正确性、设计决策合理性。

---

## 6. 参考文档

| 文档 | 说明 |
|------|------|
| [CONVENTIONS.md](CONVENTIONS.md) | 编码规范（命名、架构原则、测试规范）|
| [COMMIT_ACCEPTANCE.md](COMMIT_ACCEPTANCE.md) | 完整验收标准（覆盖率、测试通过率等）|
| [ARCHITECTURE.md](../ARCHITECTURE.md) | 架构设计（Pick 流程、组件职责、存储层）|
| [design-docs/account-lifecycle.md](design-docs/account-lifecycle.md) | 账号状态机定义 |
