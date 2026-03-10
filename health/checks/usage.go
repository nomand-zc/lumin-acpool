package checks

import (
	"context"
	"fmt"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/health"
	"github.com/nomand-zc/lumin-client/utils"
)

const (
	UsageQuotaCheckName = "usage_quota"

	defaultWarningThreshold = 0.01

	UsageStatKey = "usage_stats"
)

// UsageQuotaCheck performs remaining quota checking.
// Calls lumin-client's GetUsageStats to obtain the latest usage data.
// Depends on credential validity check (cannot query usage with invalid credentials).
type UsageQuotaCheck struct {
	// WarningThreshold is the warning threshold (0.0~1.0); returns Warning when remaining ratio is below this value.
	// Default: 0.1 (warn when 10% remaining).
	WarningThreshold float64
}

func (c *UsageQuotaCheck) Name() string {
	return UsageQuotaCheckName
}

func (c *UsageQuotaCheck) Severity() health.CheckSeverity {
	return health.SeverityCritical
}

func (c *UsageQuotaCheck) DependsOn() []string {
	return []string{CredentialValidityCheckName}
}

func (c *UsageQuotaCheck) Check(ctx context.Context, target health.CheckTarget) *health.CheckResult {
	start := time.Now()

	// Call lumin-client to get the latest usage
	stats, err := target.ProviderInstance().Client.GetUsageStats(ctx, target.Credential())
	if err != nil {
		return &health.CheckResult{
			CheckName: UsageQuotaCheckName,
			Status:    health.CheckError,
			Severity:  health.SeverityCritical,
			Message:   "failed to get usage info: " + err.Error(),
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		}
	}

	// Place the latest UsageStats in Data for the upper layer to write back to Account
	data := map[string]any{UsageStatKey: stats}
	minRatio := 1.0

	// Check if any usage rule has been triggered
	for _, s := range stats {
		if s != nil && s.IsTriggered() {
			status := account.StatusCoolingDown
			// Try to get the start time of the next window as cooldown expiration
			if s.EndTime != nil {
				data[CooldownUntilKey] = s.EndTime
			}
			return &health.CheckResult{
				CheckName:       UsageQuotaCheckName,
				Status:          health.CheckFailed,
				Severity:        health.SeverityCritical,
				Message:         fmt.Sprintf("usage exhausted (used: %.2f, remaining: %.2f)", s.Used, s.Remain),
				SuggestedStatus: &status,
				Data:            data,
				Duration:        time.Since(start),
				Timestamp:       time.Now(),
			}
		}

		// Calculate minimum remaining ratio
		if s != nil && s.Rule != nil && s.Rule.Total > 0 {
			ratio := s.Remain / s.Rule.Total
			if ratio < minRatio {
				minRatio = ratio
			}
		}
	}

	// Check if nearing exhaustion
	threshold := utils.If(c.WarningThreshold <= 0, defaultWarningThreshold, c.WarningThreshold)
	if minRatio < threshold {
		return &health.CheckResult{
			CheckName: UsageQuotaCheckName,
			Status:    health.CheckWarning,
			Severity:  health.SeverityCritical,
			Message:   fmt.Sprintf("usage is about to be exhausted (%.1f%% remaining)", minRatio*100),
			Data:      data,
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		}
	}

	return &health.CheckResult{
		CheckName: UsageQuotaCheckName,
		Status:    health.CheckPassed,
		Severity:  health.SeverityCritical,
		Message:   fmt.Sprintf("usage is sufficient (%.1f%% remaining)", minRatio*100),
		Data:      data,
		Duration:  time.Since(start),
		Timestamp: time.Now(),
	}
}
