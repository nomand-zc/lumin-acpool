package health

import (
	"context"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/provider"
	"github.com/nomand-zc/lumin-client/credentials"
)

// CheckSeverity represents the severity level of a check item.
type CheckSeverity int

const (
	// SeverityInfo is the informational level, does not affect account status (e.g., statistics update).
	SeverityInfo CheckSeverity = iota + 1
	// SeverityWarning is the warning level, may need attention but does not immediately affect availability.
	SeverityWarning
	// SeverityCritical is the critical level, directly affects whether the account is available.
	SeverityCritical
)

// String returns a human-readable string of the severity.
func (s CheckSeverity) String() string {
	switch s {
	case SeverityInfo:
		return "info"
	case SeverityWarning:
		return "warning"
	case SeverityCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// CheckStatus represents the result status of a single check.
type CheckStatus int

const (
	// CheckPassed means the check passed.
	CheckPassed CheckStatus = iota + 1
	// CheckWarning means the check passed but with warnings.
	CheckWarning
	// CheckFailed means the check did not pass.
	CheckFailed
	// CheckSkipped means the check was skipped (e.g., a prerequisite check failed).
	CheckSkipped
	// CheckError means an error occurred during the check itself (e.g., network timeout).
	CheckError
)

// String returns a human-readable string of the check status.
func (s CheckStatus) String() string {
	switch s {
	case CheckPassed:
		return "passed"
	case CheckWarning:
		return "warning"
	case CheckFailed:
		return "failed"
	case CheckSkipped:
		return "skipped"
	case CheckError:
		return "error"
	default:
		return "unknown"
	}
}

// CheckResult holds the execution result of a single check item.
type CheckResult struct {
	// CheckName is the name of the check item.
	CheckName string
	// Status is the result status.
	Status CheckStatus
	// Severity is the severity level.
	Severity CheckSeverity
	// Message is the result description.
	Message string
	// SuggestedStatus is the suggested account status to transition to (optional).
	// nil means this check does not suggest any status change.
	SuggestedStatus *account.Status
	// Data holds additional data produced by the check item.
	// For example, UsageQuotaCheck can place the latest UsageStats here for the upper layer to write back.
	Data any
	// Duration is the execution time of this check item.
	Duration time.Duration
	// Timestamp is the completion time of the check.
	Timestamp time.Time
}

// HealthReport is the summary report of a complete health check.
type HealthReport struct {
	// AccountID is the ID of the checked account.
	AccountID string
	// ProviderKey is the provider the account belongs to.
	ProviderKey provider.ProviderKey
	// Results holds results of each check item (in execution order).
	Results []*CheckResult
	// TotalDuration is the total time of the complete health check.
	TotalDuration time.Duration
	// Timestamp is the completion time of the health check.
	Timestamp time.Time
}

// HasCriticalFailure returns whether the report contains any critical-level check failure.
func (r *HealthReport) HasCriticalFailure() bool {
	for _, result := range r.Results {
		if result.Status == CheckFailed && result.Severity == SeverityCritical {
			return true
		}
	}
	return false
}

// FailedChecks returns all failed check items.
func (r *HealthReport) FailedChecks() []*CheckResult {
	var failed []*CheckResult
	for _, result := range r.Results {
		if result.Status == CheckFailed {
			failed = append(failed, result)
		}
	}
	return failed
}

// WarningChecks returns all warning check items.
func (r *HealthReport) WarningChecks() []*CheckResult {
	var warnings []*CheckResult
	for _, result := range r.Results {
		if result.Status == CheckWarning {
			warnings = append(warnings, result)
		}
	}
	return warnings
}

// PassedChecks returns all passed check items.
func (r *HealthReport) PassedChecks() []*CheckResult {
	var passed []*CheckResult
	for _, result := range r.Results {
		if result.Status == CheckPassed {
			passed = append(passed, result)
		}
	}
	return passed
}

// CheckTarget wraps the information of the object being checked.
// Uses an interface instead of a concrete type to maintain independence of the HealthCheck interface.
type CheckTarget interface {
	// Credential returns the account credential.
	Credential() credentials.Credential
	// ProviderInstance returns the provider runtime instance (including metadata and underlying SDK instance).
	ProviderInstance() *provider.ProviderInstance
	// Account returns the full account object (for check items needing access to statistics and other extra fields).
	Account() *account.Account
}

// HealthCheck is the universal interface for health check items.
// This is the core contract of the entire health check system.
// Both built-in and user-defined check items extend the system by implementing this interface.
type HealthCheck interface {
	// Name returns the unique identifier name of the check item.
	Name() string

	// Severity returns the severity level of this check item.
	Severity() CheckSeverity

	// Check executes the check.
	// ctx: context (with timeout control)
	// target: the object being checked, carrying account info and the underlying Provider instance
	//
	// Convention: even if the check process errors, a CheckResult (Status=CheckError) should be returned
	// instead of relying on error. This ensures HealthReport always collects results from all check items.
	Check(ctx context.Context, target CheckTarget) *CheckResult

	// DependsOn returns the names of prerequisite check items this check depends on.
	// If a dependency's Status is Failed, the current check will be marked as Skipped.
	// Returns nil if there are no dependencies.
	DependsOn() []string
}
