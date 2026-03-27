#!/usr/bin/env bash
set -euo pipefail

# ==============================================================================
# 覆盖率阈值配置
# ==============================================================================
COVERAGE_GLOBAL_MIN=80   # 可测包全局行覆盖率最低要求（%）
COVERAGE_CRITICAL_MIN=90 # 关键调度路径最低要求（%）

# 关键路径包（覆盖率要求更高）
CRITICAL_PKGS=(
  "github.com/nomand-zc/lumin-acpool/balancer"
  "github.com/nomand-zc/lumin-acpool/selector"
  "github.com/nomand-zc/lumin-acpool/resolver"
)

# 从全局覆盖率统计中排除的包前缀（纯 CLI 工具，无法独立单测）
# storage/mysql、storage/redis、storage/sqlite 已通过 sqlmock/miniredis/内存库 完成单测，不再排除
EXCLUDE_PKG_PREFIXES=(
  "github.com/nomand-zc/lumin-acpool/cli"
)

COVERAGE_OUT="coverage.out"
COVERAGE_TMP=$(mktemp)
trap 'rm -f "$COVERAGE_OUT" "$COVERAGE_TMP"' EXIT

# ==============================================================================
# Step 1: 运行测试（race 检测 + 覆盖率采集）
# ==============================================================================
echo "==> Running go test with race detection and coverage..."
go test -race -count=1 -coverprofile="$COVERAGE_OUT" -covermode=atomic ./...

# ==============================================================================
# Step 2: 解析全局覆盖率（排除不可测包）
# ==============================================================================
echo ""
echo "==> Analyzing coverage (excluding cli/ which requires interactive terminal)..."

go tool cover -func="$COVERAGE_OUT" > "$COVERAGE_TMP"

# 构造排除正则：匹配任意以排除前缀开头的行
EXCLUDE_PATTERN=$(printf "|^%s" "${EXCLUDE_PKG_PREFIXES[@]}")
EXCLUDE_PATTERN="${EXCLUDE_PATTERN:1}" # 去掉开头的 |

# 从函数列表中排除不可测包，计算剩余包的语句覆盖率
# coverprofile 格式: file:line.col,line.col N covered
# 使用 grep 过滤 coverprofile 文件，只保留可测包的行
FILTERED_OUT=$(mktemp)
trap 'rm -f "$COVERAGE_OUT" "$COVERAGE_TMP" "$FILTERED_OUT"' EXIT

# 从 coverage.out 中过滤掉不可测包（coverprofile 第一列是 pkg/file.go:...）
{
  # 保留 mode 行
  head -1 "$COVERAGE_OUT"
  # 过滤掉排除前缀的包行
  tail -n +2 "$COVERAGE_OUT" | grep -v -E "$(printf '%s|' "${EXCLUDE_PKG_PREFIXES[@]}" | sed 's/|$//')" || true
} > "$FILTERED_OUT"

GLOBAL_COVERAGE=$(go tool cover -func="$FILTERED_OUT" | grep '^total:' | awk '{print $3}' | tr -d '%')

if [[ -z "$GLOBAL_COVERAGE" ]]; then
  echo "ERROR: Failed to parse global coverage" >&2
  exit 1
fi

echo "Global coverage (testable packages): ${GLOBAL_COVERAGE}% (required: >=${COVERAGE_GLOBAL_MIN}%)"

FAIL=0

# 使用 awk 进行浮点比较
if awk "BEGIN { exit !($GLOBAL_COVERAGE < $COVERAGE_GLOBAL_MIN) }"; then
  echo "FAIL: Global coverage ${GLOBAL_COVERAGE}% is below threshold ${COVERAGE_GLOBAL_MIN}%" >&2
  FAIL=1
fi

# ==============================================================================
# Step 3: 解析关键路径覆盖率
# ==============================================================================
for PKG in "${CRITICAL_PKGS[@]}"; do
  # 只取该包的直接函数行（不含子包），过滤掉 total 行
  PKG_LINES=$(grep "^${PKG}/" "$COVERAGE_TMP" | grep -v '/[a-z_]*/[a-z_]*/' || true)

  if [[ -z "$PKG_LINES" ]]; then
    echo "WARN: No coverage data found for critical package: $PKG (skipping)"
    continue
  fi

  # 计算包内平均覆盖率：提取所有百分比数字，求算术平均
  PKG_COVERAGE=$(echo "$PKG_LINES" | awk '
    {
      val = $(NF)
      gsub(/%/, "", val)
      sum += val
      count++
    }
    END {
      if (count > 0) printf "%.1f", sum/count
      else print "0"
    }
  ')

  echo "Critical package [${PKG##*/}] coverage: ${PKG_COVERAGE}% (required: >=${COVERAGE_CRITICAL_MIN}%)"

  if awk "BEGIN { exit !($PKG_COVERAGE < $COVERAGE_CRITICAL_MIN) }"; then
    echo "FAIL: Package ${PKG##*/} coverage ${PKG_COVERAGE}% is below threshold ${COVERAGE_CRITICAL_MIN}%" >&2
    FAIL=1
  fi
done

# ==============================================================================
# Step 4: 汇总结果
# ==============================================================================
echo ""
if [[ $FAIL -ne 0 ]]; then
  echo "Coverage check FAILED. Please add more tests and retry." >&2
  exit 1
fi

echo "Coverage check PASSED."
