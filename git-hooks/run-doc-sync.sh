#!/usr/bin/env bash
set -euo pipefail

# ==============================================================================
# pre-push Doc Sync 门禁
#
# 行为：
#   - 纯文档/测试/CLI 变更（无核心模块变更）：自动 SKIP
#   - SKIP_DOC_SYNC=1：强制跳过（仅限紧急情况，须在 PR 中注明）
#   - --manual：绕过跳过条件，强制执行
#   - codebuddy CLI 不可用：打印警告并放行（软性降级）
#   - 文档有修改：自动创建 "docs: auto sync [skip ci]" commit
# ==============================================================================

REPORT_PATH=".doc-sync-report.md"
CURRENT_BRANCH=$(git symbolic-ref --short HEAD 2>/dev/null || echo "HEAD")
HEAD_SHA=$(git rev-parse HEAD)
HEAD_SHA_SHORT=$(git rev-parse --short HEAD)
MANUAL_MODE=0

# --manual 参数解析（pre-commit 通过 stdin 传 refs，此参数仅手动调用时使用）
for arg in "$@"; do
  [[ "$arg" == "--manual" ]] && MANUAL_MODE=1
done

# ------------------------------------------------------------------------------
# Step 1: 检查是否强制跳过
# ------------------------------------------------------------------------------
if [[ "${SKIP_DOC_SYNC:-0}" == "1" ]]; then
  echo "WARN: Doc sync skipped via SKIP_DOC_SYNC=1"
  echo "      Please document the reason in your PR description."
  exit 0
fi

# ------------------------------------------------------------------------------
# Step 2: 确定 base SHA
# ------------------------------------------------------------------------------
if git rev-parse "origin/${CURRENT_BRANCH}" &>/dev/null 2>&1; then
  BASE_SHA=$(git rev-parse "origin/${CURRENT_BRANCH}")
else
  BASE_SHA=$(git rev-parse HEAD~1 2>/dev/null || git rev-parse --root)
fi

# ------------------------------------------------------------------------------
# Step 3: 获取变更文件列表
# ------------------------------------------------------------------------------
CHANGED_FILES=$(git diff --name-only "${BASE_SHA}".."${HEAD_SHA}" \
  | grep -v -E '(_generated\.go$|^testdata/|^docs/references/)' \
  || true)

COMMIT_MSGS=$(git log --oneline "${BASE_SHA}".."${HEAD_SHA}" || true)

# ------------------------------------------------------------------------------
# Step 4: 触发条件检测
# 核心模块路径 或 commit message 关键词
# ------------------------------------------------------------------------------
should_trigger() {
  local files="$1"
  local msgs="$2"

  # commit message 关键词（[docs-sync] 也强制触发，即使满足跳过条件）
  if echo "$msgs" | grep -qiE '^[a-f0-9]+ (feat|refactor|breaking|api):|\[docs-sync\]'; then
    return 0
  fi

  # 核心模块路径匹配
  local core_paths="^account/ ^balancer/ ^selector/ ^health/ ^storage/ ^usagetracker/ ^circuitbreaker/ ^cooldown/ ^docs/CODE_REVIEW\.md"
  for pat in $core_paths; do
    if echo "$files" | grep -qE "$pat"; then
      return 0
    fi
  done

  return 1
}

# ------------------------------------------------------------------------------
# Step 5: 跳过条件检测（--manual 时跳过此判断）
# 仅当所有变更文件属于 docs/**、*_test.go、cli/** 的组合时跳过
# [docs-sync] commit 强制触发（不受跳过条件影响）
# ------------------------------------------------------------------------------
should_skip() {
  local files="$1"
  local msgs="$2"

  # [docs-sync] 强制触发
  if echo "$msgs" | grep -q '\[docs-sync\]'; then
    return 1
  fi

  # 检查是否有超出豁免范围的文件
  local non_exempt
  non_exempt=$(echo "$files" | grep -vE '^docs/|_test\.go$|^cli/' || true)
  [[ -z "$non_exempt" ]]
}

if [[ "$MANUAL_MODE" -eq 0 ]]; then
  if [[ -z "$CHANGED_FILES" ]]; then
    echo "==> Doc sync skipped: no changed files detected."
    exit 0
  fi

  if ! should_trigger "$CHANGED_FILES" "$COMMIT_MSGS"; then
    echo "==> Doc sync skipped: no core module changes or trigger keywords."
    exit 0
  fi

  if should_skip "$CHANGED_FILES" "$COMMIT_MSGS"; then
    echo "==> Doc sync skipped: changes are docs/tests/cli only."
    exit 0
  fi
fi

# ------------------------------------------------------------------------------
# Step 6: 检测 codebuddy CLI 可用性
# ------------------------------------------------------------------------------
if ! command -v codebuddy &>/dev/null; then
  echo "WARN: codebuddy CLI not found, skipping doc sync."
  echo "      Install codebuddy CLI or run sync manually:"
  echo "      ./git-hooks/run-doc-sync.sh --manual"
  exit 0
fi

# ------------------------------------------------------------------------------
# Step 7: 组装上下文文件
# ------------------------------------------------------------------------------
echo "==> Doc sync: analyzing changes (base: ${BASE_SHA:0:8}, head: ${HEAD_SHA_SHORT})..."

CONTEXT_FILE=$(mktemp /tmp/doc-sync-context.XXXXXX.md)
PROMPT_FILE=$(mktemp /tmp/doc-sync-prompt.XXXXXX.md)
trap 'rm -f "$CONTEXT_FILE" "$PROMPT_FILE"' EXIT

# 分类变更文件
CORE_FILES=$(echo "$CHANGED_FILES" | grep -E '^(account|balancer|selector|health|storage|usagetracker|circuitbreaker|cooldown)/' || true)
DOC_FILES=$(echo "$CHANGED_FILES"  | grep -E '^docs/'  || true)
TEST_FILES=$(echo "$CHANGED_FILES" | grep -E '_test\.go$' || true)
OTHER_FILES=$(echo "$CHANGED_FILES" | grep -vE '^(account|balancer|selector|health|storage|usagetracker|circuitbreaker|cooldown|docs)/|_test\.go$' || true)

# 候选受影响文档（基于路径规则初步筛选，最终由 agent 判断）
CANDIDATE_DOCS=""
echo "$CORE_FILES" | grep -q '^account/'       && CANDIDATE_DOCS+="docs/design-docs/account-lifecycle.md ARCHITECTURE.md "
echo "$CORE_FILES" | grep -q '^balancer/'      && CANDIDATE_DOCS+="docs/design-docs/pick-flow.md ARCHITECTURE.md "
echo "$CORE_FILES" | grep -q '^selector/'      && CANDIDATE_DOCS+="docs/design-docs/pick-flow.md docs/design-docs/add-strategy.md "
echo "$CORE_FILES" | grep -q '^health/'        && CANDIDATE_DOCS+="docs/design-docs/health-check.md "
echo "$CORE_FILES" | grep -q '^storage/'       && CANDIDATE_DOCS+="docs/design-docs/storage.md ARCHITECTURE.md "
echo "$CORE_FILES" | grep -q '^usagetracker/'  && CANDIDATE_DOCS+="docs/design-docs/usage-and-cooldown.md "
echo "$CORE_FILES" | grep -q '^circuitbreaker/' && CANDIDATE_DOCS+="docs/design-docs/usage-and-cooldown.md "
echo "$CORE_FILES" | grep -q '^cooldown/'      && CANDIDATE_DOCS+="docs/design-docs/usage-and-cooldown.md "
echo "$DOC_FILES"  | grep -q 'CODE_REVIEW'     && CANDIDATE_DOCS+=".codebuddy/agents/code-review.md "
# 去重
CANDIDATE_DOCS=$(echo "$CANDIDATE_DOCS" | tr ' ' '\n' | sort -u | tr '\n' ' ')

{
  echo "## Changed Files"
  echo ""
  [[ -n "$CORE_FILES" ]]  && { echo "**core:**"; echo "$CORE_FILES"; echo ""; }
  [[ -n "$DOC_FILES" ]]   && { echo "**docs:**"; echo "$DOC_FILES"; echo ""; }
  [[ -n "$TEST_FILES" ]]  && { echo "**tests:**"; echo "$TEST_FILES"; echo ""; }
  [[ -n "$OTHER_FILES" ]] && { echo "**other:**"; echo "$OTHER_FILES"; echo ""; }
  echo "## Affected Doc Candidates"
  echo ""
  echo "${CANDIDATE_DOCS:-（none — agent 自主判断）}"
  echo ""
  echo "## Commit Messages"
  echo ""
  echo "$COMMIT_MSGS"
  echo ""
  echo "## Push Metadata"
  echo ""
  echo "base_sha: ${BASE_SHA}"
  echo "head_sha: ${HEAD_SHA}"
  echo "branch: ${CURRENT_BRANCH}"
} > "$CONTEXT_FILE"

# ------------------------------------------------------------------------------
# Step 8: 调用 doc-maintainer agent
# ------------------------------------------------------------------------------
rm -f "$REPORT_PATH"

export DOC_SYNC_BASE_SHA="$BASE_SHA"
export DOC_SYNC_HEAD_SHA="$HEAD_SHA"
export DOC_SYNC_BRANCH="$CURRENT_BRANCH"
export DOC_SYNC_CONTEXT_PATH="$CONTEXT_FILE"
export DOC_SYNC_REPORT_PATH="$REPORT_PATH"

{
  cat .codebuddy/agents/doc-maintainer.md
  echo ""
  echo "---"
  echo "## 当前同步任务"
  echo ""
  echo "base_sha: ${DOC_SYNC_BASE_SHA}"
  echo "head_sha: ${DOC_SYNC_HEAD_SHA}"
  echo "branch: ${DOC_SYNC_BRANCH}"
  echo "context_path: ${DOC_SYNC_CONTEXT_PATH}"
  echo "report_path: ${DOC_SYNC_REPORT_PATH}"
  echo ""
  echo "请严格按照 docs/DOC_SYNC.md 的规范执行文档同步，将完整报告写入 ${DOC_SYNC_REPORT_PATH}。"
  echo "报告必须包含 '## Verdict:' 行（UPDATED / SKIPPED / NO_CHANGES / SUGGESTIONS_ONLY）。"
} > "$PROMPT_FILE"

set +e
codebuddy --print -y < "$PROMPT_FILE" > "$REPORT_PATH" 2>&1
CODEBUDDY_EXIT=$?
set -e

if [[ $CODEBUDDY_EXIT -ne 0 ]]; then
  echo "WARN: Doc sync agent exited with error (code: ${CODEBUDDY_EXIT}), skipping doc sync gate."
  echo "      Check codebuddy configuration and retry manually."
  exit 0
fi

# ------------------------------------------------------------------------------
# Step 9: 解析 Verdict
# ------------------------------------------------------------------------------
if [[ ! -f "$REPORT_PATH" ]]; then
  echo "WARN: Doc sync report not generated, skipping gate."
  exit 0
fi

VERDICT="UNKNOWN"
for v in UPDATED SKIPPED NO_CHANGES SUGGESTIONS_ONLY; do
  grep -q "## Verdict:.*${v}" "$REPORT_PATH" 2>/dev/null && VERDICT="$v" && break
done

echo ""
case "$VERDICT" in
  NO_CHANGES|SKIPPED)
    echo "✓ Doc sync: no documentation changes needed."
    ;;
  UPDATED)
    echo "✓ Doc sync: documentation updated."
    # ------------------------------------------------------------------------------
    # Step 10: 检测文档是否有修改，若有则自动创建独立 commit
    # ------------------------------------------------------------------------------
    UPDATED_DOCS=$(git diff --name-only -- 'docs/' '.codebuddy/agents/' 2>/dev/null || true)
    if [[ -n "$UPDATED_DOCS" ]]; then
      UPDATED_LIST=$(echo "$UPDATED_DOCS" | tr '\n' ',' | sed 's/,$//')
      git add docs/ .codebuddy/agents/ 2>/dev/null || true
      git commit -m "$(cat <<EOF
docs: auto sync [skip ci]

Triggered by: ${HEAD_SHA_SHORT}
Branch: ${CURRENT_BRANCH}
Updated: ${UPDATED_LIST}
EOF
)" || true
      echo "  Created: docs: auto sync commit (${UPDATED_LIST})"
    fi
    ;;
  SUGGESTIONS_ONLY)
    echo "! Doc sync: large changes require manual review."
    echo "  See ${REPORT_PATH} for suggestions."
    ;;
  *)
    echo "WARN: Could not parse verdict from ${REPORT_PATH}, doc sync gate bypassed."
    ;;
esac

exit 0
