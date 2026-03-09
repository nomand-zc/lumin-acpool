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

// UsageQuotaCheck 剩余额度检查
// 调用 lumin-client 的 GetUsageStats 获取最新用量数据
// 依赖凭证有效性检查（凭证无效时无法查用量）
type UsageQuotaCheck struct {
	// WarningThreshold 告警阈值（0.0~1.0），当剩余比例低于此值时返回 Warning
	// 默认 0.1（剩余 10% 时告警）
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

	// 调用 lumin-client 获取最新用量
	stats, err := target.Provider().GetUsageStats(ctx, target.Credential())
	if err != nil {
		return &health.CheckResult{
			CheckName: UsageQuotaCheckName,
			Status:    health.CheckError,
			Severity:  health.SeverityCritical,
			Message:   "获取用量信息失败: " + err.Error(),
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		}
	}

	// 将最新 UsageStats 放入 Data，供上层回写到 Account
	data := map[string]any{UsageStatKey: stats}
	minRatio := 1.0

	// 检查是否有用量规则触发了限制
	for _, s := range stats {
		if s != nil && s.IsTriggered() {
			status := account.StatusCoolingDown
			// 尝试获取下一个窗口的开始时间作为冷却截止时间
			if s.EndTime != nil {
				data[CooldownUntilKey] = s.EndTime
			}
			return &health.CheckResult{
				CheckName:       UsageQuotaCheckName,
				Status:          health.CheckFailed,
				Severity:        health.SeverityCritical,
				Message:         fmt.Sprintf("用量已耗尽 (已用: %.2f, 剩余: %.2f)", s.Used, s.Remain),
				SuggestedStatus: &status,
				Data:            data,
				Duration:        time.Since(start),
				Timestamp:       time.Now(),
			}
		}

		// 计算最小剩余比例
		if s != nil && s.Rule != nil && s.Rule.Total > 0 {
			ratio := s.Remain / s.Rule.Total
			if ratio < minRatio {
				minRatio = ratio
			}
		}
	}

	// 检查是否接近耗尽
	threshold := utils.If(c.WarningThreshold <= 0, defaultWarningThreshold, c.WarningThreshold)
	if minRatio < threshold {
		return &health.CheckResult{
			CheckName: UsageQuotaCheckName,
			Status:    health.CheckWarning,
			Severity:  health.SeverityCritical,
			Message:   fmt.Sprintf("用量即将耗尽 (剩余 %.1f%%)", minRatio*100),
			Data:      data,
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		}
	}

	return &health.CheckResult{
		CheckName: UsageQuotaCheckName,
		Status:    health.CheckPassed,
		Severity:  health.SeverityCritical,
		Message:   fmt.Sprintf("用量充足 (剩余 %.1f%%)", minRatio*100),
		Data:      data,
		Duration:  time.Since(start),
		Timestamp: time.Now(),
	}
}
