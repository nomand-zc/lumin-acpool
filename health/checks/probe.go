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

const ProbeCheckName = "probe"

// ProbeCheck 请求探测检查
// 发送一个轻量级的真实请求来验证账号是否能正常工作
// 适用于发现"凭证未过期但已被平台禁用"等隐蔽问题
// 依赖凭证有效性检查
type ProbeCheck struct {
	// ProbeRequest 探测用的请求体（应尽可能轻量）
	// 如果为 nil，使用默认的最小请求
	ProbeRequest *providers.Request
	// Timeout 探测超时时间，默认 10s
	Timeout time.Duration
}

func (c *ProbeCheck) Name() string {
	return ProbeCheckName
}

func (c *ProbeCheck) Severity() health.CheckSeverity {
	return health.SeverityWarning
}

func (c *ProbeCheck) DependsOn() []string {
	return []string{CredentialValidityCheckName}
}

func (c *ProbeCheck) Check(ctx context.Context, target health.CheckTarget) *health.CheckResult {
	start := time.Now()

	req := c.buildProbeRequest()
	_, err := target.Provider().GenerateContent(ctx, target.Credential(), req)
	if err == nil {
		return &health.CheckResult{
			CheckName: ProbeCheckName,
			Status:    health.CheckPassed,
			Severity:  health.SeverityWarning,
			Message:   "探测请求成功",
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		}
	}
	var httpErr *providers.HTTPError
	if errors.As(err, &httpErr) {
		switch httpErr.ErrorType {
		case providers.ErrorTypeForbidden:
			return &health.CheckResult{
				CheckName:       ProbeCheckName,
				Status:          health.CheckFailed,
				Severity:        health.SeverityWarning,
				Message:         "探测请求被拒绝，账号可能被封禁: " + httpErr.Message,
				SuggestedStatus: utils.ToPtr(account.StatusBanned),
				Duration:        time.Since(start),
				Timestamp:       time.Now(),
			}
		case providers.ErrorTypeRateLimit:
			return &health.CheckResult{
				CheckName:       ProbeCheckName,
				Status:          health.CheckFailed,
				Severity:        health.SeverityWarning,
				Message:         "探测请求触发限流",
				SuggestedStatus: utils.ToPtr(account.StatusCoolingDown),
				Data: map[string]any{
					"cooldown_until": httpErr.CooldownUntil,
				},
				Duration:  time.Since(start),
				Timestamp: time.Now(),
			}
		case providers.ErrorTypeUnauthorized:
			return &health.CheckResult{
				CheckName:       ProbeCheckName,
				Status:          health.CheckFailed,
				Severity:        health.SeverityWarning,
				Message:         "探测请求认证失败，凭证可能已过期",
				SuggestedStatus: utils.ToPtr(account.StatusExpired),
				Duration:        time.Since(start),
				Timestamp:       time.Now(),
			}
		case providers.ErrorTypeBadRequest:
			// BadRequest 通常说明账号是正常的，只是请求格式问题
			// 对于探测请求来说，能得到 400 响应说明账号能正常通信
			return &health.CheckResult{
				CheckName: ProbeCheckName,
				Status:    health.CheckPassed,
				Severity:  health.SeverityWarning,
				Message:   "探测请求返回 BadRequest，账号通信正常",
				Duration:  time.Since(start),
				Timestamp: time.Now(),
			}
		}
	}

	// 其他错误（如网络超时等）
	return &health.CheckResult{
		CheckName: ProbeCheckName,
		Status:    health.CheckError,
		Severity:  health.SeverityWarning,
		Message:   "探测请求出错: " + err.Error(),
		Duration:  time.Since(start),
		Timestamp: time.Now(),
	}

}

// buildProbeRequest 构造探测请求
func (c *ProbeCheck) buildProbeRequest() providers.Request {
	if c.ProbeRequest != nil {
		return *c.ProbeRequest
	}
	// 构造最轻量的探测请求
	maxTokens := 1
	return providers.Request{
		Messages: []providers.Message{
			{Role: providers.RoleUser, Content: "hi, please only reply `Hi`"},
		},
		GenerationConfig: providers.GenerationConfig{
			MaxTokens: &maxTokens,
		},
	}
}
