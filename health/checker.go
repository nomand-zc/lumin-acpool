package health

import (
	"context"
	"time"
)

// CheckSchedule 检查项的调度配置
// 不同检查项可以有不同的执行频率
type CheckSchedule struct {
	// Check 检查项实例
	Check HealthCheck
	// Interval 执行间隔
	// 例如凭证校验 10s，用量刷新 5m，请求探测 2m
	Interval time.Duration
	// Enabled 是否启用
	Enabled bool
}

// HealthChecker 健康巡检编排器接口
// 负责管理注册的检查项，按依赖顺序编排执行
type HealthChecker interface {
	// Register 注册检查项及其调度配置
	Register(schedule CheckSchedule)

	// Unregister 取消注册检查项
	Unregister(checkName string)

	// ListChecks 列出当前注册的所有检查项及调度配置
	ListChecks() []CheckSchedule

	// RunAll 对指定目标执行所有已注册的检查项
	// 按 DependsOn 拓扑排序后依次执行，依赖失败的检查项自动 Skipped
	RunAll(ctx context.Context, target CheckTarget) (*HealthReport, error)

	// RunOne 对指定目标执行单个检查项
	RunOne(ctx context.Context, target CheckTarget, checkName string) (*CheckResult, error)

	// Start 启动后台定时巡检任务
	// 按各检查项的 Interval 独立定时执行
	Start(ctx context.Context) error

	// Stop 停止后台巡检
	Stop() error
}
