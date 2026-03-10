package checks

import (
	"context"
	"fmt"
	"time"

	"github.com/nomand-zc/lumin-acpool/health"
)

const UsageRulesRefreshCheckName = "usage_rules_refresh"

// usageRulesDataKey 是 UsageRules 数据在 CheckResult.Data 中的标准 Key。
const usageRulesDataKey = "usage_rules"

// UsageRulesRefreshCheck 用量规则刷新检查。
// 调用 lumin-client 的 GetUsageRules() 动态获取最新的用量规则，
// 并将结果放入 Data 中供上层回调更新 Account.UsageRules 和 UsageTracker。
//
// 适用场景：平台动态调整了账号的用量规则（如从免费升级为付费，额度增加等）。
// 依赖凭证有效性检查（需要有效凭证才能查询规则）。
type UsageRulesRefreshCheck struct{}

func (c *UsageRulesRefreshCheck) Name() string {
	return UsageRulesRefreshCheckName
}

func (c *UsageRulesRefreshCheck) Severity() health.CheckSeverity {
	return health.SeverityInfo
}

func (c *UsageRulesRefreshCheck) DependsOn() []string {
	return []string{CredentialValidityCheckName}
}

func (c *UsageRulesRefreshCheck) Check(ctx context.Context, target health.CheckTarget) *health.CheckResult {
	start := time.Now()

	rules, err := target.Client().GetUsageRules(ctx, target.Credential())
	if err != nil {
		return &health.CheckResult{
			CheckName: UsageRulesRefreshCheckName,
			Status:    health.CheckError,
			Severity:  health.SeverityInfo,
			Message:   "failed to refresh usage rules: " + err.Error(),
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		}
	}

	if len(rules) == 0 {
		return &health.CheckResult{
			CheckName: UsageRulesRefreshCheckName,
			Status:    health.CheckPassed,
			Severity:  health.SeverityInfo,
			Message:   "no usage rules found (unlimited)",
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		}
	}

	return &health.CheckResult{
		CheckName: UsageRulesRefreshCheckName,
		Status:    health.CheckPassed,
		Severity:  health.SeverityInfo,
		Message:   fmt.Sprintf("refreshed %d usage rules", len(rules)),
		Data: map[string]any{
			usageRulesDataKey: rules,
		},
		Duration:  time.Since(start),
		Timestamp: time.Now(),
	}
}
