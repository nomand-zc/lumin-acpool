#!/usr/bin/env bash
set -euo pipefail

# ==============================================================================
# pre-push Code Review 门禁
#
# 行为：
#   - 纯文档变更（无 .go 文件）：自动 PASS，不调用 agent
#   - SKIP_CODE_REVIEW=1：强制跳过（仅限紧急情况，须在 PR 中注明）
#   - codebuddy CLI 不可用：打印警告并放行（软性降级）
#   - review PASS：放行 push
#   - review FAIL：阻断 push，打印摘要，创建修复任务
# ==============================================================================

REPORT_PATH=".code-review-report.md"
CURRENT_BRANCH=$(git symbolic-ref --short HEAD 2>/dev/null || echo "HEAD")
HEAD_SHA=$(git rev-parse HEAD)
HEAD_SHA_SHORT=$(git rev-parse --short HEAD)

# ------------------------------------------------------------------------------
# Step 1: 检查是否强制跳过
# ------------------------------------------------------------------------------
if [[ "${SKIP_CODE_REVIEW:-0}" == "1" ]]; then
  echo "WARN: Code review skipped via SKIP_CODE_REVIEW=1"
  echo "      This is only acceptable for emergency hotfixes."
  echo "      Please document the reason in your PR description."
  exit 0
fi

# ------------------------------------------------------------------------------
# Step 2: 确定 review 的 base SHA
# 优先使用 origin/<branch>，回退到上一个 commit
# ------------------------------------------------------------------------------
if git rev-parse "origin/${CURRENT_BRANCH}" &>/dev/null 2>&1; then
  BASE_SHA=$(git rev-parse "origin/${CURRENT_BRANCH}")
else
  # 新分支或首次 push：与上一个 commit 比较
  BASE_SHA=$(git rev-parse HEAD~1 2>/dev/null || git rev-parse --root)
fi

# ------------------------------------------------------------------------------
# Step 3: 检测是否有 Go 文件变更（无 Go 变更则自动跳过）
# ------------------------------------------------------------------------------
CHANGED_GO_FILES=$(git diff --name-only "${BASE_SHA}".."${HEAD_SHA}" \
  | grep -E '\.go$' \
  | grep -v -E '(_generated\.go$|^testdata/|^docs/references/)' \
  || true)

if [[ -z "$CHANGED_GO_FILES" ]]; then
  echo "==> Code review skipped: no Go file changes detected."
  exit 0
fi

CHANGED_GO_COUNT=$(echo "$CHANGED_GO_FILES" | wc -l | tr -d ' ')
echo "==> Code review: ${CHANGED_GO_COUNT} Go file(s) changed, starting review..."

# ------------------------------------------------------------------------------
# Step 4: 检测 codebuddy CLI 可用性
# ------------------------------------------------------------------------------
if ! command -v codebuddy &>/dev/null; then
  echo "WARN: codebuddy CLI not found, skipping AI code review."
  echo "      Install codebuddy CLI or run review manually:"
  echo "      https://cnb.cool/codebuddy/codebuddy-code"
  exit 0
fi

# ------------------------------------------------------------------------------
# Step 5: 清理旧报告，注入环境变量，调用 review agent
# ------------------------------------------------------------------------------
rm -f "$REPORT_PATH"

echo "==> Invoking code-review agent (base: ${BASE_SHA:0:8}, head: ${HEAD_SHA_SHORT})..."

export REVIEW_BASE_SHA="$BASE_SHA"
export REVIEW_HEAD_SHA="$HEAD_SHA"
export REVIEW_BRANCH="$CURRENT_BRANCH"
export REVIEW_REPORT_PATH="$REPORT_PATH"

# 构造 review prompt（包含完整的 diff 内容），写入临时文件避免 shell 特殊字符问题
PROMPT_FILE=$(mktemp /tmp/code-review-prompt.XXXXXX.md)
trap 'rm -f "$PROMPT_FILE"' EXIT

{
  cat .codebuddy/agents/code-review.md
  echo ""
  echo "---"
  echo "## 当前审查任务"
  echo ""
  echo "base_sha: ${REVIEW_BASE_SHA}"
  echo "head_sha: ${REVIEW_HEAD_SHA}"
  echo "branch: ${REVIEW_BRANCH}"
  echo "report_path: ${REVIEW_REPORT_PATH}"
  echo ""
  echo "变更的 Go 文件（共 ${CHANGED_GO_COUNT} 个）："
  echo "${CHANGED_GO_FILES}"
  echo ""
  echo "## Git Diff"
  echo ""
  echo '```diff'
  git diff --unified=5 "${BASE_SHA}".."${HEAD_SHA}" -- '*.go' \
    | grep -v -E '(_generated\.go|testdata/|docs/references/)' || true
  echo '```'
  echo ""
  echo "请严格按照 agent 定义中的报告格式输出审查结果，将完整报告写入 ${REVIEW_REPORT_PATH}。"
  echo "最终报告必须包含 '## Verdict: PASS' 或 '## Verdict: FAIL' 行。"
} > "$PROMPT_FILE"

# 调用 codebuddy CLI 执行审查，将输出直接写入报告文件
# 使用 --print -y 实现非交互式调用，通过管道传入 prompt
codebuddy --print -y < "$PROMPT_FILE" > "$REPORT_PATH" 2>&1 || {
    echo "WARN: Code review agent exited with error, skipping review."
    echo "      Check codebuddy configuration and retry manually."
    exit 0
  }

# ------------------------------------------------------------------------------
# Step 6: 解析报告中的 verdict
# ------------------------------------------------------------------------------
if [[ ! -f "$REPORT_PATH" ]]; then
  echo "WARN: Review report not generated, skipping review gate."
  exit 0
fi

# 从报告中提取 verdict（支持多种格式）
# 1. 标准格式：## Verdict: PASS/FAIL（含加粗变体 **PASS**/**FAIL**）
# 2. 中文格式：审查结果：`FAIL`
# 3. 通用兜底：Verdict 行
VERDICT="UNKNOWN"
if grep -qE '^## Verdict:.*PASS' "$REPORT_PATH" 2>/dev/null; then
  VERDICT="PASS"
elif grep -qE '^## Verdict:.*FAIL' "$REPORT_PATH" 2>/dev/null; then
  VERDICT="FAIL"
elif grep -qE '审查结果[^`]*`PASS`' "$REPORT_PATH" 2>/dev/null; then
  VERDICT="PASS"
elif grep -qE '审查结果[^`]*`FAIL`' "$REPORT_PATH" 2>/dev/null; then
  VERDICT="FAIL"
fi

if [[ "$VERDICT" == "PASS" ]]; then
  echo ""
  echo "✓ Code review PASSED."

  # 打印 minor 问题数量提示（不阻断）
  MINOR_COUNT=$(grep -oE '\*\*Minor:\*\* ([0-9]+)' "$REPORT_PATH" | grep -oE '[0-9]+' | head -1 || echo "0")
  if [[ "$MINOR_COUNT" -gt "0" ]]; then
    echo "  Note: ${MINOR_COUNT} minor suggestion(s) recorded in ${REPORT_PATH}"
  fi

  exit 0
fi

if [[ "$VERDICT" == "FAIL" ]]; then
  echo ""
  echo "✗ Code review FAILED. Push rejected."
  echo ""

  # ------------------------------------------------------------------------------
  # Step 7: 打印问题摘要
  # ------------------------------------------------------------------------------
  CRITICAL_COUNT=$(grep -oE '\*\*Critical:\*\* ([0-9]+)' "$REPORT_PATH" | grep -oE '[0-9]+' | head -1 || echo "0")
  IMPORTANT_COUNT=$(grep -oE '\*\*Important:\*\* ([0-9]+)' "$REPORT_PATH" | grep -oE '[0-9]+' | head -1 || echo "0")

  echo "  Critical issues : ${CRITICAL_COUNT}"
  echo "  Important issues: ${IMPORTANT_COUNT}"
  echo ""
  echo "  Full report: ${REPORT_PATH}"
  echo ""

  # 打印 Critical 问题标题
  if [[ "$CRITICAL_COUNT" -gt "0" ]]; then
    echo "── Critical Issues ──────────────────────────────────────"
    grep -A1 '^### \[C' "$REPORT_PATH" | grep '^### \[C' | sed 's/^### /  /' || true
    echo ""
  fi

  # 打印 Important 问题标题
  if [[ "$IMPORTANT_COUNT" -gt "0" ]]; then
    echo "── Important Issues ─────────────────────────────────────"
    grep -A1 '^### \[I' "$REPORT_PATH" | grep '^### \[I' | sed 's/^### /  /' || true
    echo ""
  fi

  # ------------------------------------------------------------------------------
  # Step 8: 触发修复子任务（通过 codebuddy agent 创建）
  # ------------------------------------------------------------------------------
  echo "==> Creating fix task via codebuddy agent..."

  CRITICAL_SECTION=$(awk '/^## Critical Issues/,/^## (Important Issues|Minor Issues|Strengths|Summary)/' "$REPORT_PATH" | head -60 || echo "(see report)")
  IMPORTANT_SECTION=$(awk '/^## Important Issues/,/^## (Minor Issues|Strengths|Summary)/' "$REPORT_PATH" | head -60 || echo "(see report)")

  FIX_PROMPT_FILE=$(mktemp /tmp/code-review-fix-prompt.XXXXXX.md)

  {
    echo "你是 lumin-acpool 代码修复助手。请根据以下 code review 报告创建一个修复任务（使用 TaskCreate 工具），不要执行修复，只创建任务。"
    echo ""
    echo "task subject: 修复 code review 拦截问题 — ${CURRENT_BRANCH} @ ${HEAD_SHA_SHORT}"
    echo "task description 包含："
    echo "1. review 报告路径：${REPORT_PATH}"
    echo "2. 所有 Critical 问题的完整描述和修复建议"
    echo "3. 所有 Important 问题的完整描述和修复建议"
    echo "4. 修复完成后验证步骤：pre-commit run go-code-review --hook-stage pre-push --all-files"
    echo "5. 验证通过后执行：git push"
    echo ""
    echo "Critical Issues:"
    echo "${CRITICAL_SECTION}"
    echo ""
    echo "Important Issues:"
    echo "${IMPORTANT_SECTION}"
  } > "$FIX_PROMPT_FILE"

  codebuddy --print -y < "$FIX_PROMPT_FILE" &>/dev/null || true
  rm -f "$FIX_PROMPT_FILE"

  echo ""
  echo "  Fix task created. Check CodeBuddy task list to start fixing."
  echo "  After fixing, run: git push"
  echo ""

  exit 1
fi

# Verdict 未知（报告格式异常）
echo "WARN: Could not parse verdict from ${REPORT_PATH}, review gate bypassed."
echo "      Please review the file manually."
exit 0
