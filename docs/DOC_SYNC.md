# 文档同步规范 (DOC_SYNC)

本文档定义 lumin-acpool 的文档同步机制，作为 pre-push `go-doc-sync` 门禁的规范依据。

> **定位**：Code Review 的补充门禁，在 `go-code-review` 之后执行。专注代码变更引发的
> **文档漂移**（文档描述与实际行为不符），而非新增内容。

---

## 1. 触发机制

### 1.1 触发条件（满足任一即触发）

| 类型 | 条件 |
|------|------|
| 路径匹配 | 核心模块有变更（见第 2 节映射表） |
| Commit 前缀 | `feat:` `refactor:` `breaking:` `api:` `[docs-sync]` |

### 1.2 跳过条件（同时满足全部）

变更仅限 `docs/**`、`*_test.go`、`cli/**` 的任意组合，且未设置 `[docs-sync]` 前缀。

**例外**：`SKIP_DOC_SYNC=1` 强制跳过（仅限紧急情况，须在 PR 中注明）。

---

## 2. 文档映射表

文档按 harness engineering 四类职能组织，代码变更可能同时触及多类：

| 类型 | 职能 | 文档 | 触发路径 |
|------|------|------|--------|
| **inform** | 描述系统是什么、为什么这样设计 | `ARCHITECTURE.md` | 任意核心模块 |
| **inform** | 账号状态机 | `docs/design-docs/account-lifecycle.md` | `account/**` |
| **inform** | Pick 选号流程 | `docs/design-docs/pick-flow.md` | `balancer/**`, `selector/**` |
| **inform** | 健康检查系统 | `docs/design-docs/health-check.md` | `health/**` |
| **inform** | 存储层设计 | `docs/design-docs/storage.md` | `storage/**` |
| **inform** | 用量与冷却 | `docs/design-docs/usage-and-cooldown.md` | `usagetracker/**`, `circuitbreaker/**`, `cooldown/**` |
| **inform** | 新增策略指南 | `docs/design-docs/add-strategy.md` | `selector/**` |
| **constrain** | 文档编写质量规范（内容准入/格式/生命周期） | `docs/DOC_CONVENTIONS.md` | `docs/**` |
| **constrain** | 编码规范（命名/架构原则） | `docs/CONVENTIONS.md` | 任意 `*.go` |
| **constrain** | 测试规范（单元/集成/基准） | `docs/TESTING.md` | 任意 `*_test.go` |
| **constrain + verify** | 验收标准（覆盖率/门禁） | `docs/COMMIT_ACCEPTANCE.md` | `git-hooks/**`, `.pre-commit-config.yaml` |
| **constrain + feedback** | Code Review 规范与 checklist | `docs/CODE_REVIEW.md` | 任意核心模块 |
| **verify** | 本地环境与基准值 | `docs/ENVIRONMENT.md` | `go.mod`, `docker-compose.yml`, 核心路径性能变化 |
| **feedback** | Code Review agent 配置 | `.codebuddy/agents/code-review.md` | `docs/CODE_REVIEW.md` |

> 路径规则仅作初步筛选，agent 自主判断最终范围。

---

## 3. 更新 Checklist

**格式**：`[ ] 检查点 — 违规示例`

### 通用检查项（所有文档）

- [ ] 内容在其他文档中无重复块 — 若发现相同段落，用引用/链接替代；`doc-maintainer.md` 与 `DOC_SYNC.md` 间重复示例见本项目历史

### ARCHITECTURE.md

- [ ] 新增模块已反映在组件图中 — 新增 `occupancy/adaptive_limit.go` 但架构图无此条目
- [ ] Pick 六步流程描述与 `balancer/` 实现一致 — 步骤顺序变更但文档未更新
- [ ] 模块职责说明与实际代码职责一致 — Balancer 承担了 Selector 职责但文档未注明

### docs/design-docs/account-lifecycle.md

- [ ] 状态枚举与 `account.Status` 常量一致 — 新增状态值但文档仍是旧的 7 种
- [ ] 状态转换来源表与 `ReportFailure`/`HealthChecker` 触发逻辑一致

### docs/design-docs/pick-flow.md

- [ ] 三种调度模式描述与 `PickRequest.ProviderKey` 处理逻辑一致
- [ ] Failover/Retry 描述与 `defaultBalancer` 实现一致
- [ ] 错误类型表与实际返回的 `Err*` 变量一致

### docs/design-docs/health-check.md

- [ ] 内置检查项表与 `health/checks/` 目录一致（增删检查项需同步）
- [ ] 依赖链图与各检查项 `DependsOn()` 返回值一致

### docs/design-docs/storage.md

- [ ] 6 个子接口列表与 `storage/interface.go` 定义一致
- [ ] UpdateField 位掩码列表与 `storage.UpdateField*` 常量一致
- [ ] 4 种后端目录与 `storage/` 实际子目录一致

### docs/design-docs/usage-and-cooldown.md

- [ ] 冷却触发条件与 `cooldown/` + `ReportFailure` 逻辑一致
- [ ] 熔断阈值计算规则与 `circuitbreaker/` 实现一致

### docs/design-docs/add-strategy.md

- [ ] 新增策略流程与 `selector/strategies/` 注册机制一致

### docs/CONVENTIONS.md（constrain）

- [ ] 命名规范与实际项目模式一致（如 `default` 前缀惯例）

### docs/TESTING.md（constrain）

- [ ] 覆盖率阈值与 `git-hooks/run-tests.sh` 中 `COVERAGE_*_MIN` 配置一致
- [ ] Mock 工具规范（sqlmock / miniredis）与实际使用一致

### docs/COMMIT_ACCEPTANCE.md（constrain + verify）

- [ ] pre-commit hook 列表与 `.pre-commit-config.yaml` 实际 hook 一致
- [ ] 覆盖率阈值与 `git-hooks/run-tests.sh` 中 `COVERAGE_*_MIN` 配置一致
- [ ] 关键包排除列表与 `EXCLUDE_PKG_PREFIXES` 一致

### docs/CODE_REVIEW.md（constrain + feedback）

- [ ] Critical/Important checklist 与当前架构约束一致
- [ ] pre-push 门禁图与 `.pre-commit-config.yaml` 实际顺序一致

### docs/ENVIRONMENT.md（verify）

- [ ] 基准测试基线值反映当前性能水平（`BenchmarkPick_AutoMode` 等）
- [ ] 依赖工具版本与 `.pre-commit-config.yaml` / `go.mod` 一致
- [ ] Docker 服务配置与 `docker-compose.yml` 一致

### .codebuddy/agents/code-review.md（feedback）

- [ ] 审查检查项与 `docs/CODE_REVIEW.md` 中的 checklist 保持对齐

### docs/DOC_CONVENTIONS.md（constrain）

- [ ] 四类职能定义与 Harness Engineering 框架保持一致，无私自扩充
- [ ] "写入前核心判断"标准未被弱化或替换为更宽松的表述
- [ ] 大幅修改阈值数值（30 行 / 10 行 / 20%）与 `DOC_SYNC.md § 4.2` 保持一致

---

## 4. 内容约束

这是 agent 的核心行为准则，**优先级高于一切**。

> 完整内容约束规则（准入原则、禁止写入、修改门槛、大幅修改阈值）见
> [docs/DOC_CONVENTIONS.md § 2–4](DOC_CONVENTIONS.md#2-准入原则按职能写对的内容)，
> 本节仅作快速索引。

### 4.1 核心原则（摘要）

- 只写三类：**架构决策**（代码无法表达的 why）、**接口约束**（契约/状态机）、**流程全景**（跨模块协作）
- 禁止：实现细节 / 重复块 / 假设性内容 / 超过 3 层嵌套列表
- 修改前三问：事实错误？读代码无法替代？等长或更短？

### 4.2 复杂修改的执行原则

复杂修改直接执行，无需切换建议模式。完整规则见 [DOC_CONVENTIONS.md § 4.3](DOC_CONVENTIONS.md#43-复杂修改的执行原则)。

架构图（ASCII/Mermaid）改动需在报告中标注 `DIAGRAM_UPDATED` 并附说明。

---

## 5. 更新流程 SOP

```
Step 1  读取 DOC_SYNC_CONTEXT，确认受影响文档列表
Step 2  git diff 理解变更语义（聚焦接口/流程/状态变化，非实现细节）
Step 3  逐一读取受影响文档，对照第 3 节 checklist 判断漂移点
Step 4  对每处漂移：直接修改（复杂修改见 DOC_CONVENTIONS.md § 4.3）
Step 5  交叉验证 ARCHITECTURE.md 与 design-docs 描述不冲突
Step 6  生成 .doc-sync-report.md（见第 7 节格式）
```

> **注意**：大多数代码变更不需要修改文档，`NO_CHANGES` 是正常且期望的结果。

---

## 6. 豁免规则

以下情况不修改文档，但在报告中说明原因：

- 纯重构（逻辑行为不变，仅代码结构调整）
- 变更在 `testdata/`、`*_generated.go`、`*_test.go`
- 文档已包含该信息（不重复更新）
- 无法确定修改方向（报告标注 `UNCERTAIN`，给出分析）

---

## 7. 报告格式

```markdown
# Doc Sync Report

**Commit Range:** <base_sha>..<head_sha>
**Branch:** <branch>
**Timestamp:** <ISO8601>
**Triggered By:** <path-match: balancer/** | commit-prefix: feat:>

## Verdict: UPDATED | SKIPPED | NO_CHANGES

**Updated:** <n> | **No Changes:** <n>

---

## Updated
- [x] docs/design-docs/pick-flow.md — <一句话说明改了什么>
- [x] ARCHITECTURE.md — DIAGRAM_UPDATED: <说明>（如涉及架构图）

## No Changes Needed
- [ ] docs/design-docs/health-check.md — 未涉及

## Consistency Check
- OK | WARNING: <说明>

## Summary
<1-2 句技术评估>
```

---

## 8. 与现有门禁的关系

```
git push → pre-push hooks
              ├─ go-integration-test   (集成测试)
              ├─ go-code-review        (代码审查)
              └─ go-doc-sync ◄──── 本门禁（最后执行）
```

| 维度 | go-code-review | go-doc-sync |
|------|---------------|-------------|
| 触发脚本 | `run-code-review.sh` | `run-doc-sync.sh` |
| 规范文档 | `docs/CODE_REVIEW.md` | `docs/DOC_SYNC.md` |
| 报告文件 | `.code-review-report.md` | `.doc-sync-report.md` |
| 跳过机制 | `SKIP_CODE_REVIEW=1` | `SKIP_DOC_SYNC=1` |
| 手动触发 | 直接运行脚本 | `--manual` 标志 |
| 副作用 | 无 | 修改文档 + 创建 commit |
