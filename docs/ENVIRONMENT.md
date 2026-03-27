# 本地开发环境指南

本文档覆盖本地开发工作流的完整环节：依赖启动、单元测试、集成测试、基准测试。

## 前置要求

| 工具 | 版本要求 | 用途 |
|------|----------|------|
| Go | 1.24+ | 编译和测试 |
| Docker | 20.10+ | 运行 MySQL / Redis |
| Docker Compose | v2（`docker compose`） | 编排本地依赖 |
| golangci-lint | v1.64.8 | 代码静态分析 |

安装 golangci-lint：

```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8
```

---

## 存储服务依赖

项目支持四种存储后端，本地开发依赖如下：

| 后端 | 启动方式 | 默认连接 |
|------|----------|---------|
| Memory | 无需启动，纯内存 | — |
| SQLite | 无需启动，文件自动创建 | `./lumin_acpool_test.db` |
| MySQL | `make env-up` | `localhost:3306` |
| Redis | `make env-up` | `localhost:6379` |

### 启动 / 停止依赖服务

```bash
# 启动 MySQL + Redis，等待健康检查通过后返回
make env-up

# 确认服务状态
make env-status

# 停止容器并删除容器和数据卷（测试结束后执行）
make env-down
```

`make env-up` 底层执行 `docker compose up -d --wait`，仅在所有服务健康检查通过后才返回，无需手动等待。

`make env-down` 底层执行 `docker compose down -v`，停止并**删除容器和数据卷**，确保下次启动时环境完全干净。测试结束后务必执行，避免残留容器占用端口。

#### MySQL 连接信息

| 参数 | 值 |
|------|----|
| Host | `localhost:3306` |
| Database | `lumin_acpool_test` |
| User | `acpool` |
| Password | `acpool123` |
| Root Password | `root` |

#### Redis 连接信息

| 参数 | 值 |
|------|----|
| Host | `localhost:6379` |
| 无鉴权 | — |

---

## 单元测试

单元测试**不依赖外部服务**，所有存储依赖通过 `sqlmock` / `miniredis` 模拟。

```bash
# 运行全部单元测试（含竞态检测，禁用缓存）
make test

# 运行并生成覆盖率 HTML 报告（排除 cli/ 目录）
make test-cover
```

完成后打开 `cover.html` 查看覆盖率详情。

**覆盖率要求**（见 [docs/COMMIT_ACCEPTANCE.md](COMMIT_ACCEPTANCE.md)）：

- 全局 ≥ 80%
- 关键路径（`balancer/`、`selector/`、`resolver/`、`storage/`）≥ 90%

---

## 集成测试

集成测试连接真实存储服务，使用构建标签 `integration` 与单元测试隔离。

### 编写集成测试

**文件命名**：`<name>_integration_test.go`，与被测文件同包同目录。

**文件头部必须声明构建标签**：

```go
//go:build integration

package mysql

import (
    "context"
    "os"
    "testing"
)

func IntegrationTestAccountCRUD(t *testing.T) {
    // 1. 从环境变量读取连接配置，默认值为本地测试库
    dsn := os.Getenv("MYSQL_DSN")
    if dsn == "" {
        dsn = "acpool:acpool123@tcp(localhost:3306)/lumin_acpool_test?parseTime=true"
    }

    // 2. 创建 Store 实例
    store, err := NewStore(WithDSN(dsn), WithSkipInitDB(false))
    if err != nil {
        t.Fatalf("failed to create store: %v", err)
    }
    defer store.Close()

    // 3. 注册清理函数（测试后自动调用）
    t.Cleanup(func() {
        // 清理测试数据，确保幂等性和数据隔离
        cleanupTestData(t, store)
    })

    // 4. 清理初始状态（确保测试开始前环境干净）
    cleanupTestData(t, store)

    ctx := context.Background()

    // 5. 进行测试...
}
```

**关键约束**：

- 测试数据在 `t.Cleanup` 中清理，确保每次执行后环境干净
- 通过环境变量 `MYSQL_DSN` / `REDIS_ADDR` 覆盖连接配置，方便 CI/CD 注入
- 用例之间完全独立，不依赖执行顺序

**完整示例**：见 [`storage/mysql/account_integration_test.go`](../../storage/mysql/account_integration_test.go)

- `IntegrationTest_MySQLAccountCRUD` - Account 聚合根 CRUD、外键级联删除、乐观锁版本冲突
- `IntegrationTest_MySQLAccountStats` - Stats 原子操作（成功/失败/重置）

### 运行集成测试

```bash
# 先启动依赖服务
make env-up

# 运行全部集成测试
make test-integration

# 测试完毕后停止并删除容器
make env-down

# 或手动执行（支持环境变量覆盖连接配置）
MYSQL_DSN="acpool:acpool123@tcp(localhost:3306)/lumin_acpool_test?parseTime=true" \
REDIS_ADDR="localhost:6379" \
go test -tags=integration -race -count=1 -v ./...
```

### 单次完整验收（单元 + 集成）

```bash
# 自动启动依赖，依次运行单元测试和集成测试，测试结束后自动清理容器（无论成功失败）
make test-all
```

---

## 基准测试

基准测试用于追踪核心路径的性能变化，每次版本迭代**核心函数性能不得下降**。

### 编写基准测试

**文件命名**：`<name>_benchmark_test.go`，与被测文件同包同目录。

**完整示例**：

1. **Balancer 基准测试** - 见 [`balancer/default_balancer_benchmark_test.go`](../../balancer/default_balancer_benchmark_test.go)
   - 16 个基准场景：Pick 三种模式、Report 操作、Selector 策略、并发场景、完整流程

2. **Selector 策略基准测试** - 账号级见 [`selector/strategies/account/selector_benchmark_test.go`](../../selector/strategies/account/selector_benchmark_test.go)
   - 5 个账号级策略：RoundRobin、Priority、Weighted、LeastUsed、Affinity
   - 每个策略 Small/Medium/Large/Parallel 四种场景
   - Affinity 额外覆盖零命中、高命中、空过滤三种场景

3. **Selector 策略基准测试** - 供应商级见 [`selector/strategies/group/selector_benchmark_test.go`](../../selector/strategies/group/selector_benchmark_test.go)
   - 5 个供应商级策略：GroupRoundRobin、GroupPriority、GroupWeighted、GroupMostAvailable、GroupAffinity
   - 每个策略 Small/Medium/Large/Parallel 四种场景

包含以下 Balancer 基准场景：

| 基准名称 | 测试内容 | 
|---------|---------|
| `BenchmarkPick_AutoMode` | 自动模式 Pick（全自动供应商和账号选择）|
| `BenchmarkPick_ExactMode` | 精确模式 Pick（指定 Provider，快于自动模式）|
| `BenchmarkPick_LargeScale` | 大规模场景（20 个 Provider × 50 账号）|
| `BenchmarkReportSuccess_NoStatsStore` | 无统计存储的热路径 |
| `BenchmarkReportSuccess_WithStatsStore` | 含统计存储的完整流程 |
| `BenchmarkPick_And_ReportSuccess` | 完整 Pick + ReportSuccess 流程 |
| `BenchmarkPick_Parallel` | 并发 Pick（10 个 Provider × 30 账号）|
| `BenchmarkPick_And_ReportSuccess_Parallel` | 并发完整流程 |

**关键约束**：

```go
func BenchmarkPickRoundRobin(b *testing.B) {
    // 初始化放在计时器重置之前
    bal := setupBalancer()
    b.ResetTimer()  // 必须调用，避免初始化计入结果

    for i := 0; i < b.N; i++ {
        _, _ = bal.Pick(context.Background(), pickOptions)  // 循环内仅放被测逻辑
    }
}

// 并发场景基准测试
func BenchmarkPickParallel(b *testing.B) {
    bal := setupBalancer()
    b.ResetTimer()

    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            _, _ = bal.Pick(context.Background(), pickOptions)
        }
    })
}
```

- `b.ResetTimer()` 必须在初始化代码之后调用，避免初始化耗时计入结果
- 循环体内只放被测逻辑，禁止包含日志、断言、数据初始化
- `b.RunParallel` 用于测试高并发场景性能

### 运行基准测试

```bash
# 运行全部包的基准测试（5 轮，含内存分配统计）
make bench

# 运行指定包的基准测试
make bench-pkg PKG=./balancer/...

# 对比两次提交的性能（需安装 benchstat）
go install golang.org/x/perf/cmd/benchstat@latest
make bench > old.txt
# 修改代码后
make bench > new.txt
benchstat old.txt new.txt
```

**关注指标**：

| 指标 | 含义 | 目标 |
|------|------|------|
| `ns/op` | 每次操作耗时（纳秒） | Pick 全链路 < 100µs（100000 ns） |
| `B/op` | 每次操作内存分配量（字节） | Pick 单次 < 10 KB |
| `allocs/op` | 每次操作分配次数 | Pick 单次 < 60 次 |

**已验证基准值**（Memory 存储后端，Intel Xeon Gold 6133 @ 2.50GHz，4 核）：

| 基准名称 | ns/op | B/op | allocs/op | 说明 |
|---------|-------|------|-----------|------|
| `BenchmarkPick_AutoMode` | ~6,100 | 5,992 | 48 | 5 Provider × 20 账号 |
| `BenchmarkPick_ExactMode` | ~3,900 | 4,720 | 29 | 精确模式，跳过供应商选择 |
| `BenchmarkPick_TypeOnlyMode` | ~6,150 | 6,024 | 49 | 类型模式，同自动模式相近 |
| `BenchmarkPick_LargeScale` | ~18,300 | 17,696 | 115 | 20 Provider × 50 账号 |
| `BenchmarkReportSuccess_NoStatsStore` | ~6 | 0 | 0 | 热路径无分配 |
| `BenchmarkReportSuccess_WithStatsStore` | ~149 | 24 | 1 | 含统计存储 |
| `BenchmarkReportFailure_NonRateLimit` | ~493 | 248 | 4 | 非限流失败 |
| `BenchmarkPick_And_ReportSuccess` | ~6,400 | 5,992 | 48 | 完整流程 |
| `BenchmarkPick_And_ReportFailure` | ~6,550 | 6,216 | 51 | 完整流程含失败 |
| `BenchmarkPick_Parallel` | ~7,700 | 10,144 | 73 | 并发，10P × 30A |
| `BenchmarkPick_And_ReportSuccess_Parallel` | ~7,650 | 10,144 | 73 | 并发完整流程 |
| `BenchmarkReportSuccess_Parallel` | ~345 | 29 | 2 | 并发 Report |

**关键发现**：
- Pick 全链路（含 Memory 存储）**远低于 100µs 目标**，实测 ~6µs（快 15 倍）
- `ReportSuccess_NoStatsStore` **零内存分配**，热路径优化有效
- 精确模式 (~3.9µs) 比自动模式 (~6.1µs) 快 36%（跳过供应商选择）
- 大规模场景 (20P×50A) 耗时 ~18µs，仍完全满足目标

**性能对比和追踪**：

```bash
# 保存基准结果到文件
make bench-pkg PKG=./balancer/... > baseline.txt

# 修改代码后再运行一次
make bench-pkg PKG=./balancer/... > current.txt

# 对比性能变化（需安装 benchstat）
go install golang.org/x/perf/cmd/benchstat@latest
benchstat baseline.txt current.txt
```

**性能回归检测**：

每次版本迭代，**核心函数（Pick、Selector、OccupancyController）性能不得下降超过 5%**。若性能回归超过阈值，需要：
1. 检查是否有避免不了的逻辑增加
2. 考虑是否有优化空间（缓存、批量操作等）
3. 必要时调整性能基准值并在代码审查中解释原因

---

## Makefile 命令速览

| 命令 | 说明 |
|------|------|
| `make env-up` | 启动 MySQL + Redis，等待健康检查 |
| `make env-down` | 停止并删除容器和数据卷 |
| `make env-status` | 查看依赖服务状态 |
| `make test` | 单元测试（含竞态检测） |
| `make test-cover` | 单元测试 + 覆盖率报告 |
| `make test-integration` | 集成测试（需先 env-up） |
| `make test-all` | 自动启动依赖并运行全量测试，结束后自动清理容器 |
| `make bench` | 全包基准测试 |
| `make bench-pkg PKG=./xxx/...` | 指定包基准测试 |
| `make fmt` | 格式化代码 |
| `make lint` | 运行 golangci-lint |
| `make tidy` | 整理 go.mod |
| `make check` | 本地完整验收（fmt + tidy + lint + test） |

---

## CI/CD 集成参考

在 CI 环境中，通过环境变量注入连接配置，跳过 Docker 启动步骤（由 CI 提供服务）：

```yaml
# 示例：GitHub Actions / CNB CI
services:
  mysql:
    image: mysql:8.0
    env:
      MYSQL_DATABASE: lumin_acpool_test
      MYSQL_USER: acpool
      MYSQL_PASSWORD: acpool123
      MYSQL_ROOT_PASSWORD: root
  redis:
    image: redis:7-alpine

steps:
  - name: 单元测试
    run: go test -race -count=1 ./...

  - name: 集成测试
    env:
      MYSQL_DSN: acpool:acpool123@tcp(localhost:3306)/lumin_acpool_test?parseTime=true
      REDIS_ADDR: localhost:6379
    run: go test -tags=integration -race -count=1 -v ./...
```
