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

| 变更路径 | 需检查的文档 |
|---------|-----------|
| `account/**` | `docs/design-docs/account-lifecycle.md`, `ARCHITECTURE.md` |
| `balancer/**` | `docs/design-docs/pick-flow.md`, `ARCHITECTURE.md` |
| `selector/**` | `docs/design-docs/pick-flow.md`, `docs/design-docs/add-strategy.md` |
| `health/**` | `docs/design-docs/health-check.md` |
| `storage/**` | `docs/design-docs/storage.md`, `ARCHITECTURE.md` |
| `usagetracker/**` | `docs/design-docs/usage-and-cooldown.md` |
| `circuitbreaker/**` | `docs/design-docs/usage-and-cooldown.md` |
| `cooldown/**` | `docs/design-docs/usage-and-cooldown.md` |
| `docs/CODE_REVIEW.md` | `.codebuddy/agents/code-review.md` |

> 路径规则仅作初步筛选，agent 自主判断最终范围。

---

## 3. 更新 Checklist

**格式**：`[ ] 检查点 — 违规示例`

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

### docs/CONVENTIONS.md

- [ ] 命名规范与实际项目模式一致（如 `default` 前缀惯例）
- [ ] 测试规范中的覆盖率阈值与 `git-hooks/run-tests.sh` 配置一致

### docs/CODE_REVIEW.md

- [ ] Critical/Important checklist 与当前架构约束一致

### docs/ENVIRONMENT.md

- [ ] 基准测试基线值反映当前性能水平（`BenchmarkPick_AutoMode` 等）
- [ ] 依赖工具版本与 `.pre-commit-config.yaml` / `go.mod` 一致

### .codebuddy/agents/code-review.md

- [ ] 审查检查项与 `docs/CODE_REVIEW.md` 中的 checklist 保持对齐

---

## 4. 内容约束

这是 agent 的核心行为准则，**优先级高于一切**。

### 4.1 准入原则（只写三类）

| 类型 | 说明 |
|------|------|
| 架构决策 | 为什么这样设计，代码无法表达的 why |
| 接口约束 | 调用方必须知道的契约、状态机规则 |
| 流程全景 | 跨模块协作，读代码无法一目了然的 |

### 4.2 禁止写入

- 代码能直接表达的实现细节（字段类型、函数签名）
- 已有内容的换一种说法（无增量信息）
- 假设性内容（"可能"、"将来考虑"）
- 超过 3 层嵌套的列表

### 4.3 修改门槛（三条件须全部满足）

1. 代码变更导致现有文档**事实错误**或**流程漂移**
2. 该信息是**读代码无法替代**的（必须在文档中存在）
3. 修改后文档**等长或更短**（有等量删除才允许增长）

### 4.4 大幅修改阈值

满足以下任一条件 → 切换为**建议模式**，不直接修改：

| 条件 | 说明 |
|------|------|
| 文档 diff ≥ 30 行 | 单次修改行数过大 |
| 涉及架构图（ASCII/Mermaid） | 图形理解成本高 |
| 删除 > 10 行内容 | 高破坏性操作 |
| 新增 > 已有内容 20% | 防止内容爆发式增长 |

---

## 5. 更新流程 SOP

```
Step 1  读取 DOC_SYNC_CONTEXT，确认受影响文档列表
Step 2  git diff 理解变更语义（聚焦接口/流程/状态变化，非实现细节）
Step 3  逐一读取受影响文档，对照第 3 节 checklist 判断漂移点
Step 4  对每处漂移：判断修改幅度 → 直接修改 or 切建议模式
Step 5  交叉验证 ARCHITECTURE.md 与 design-docs 描述不冲突
Step 6  生成 .doc-sync-report.md（见第 7 节格式）
```

> **注意**：大多数代码变更不需要修改文档，`NO_CHANGES` 是正常且期望的结果。

---

## 6. 豁免规则

以下情况不修改文档，但在报告中说明原因：

- 纯重构（逻辑行为不变，仅代码结构调整）
- 变更在 `testdata/`、`*_generated.go`
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

## Verdict: UPDATED | SKIPPED | NO_CHANGES | SUGGESTIONS_ONLY

**Auto Updated:** <n> | **Suggestions:** <n> | **No Changes:** <n>

---

## Auto Updated
- [x] docs/design-docs/pick-flow.md — <一句话说明改了什么>

## Suggestions (Manual Review Required)
- [ ] ARCHITECTURE.md — <原因> + <建议修改位置和内容>

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
