#!/usr/bin/env bash
set -euo pipefail

# ==============================================================================
# pre-push 集成测试门禁
#
# 行为：
#   - Docker 不可用时：打印警告并放行（软性降级）
#   - Docker 可用时：启动依赖 → 运行集成测试 → 始终清理容器（trap）
# ==============================================================================

# ------------------------------------------------------------------------------
# Step 1: 检测 Docker 可用性
# ------------------------------------------------------------------------------
if ! command -v docker &>/dev/null || ! docker info &>/dev/null 2>&1; then
  echo "WARN: Docker is not available, skipping integration tests."
  echo "      Please run 'make test-integration' manually before merging."
  exit 0
fi

# ------------------------------------------------------------------------------
# Step 2: 注册清理 trap，确保无论成功还是失败都执行 env-down
# ------------------------------------------------------------------------------
cleanup() {
  echo ""
  echo "==> Cleaning up: docker compose down -v ..."
  make env-down
}
trap cleanup EXIT

# ------------------------------------------------------------------------------
# Step 3: 启动依赖服务（等待健康检查通过）
# ------------------------------------------------------------------------------
echo "==> Starting dependencies (MySQL + Redis) ..."
make env-up

# ------------------------------------------------------------------------------
# Step 4: 运行集成测试
# ------------------------------------------------------------------------------
echo ""
echo "==> Running integration tests (-tags=integration -race -count=1) ..."
go test -tags=integration -race -count=1 -v ./...

echo ""
echo "Integration tests PASSED."
