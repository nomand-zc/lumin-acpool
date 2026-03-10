package health

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// TargetProvider is the function type for providing targets to be checked.
// The background periodic health check needs this function to obtain the current list of targets to check.
type TargetProvider func(ctx context.Context) []CheckTarget

// ReportCallback is the callback function type for health check reports.
// Called after a background check completes; the upper layer can use it to update account status, persist reports, etc.
type ReportCallback func(ctx context.Context, report *HealthReport)

// CheckerOption is a functional option for configuring defaultHealthChecker.
type CheckerOption func(*defaultHealthChecker)

// WithTargetProvider sets the target provider for obtaining targets to check during background scanning.
func WithTargetProvider(tp TargetProvider) CheckerOption {
	return func(c *defaultHealthChecker) {
		c.targetProvider = tp
	}
}

// WithReportCallback sets the health check report callback.
// Called after each background check completes for a target, passing the report.
func WithReportCallback(fn ReportCallback) CheckerOption {
	return func(c *defaultHealthChecker) {
		c.onReport = fn
	}
}

// defaultHealthChecker is the default implementation of the HealthChecker interface.
// It manages registered check items, supports execution in dependency topological order,
// and provides background periodic health check capability.
type defaultHealthChecker struct {
	mu        sync.RWMutex
	schedules map[string]CheckSchedule // key: check name

	// 缓存的拓扑排序结果，在 Register/Unregister 时预计算
	sortedChecks []HealthCheck // 已排序的启用检查项
	sortErr      error         // 拓扑排序错误（如循环依赖）

	// targetProvider provides the list of targets to check during background scanning.
	targetProvider TargetProvider
	// onReport is the health check report callback.
	onReport ReportCallback

	// Background check lifecycle control
	cancel context.CancelFunc
	done   chan struct{}
}

// NewHealthChecker creates a default HealthChecker instance.
func NewHealthChecker(opts ...CheckerOption) HealthChecker {
	c := &defaultHealthChecker{
		schedules: make(map[string]CheckSchedule),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Register registers a check item with its scheduling configuration.
func (c *defaultHealthChecker) Register(schedule CheckSchedule) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.schedules[schedule.Check.Name()] = schedule
	c.rebuildSortedChecksLocked()
}

// Unregister removes a registered check item.
func (c *defaultHealthChecker) Unregister(checkName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.schedules, checkName)
	c.rebuildSortedChecksLocked()
}

// ListChecks lists all currently registered check items and their scheduling configurations.
func (c *defaultHealthChecker) ListChecks() []CheckSchedule {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]CheckSchedule, 0, len(c.schedules))
	for _, s := range c.schedules {
		result = append(result, s)
	}
	return result
}

// RunAll executes all enabled check items against the specified target.
// Executes in topological order based on DependsOn; checks whose dependencies failed are automatically marked as Skipped.
func (c *defaultHealthChecker) RunAll(ctx context.Context, target CheckTarget) (*HealthReport, error) {
	c.mu.RLock()
	sorted := c.sortedChecks
	sortErr := c.sortErr
	c.mu.RUnlock()

	start := time.Now()

	if sortErr != nil {
		return nil, fmt.Errorf("health: topological sort failed: %w", sortErr)
	}

	if len(sorted) == 0 {
		return &HealthReport{
			AccountID:     target.Account().ID,
			ProviderKey:   target.Account().ProviderKey(),
			TotalDuration: 0,
			Timestamp:     time.Now(),
		}, nil
	}

	results := make([]*CheckResult, 0, len(sorted))
	statusMap := make(map[string]CheckStatus) // records the status of executed check items

	for _, check := range sorted {
		// Check if all prerequisites have passed
		if shouldSkip(check, statusMap) {
			result := &CheckResult{
				CheckName: check.Name(),
				Status:    CheckSkipped,
				Severity:  check.Severity(),
				Message:   "prerequisite dependency check failed, skipping execution",
				Timestamp: time.Now(),
			}
			results = append(results, result)
			statusMap[check.Name()] = CheckSkipped
			continue
		}

		// Execute the check
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

// RunOne executes a single check item against the specified target.
func (c *defaultHealthChecker) RunOne(ctx context.Context, target CheckTarget, checkName string) (*CheckResult, error) {
	c.mu.RLock()
	schedule, ok := c.schedules[checkName]
	c.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("health: check %q is not registered", checkName)
	}

	return schedule.Check.Check(ctx, target), nil
}

// Start launches the background periodic health check task.
// Each check item runs independently based on its own Interval.
func (c *defaultHealthChecker) Start(ctx context.Context) error {
	if c.targetProvider == nil {
		return fmt.Errorf("health: TargetProvider is not set, cannot start background health check")
	}

	c.mu.Lock()
	if c.cancel != nil {
		c.mu.Unlock()
		return fmt.Errorf("health: background health check is already running")
	}
	ctx, c.cancel = context.WithCancel(ctx)
	c.done = make(chan struct{})
	c.mu.Unlock()

	go c.backgroundLoop(ctx)
	return nil
}

// Stop halts the background health check.
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

// backgroundLoop is the main loop for background health checking.
// Uses the minimum Interval as the tick base, and checks each item's own Interval to determine if execution is needed.
func (c *defaultHealthChecker) backgroundLoop(ctx context.Context) {
	defer close(c.done)

	// Execute a full scan immediately on startup
	c.runFullScan(ctx)

	c.mu.RLock()
	minInterval := c.getMinInterval()
	c.mu.RUnlock()

	if minInterval <= 0 {
		return
	}

	ticker := time.NewTicker(minInterval)
	defer ticker.Stop()

	// Record the last execution time for each check item
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

// getMinInterval returns the minimum Interval among all enabled check items.
// The caller must hold a read lock.
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

// tickRun executes due check items on each tick.
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

// runFullScan performs a full health check on all targets.
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

// rebuildSortedChecksLocked 重新计算启用检查项的拓扑排序并缓存结果。
// 调用方必须持有写锁。
func (c *defaultHealthChecker) rebuildSortedChecksLocked() {
	var enabledChecks []HealthCheck
	for _, s := range c.schedules {
		if s.Enabled {
			enabledChecks = append(enabledChecks, s.Check)
		}
	}
	if len(enabledChecks) == 0 {
		c.sortedChecks = nil
		c.sortErr = nil
		return
	}
	sorted, err := topologicalSort(enabledChecks)
	c.sortedChecks = sorted
	c.sortErr = err
}

// --- Helper functions ---

// topologicalSort performs topological sorting on check items based on DependsOn relationships (Kahn's algorithm).
// Ensures that dependent check items are placed before their dependents.
func topologicalSort(checks []HealthCheck) ([]HealthCheck, error) {
	// Build name-to-check mapping
	checkMap := make(map[string]HealthCheck, len(checks))
	for _, check := range checks {
		checkMap[check.Name()] = check
	}

	// Build in-degree table and adjacency list
	inDegree := make(map[string]int, len(checks))
	dependents := make(map[string][]string) // dep -> list of checks that depend on dep

	for _, check := range checks {
		name := check.Name()
		if _, exists := inDegree[name]; !exists {
			inDegree[name] = 0
		}
		for _, dep := range check.DependsOn() {
			// Only count dependencies that exist in the currently registered check items
			if _, exists := checkMap[dep]; exists {
				inDegree[name]++
				dependents[dep] = append(dependents[dep], name)
			}
		}
	}

	// Kahn's algorithm: start from nodes with in-degree 0
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
		return nil, fmt.Errorf("circular dependency detected among check items")
	}

	return sorted, nil
}

// shouldSkip determines whether the current check item should be skipped.
// If any prerequisite's status is Failed / Error / Skipped, the current check is skipped.
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
