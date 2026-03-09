package checks

import (
	"context"
	"errors"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/health"
	"github.com/nomand-zc/lumin-client/providers"
	"github.com/nomand-zc/lumin-client/utils"
)

const (
	CredentialRefreshCheckName = "credential_refresh"

	CooldownUntilKey = "cooldown_until"
)

// CredentialRefreshCheck Token 刷新检查
// 当凭证已过期或剩余有效时间低于 RefreshThreshold 时，尝试调用 Provider.Refresh 刷新凭证
// 依赖 CredentialValidityCheck：只有凭证过期或即将过期时才需要执行刷新
type CredentialRefreshCheck struct {
	// RefreshThreshold 提前刷新阈值
	// 当凭证剩余有效时间低于此值时，提前触发刷新
	// 为 0 时仅在凭证完全过期后才刷新
	RefreshThreshold time.Duration
}

func (c *CredentialRefreshCheck) Name() string {
	return CredentialRefreshCheckName
}

func (c *CredentialRefreshCheck) Severity() health.CheckSeverity {
	return health.SeverityCritical
}

func (c *CredentialRefreshCheck) DependsOn() []string {
	return []string{CredentialValidityCheckName}
}

func (c *CredentialRefreshCheck) Check(ctx context.Context, target health.CheckTarget) *health.CheckResult {
	start := time.Now()
	cred := target.Credential()

	// 凭证未过期且剩余有效时间大于阈值，无需刷新
	if !cred.IsExpired() && start.Before(cred.GetExpiresAt().Add(c.RefreshThreshold)) {
		return &health.CheckResult{
			CheckName: CredentialRefreshCheckName,
			Status:    health.CheckPassed,
			Severity:  health.SeverityCritical,
			Message:   "凭证有效期充足，无需刷新",
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		}
	}

	err := target.ProviderInstance().Client.Refresh(ctx, cred)
	if err == nil {
		return &health.CheckResult{
			CheckName:       CredentialRefreshCheckName,
			Status:          health.CheckPassed,
			Severity:        health.SeverityCritical,
			Message:         "凭证刷新成功",
			SuggestedStatus: utils.ToPtr(account.StatusAvailable),
			Duration:        time.Since(start),
			Timestamp:       time.Now(),
		}
	}

	// 根据错误类型判断
	if errors.Is(err, providers.ErrInvalidGrant) {
		return &health.CheckResult{
			CheckName:       CredentialRefreshCheckName,
			Status:          health.CheckFailed,
			Severity:        health.SeverityCritical,
			Message:         "凭证永久失效 (invalid_grant)，无法恢复",
			SuggestedStatus: utils.ToPtr(account.StatusInvalidated),
			Duration:        time.Since(start),
			Timestamp:       time.Now(),
		}
	}

	var httpErr *providers.HTTPError
	if errors.As(err, &httpErr) {
		switch httpErr.ErrorType {
		case providers.ErrorTypeForbidden:
			return &health.CheckResult{
				CheckName:       CredentialRefreshCheckName,
				Status:          health.CheckFailed,
				Severity:        health.SeverityCritical,
				Message:         "账号被封禁: " + httpErr.Message,
				SuggestedStatus: utils.ToPtr(account.StatusBanned),
				Duration:        time.Since(start),
				Timestamp:       time.Now(),
			}
		case providers.ErrorTypeRateLimit:
			return &health.CheckResult{
				CheckName:       CredentialRefreshCheckName,
				Status:          health.CheckFailed,
				Severity:        health.SeverityCritical,
				Message:         "刷新请求触发限流",
				SuggestedStatus: utils.ToPtr(account.StatusCoolingDown),
				Data: map[string]any{
					CooldownUntilKey: httpErr.CooldownUntil,
				},
				Duration:  time.Since(start),
				Timestamp: time.Now(),
			}
		}
	}

	// 网络错误等临时故障
	result := &health.CheckResult{
		CheckName: CredentialRefreshCheckName,
		Status:    health.CheckError,
		Severity:  health.SeverityCritical,
		Message:   "刷新过程出错: " + err.Error(),
		Duration:  time.Since(start),
		Timestamp: time.Now(),
	}
	// 如果凭证已经过期且刷新失败，建议将账号标记为凭证过期
	if cred.IsExpired() {
		result.SuggestedStatus = utils.ToPtr(account.StatusExpired)
	}
	return result
}
