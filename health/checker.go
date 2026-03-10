package health

import (
	"context"
	"time"
)

// CheckSchedule is the scheduling configuration for a check item.
// Different check items can have different execution frequencies.
type CheckSchedule struct {
	// Check is the check item instance.
	Check HealthCheck
	// Interval is the execution interval.
	// For example: credential validation 10s, usage refresh 5m, probe request 2m.
	Interval time.Duration
	// Enabled indicates whether this check is enabled.
	Enabled bool
}

// HealthChecker is the health check orchestrator interface.
// It manages registered check items and orchestrates their execution in dependency order.
type HealthChecker interface {
	// Register registers a check item with its scheduling configuration.
	Register(schedule CheckSchedule)

	// Unregister removes a registered check item.
	Unregister(checkName string)

	// ListChecks lists all currently registered check items and their scheduling configurations.
	ListChecks() []CheckSchedule

	// RunAll executes all registered check items against the specified target.
	// Executes in topological order based on DependsOn; checks whose dependencies failed are automatically Skipped.
	RunAll(ctx context.Context, target CheckTarget) (*HealthReport, error)

	// RunOne executes a single check item against the specified target.
	RunOne(ctx context.Context, target CheckTarget, checkName string) (*CheckResult, error)

	// Start launches the background periodic health check task.
	// Each check item runs independently based on its own Interval.
	Start(ctx context.Context) error

	// Stop halts the background health check.
	Stop() error
}
