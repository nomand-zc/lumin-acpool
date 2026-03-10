package health

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// TargetProvider 提供待巡检目标的函数类型
// 后台定时巡检需要通过此函数获取当前需要检查的目标列表
type TargetProvider func(ctx context.Context) []CheckTarget

// ReportCallback 巡检报告回调函数类型
// 后台巡检完成后调用，上层可用于更新账号状态、持久化报告等
type ReportCallback func(ctx context.Context, report *HealthReport)

// CheckerOption 配置 defaultHealthChecker 的选项函数
type CheckerOption func(*defaultHealthChecker)

// WithTargetProvider 设置目标提供者，后台巡检时通过此函数获取待检查的目标
func WithTargetProvider(tp TargetProvider) CheckerOption {
	return func(c *defaultHealthChecker) {
		c.targetProvider = tp
	}
}

// WithReportCallback 设置巡检报告回调
// 每次后台巡检完成一个目标后，会调用此回调传递报告
func WithReportCallback(fn ReportCallback) CheckerOption {
	return func(c *defaultHealthChecker) {
		c.onReport = fn
	}
}

// defaultHealthChecker 是 HealthChecker 接口的默认实现
// 管理已注册的检查项，支持按依赖拓扑排序执行，并提供后台定时巡检能力
type defaultHealthChecker struct {
	mu        sync.RWMutex
	schedules map[string]CheckSchedule // key: check name

	// targetProvider 后台巡检时用于获取待检查目标列表
	targetProvider TargetProvider
	// onReport 巡检报告回调
	onReport ReportCallback

	// 后台巡检生命周期控制
	cancel context.CancelFunc
	done   chan struct{}
}

// NewHealthChecker 创建一个默认的 HealthChecker 实例
func NewHealthChecker(opts ...CheckerOption) HealthChecker {
	c := &defaultHealthChecker{
		schedules: make(map[string]CheckSchedule),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Register 注册检查项及其调度配置
func (c *defaultHealthChecker) Register(schedule CheckSchedule) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.schedules[schedule.Check.Name()] = schedule
}

// Unregister 取消注册检查项
func (c *defaultHealthChecker) Unregister(checkName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.schedules, checkName)
}

// ListChecks 列出当前注册的所有检查项及调度配置
func (c *defaultHealthChecker) ListChecks() []CheckSchedule {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]CheckSchedule, 0, len(c.schedules))
	for _, s := range c.schedules {
		result = append(result, s)
	}
	return result
}

// RunAll 对指定目标执行所有已启用的检查项
// 按 DependsOn 拓扑排序后依次执行，依赖失败的检查项自动标记为 Skipped
func (c *defaultHealthChecker) RunAll(ctx context.Context, target CheckTarget) (*HealthReport, error) {
	c.mu.RLock()
	var enabledChecks []HealthCheck
	for _, s := range c.schedules {
		if s.Enabled {
			enabledChecks = append(enabledChecks, s.Check)
		}
	}
	c.mu.RUnlock()

	start := time.Now()

	if len(enabledChecks) == 0 {
		return &HealthReport{
			AccountID:     target.Account().ID,
			ProviderKey:   target.Account().ProviderKey(),
			TotalDuration: 0,
			Timestamp:     time.Now(),
		}, nil
	}

	// 按依赖关系拓扑排序
	sorted, err := topologicalSort(enabledChecks)
	if err != nil {
		return nil, fmt.Errorf("health: 拓扑排序失败: %w", err)
	}

	results := make([]*CheckResult, 0, len(sorted))
	statusMap := make(map[string]CheckStatus) // 记录已执行检查项的状态

	for _, check := range sorted {
		// 检查前置依赖是否都通过了
		if shouldSkip(check, statusMap) {
			result := &CheckResult{
				CheckName: check.Name(),
				Status:    CheckSkipped,
				Severity:  check.Severity(),
				Message:   "前置依赖检查未通过，跳过执行",
				Timestamp: time.Now(),
			}
			results = append(results, result)
			statusMap[check.Name()] = CheckSkipped
			continue
		}

		// 执行检查
		result := check.Check(ctx, target)
		results = append(results, result)
		statusMap[check.Name()] = result.Status
	}

	return &HealthReport{
		AccountID:     target.Account().ID,
		ProviderKey:   target.Account().ProviderKey(),
		Results:       results,
		TotalDuration: time.Since(start),
		Timestamp:     time.Now(),
	}, nil
}

// RunOne 对指定目标执行单个检查项
func (c *defaultHealthChecker) RunOne(ctx context.Context, target CheckTarget, checkName string) (*CheckResult, error) {
	c.mu.RLock()
	schedule, ok := c.schedules[checkName]
	c.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("health: 检查项 %q 未注册", checkName)
	}

	return schedule.Check.Check(ctx, target), nil
}

// Start 启动后台定时巡检任务
// 按各检查项的 Interval 独立定时执行
func (c *defaultHealthChecker) Start(ctx context.Context) error {
	if c.targetProvider == nil {
		return fmt.Errorf("health: 未设置 TargetProvider，无法启动后台巡检")
	}

	c.mu.Lock()
	if c.cancel != nil {
		c.mu.Unlock()
		return fmt.Errorf("health: 后台巡检已在运行中")
	}
	ctx, c.cancel = context.WithCancel(ctx)
	c.done = make(chan struct{})
	c.mu.Unlock()

	go c.backgroundLoop(ctx)
	return nil
}

// Stop 停止后台巡检
func (c *defaultHealthChecker) Stop() error {
	c.mu.Lock()
	if c.cancel == nil {
		c.mu.Unlock()
		return nil
	}
	cancelFn := c.cancel
	c.cancel = nil
	doneCh := c.done
	c.mu.Unlock()

	cancelFn()
	<-doneCh
	return nil
}

// backgroundLoop 后台巡检主循环
// 使用最小 Interval 作为 tick 基准，按各检查项各自的 Interval 判断是否需要执行
func (c *defaultHealthChecker) backgroundLoop(ctx context.Context) {
	defer close(c.done)

	// 启动时立即执行一轮完整巡检
	c.runFullScan(ctx)

	c.mu.RLock()
	minInterval := c.getMinInterval()
	c.mu.RUnlock()

	if minInterval <= 0 {
		return
	}

	ticker := time.NewTicker(minInterval)
	defer ticker.Stop()

	// 记录每个检查项的上次执行时间
	lastRun := make(map[string]time.Time)

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			c.tickRun(ctx, now, lastRun)
		}
	}
}

// getMinInterval 获取所有已启用检查项中最小的 Interval
// 调用方需持有读锁
func (c *defaultHealthChecker) getMinInterval() time.Duration {
	var min time.Duration
	for _, s := range c.schedules {
		if s.Enabled && s.Interval > 0 {
			if min == 0 || s.Interval < min {
				min = s.Interval
			}
		}
	}
	return min
}

// tickRun 每次 tick 时执行到期的检查项
func (c *defaultHealthChecker) tickRun(ctx context.Context, now time.Time, lastRun map[string]time.Time) {
	c.mu.RLock()
	var dueChecks []HealthCheck
	for _, s := range c.schedules {
		if !s.Enabled || s.Interval <= 0 {
			continue
		}
		last, ok := lastRun[s.Check.Name()]
		if !ok || now.Sub(last) >= s.Interval {
			dueChecks = append(dueChecks, s.Check)
		}
	}
	c.mu.RUnlock()

	if len(dueChecks) == 0 {
		return
	}

	targets := c.targetProvider(ctx)
	for _, target := range targets {
		for _, check := range dueChecks {
			select {
			case <-ctx.Done():
				return
			default:
				check.Check(ctx, target)
				lastRun[check.Name()] = now
			}
		}
	}
}

// runFullScan 对所有目标执行一轮完整巡检
func (c *defaultHealthChecker) runFullScan(ctx context.Context) {
	targets := c.targetProvider(ctx)
	for _, target := range targets {
		select {
		case <-ctx.Done():
			return
		default:
			report, err := c.RunAll(ctx, target)
			if err == nil && c.onReport != nil {
				c.onReport(ctx, report)
			}
		}
	}
}

// --- 辅助函数 ---

// topologicalSort 按 DependsOn 关系对检查项进行拓扑排序（Kahn 算法）
// 确保被依赖的检查项排在前面
func topologicalSort(checks []HealthCheck) ([]HealthCheck, error) {
	// 构建名称到检查项的映射
	checkMap := make(map[string]HealthCheck, len(checks))
	for _, check := range checks {
		checkMap[check.Name()] = check
	}

	// 构建入度表和邻接表
	inDegree := make(map[string]int, len(checks))
	dependents := make(map[string][]string) // dep -> 依赖 dep 的检查项列表

	for _, check := range checks {
		name := check.Name()
		if _, exists := inDegree[name]; !exists {
			inDegree[name] = 0
		}
		for _, dep := range check.DependsOn() {
			// 只统计在当前已注册检查项中存在的依赖
			if _, exists := checkMap[dep]; exists {
				inDegree[name]++
				dependents[dep] = append(dependents[dep], name)
			}
		}
	}

	// Kahn 算法：从入度为 0 的节点开始
	queue := make([]string, 0)
	for name, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, name)
		}
	}

	sorted := make([]HealthCheck, 0, len(checks))
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		sorted = append(sorted, checkMap[current])

		for _, next := range dependents[current] {
			inDegree[next]--
			if inDegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}

	if len(sorted) != len(checks) {
		return nil, fmt.Errorf("检查项之间存在循环依赖")
	}

	return sorted, nil
}

// shouldSkip 判断当前检查项是否应被跳过
// 如果任一前置依赖项的状态为 Failed / Error / Skipped，则跳过当前检查项
func shouldSkip(check HealthCheck, statusMap map[string]CheckStatus) bool {
	for _, dep := range check.DependsOn() {
		if status, ok := statusMap[dep]; ok {
			switch status {
			case CheckFailed, CheckError, CheckSkipped:
				return true
			}
		}
	}
	return false
}
