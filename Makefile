.PHONY: env-up env-down env-status test test-race test-integration bench lint fmt tidy check

# --- 环境管理 ---

## 启动本地测试依赖（MySQL + Redis），等待健康检查通过
env-up:
	docker compose up -d --wait

## 停止并清理本地测试依赖
env-down:
	docker compose down -v

## 查看依赖服务状态
env-status:
	docker compose ps

# --- 测试 ---

## 运行所有单元测试（带竞态检测，不使用缓存）
test:
	go test -race -count=1 ./...

## 运行单元测试并生成覆盖率报告
test-cover:
	go test -race -count=1 -cover -coverprofile=cover.out \
		$(shell go list ./... | grep -v '/cli') && \
	go tool cover -html=cover.out -o cover.html
	@echo "覆盖率报告已生成: cover.html"

## 运行集成测试（需要先执行 make env-up）
test-integration:
	go test -tags=integration -race -count=1 -v ./...

## 运行所有测试（单元 + 集成），完整验收，测试后自动清理容器
test-all: env-up
	go test -race -count=1 ./...
	go test -tags=integration -race -count=1 -v ./... ; $(MAKE) env-down

# --- 基准测试 ---

## 对所有包运行基准测试（5 轮，含内存分配统计）
bench:
	go test -bench=. -benchmem -count=5 ./...

## 对指定包运行基准测试，例如：make bench-pkg PKG=./balancer/...
bench-pkg:
	go test -bench=. -benchmem -count=5 $(PKG)

# --- 代码质量 ---

## 格式化代码
fmt:
	go fmt ./...

## 运行 lint 检查
lint:
	golangci-lint run ./...

## 整理依赖
tidy:
	go mod tidy

## 完整本地验收（fmt + lint + test），等价于 pre-commit 门禁
check: fmt tidy lint test
