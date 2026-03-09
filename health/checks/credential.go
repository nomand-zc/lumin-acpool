package checks

import (
	"context"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/health"
	"github.com/nomand-zc/lumin-client/utils"
)

const CredentialValidityCheckName = "credential_validity"

// CredentialValidityCheck 凭证格式和过期状态校验
// 纯本地检查，无网络开销，通常是所有检查项的前置依赖
type CredentialValidityCheck struct{}

func (c *CredentialValidityCheck) Name() string {
	return CredentialValidityCheckName
}

func (c *CredentialValidityCheck) Severity() health.CheckSeverity {
	return health.SeverityCritical
}

func (c *CredentialValidityCheck) DependsOn() []string {
	return nil // 无依赖，最先执行
}

func (c *CredentialValidityCheck) Check(ctx context.Context, target health.CheckTarget) *health.CheckResult {
	start := time.Now()

	// 格式校验
	if err := target.Credential().Validate(); err != nil {
		return &health.CheckResult{
			CheckName:       CredentialValidityCheckName,
			Status:          health.CheckFailed,
			Severity:        health.SeverityCritical,
			Message:         "凭证格式校验失败: " + err.Error(),
			SuggestedStatus: utils.ToPtr(account.StatusInvalidated),
			Duration:        time.Since(start),
			Timestamp:       time.Now(),
		}
	}

	return &health.CheckResult{
		CheckName: CredentialValidityCheckName,
		Status:    health.CheckPassed,
		Severity:  health.SeverityCritical,
		Message:   "凭证有效",
		Duration:  time.Since(start),
		Timestamp: time.Now(),
	}
}
