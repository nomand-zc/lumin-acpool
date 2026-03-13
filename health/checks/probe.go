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

// ProbeCheck performs a probe check by sending a lightweight real request.
// It verifies that the account can function properly.
// Useful for discovering hidden issues like "credential not expired but banned by platform".
// Depends on credential validity check.
type ProbeCheck struct {
	// ProbeRequest is the request body for probing (should be as lightweight as possible).
	// If nil, a default minimal request is used.
	ProbeRequest *providers.Request
	// Timeout is the probe timeout duration, default 10s.
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
	req.Credential = target.Credential()
	_, err := target.Client().GenerateContent(ctx, req)
	if err == nil {
		return &health.CheckResult{
			CheckName: ProbeCheckName,
			Status:    health.CheckPassed,
			Severity:  health.SeverityWarning,
			Message:   "probe request succeeded",
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
				Message:         "probe request rejected, account may be banned: " + httpErr.Message,
				SuggestedStatus: utils.ToPtr(account.StatusBanned),
				Duration:        time.Since(start),
				Timestamp:       time.Now(),
			}
		case providers.ErrorTypeRateLimit:
			return &health.CheckResult{
				CheckName:       ProbeCheckName,
				Status:          health.CheckFailed,
				Severity:        health.SeverityWarning,
				Message:         "probe request triggered rate limit",
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
				Message:         "probe request authentication failed, credential may have expired",
				SuggestedStatus: utils.ToPtr(account.StatusExpired),
				Duration:        time.Since(start),
				Timestamp:       time.Now(),
			}
		case providers.ErrorTypeBadRequest:
			// BadRequest usually indicates the account is fine, just a request format issue.
			// For a probe request, getting a 400 response means the account can communicate normally.
			return &health.CheckResult{
				CheckName: ProbeCheckName,
				Status:    health.CheckPassed,
				Severity:  health.SeverityWarning,
				Message:   "probe request returned BadRequest, account communication is normal",
				Duration:  time.Since(start),
				Timestamp: time.Now(),
			}
		}
	}

	// Other errors (e.g., network timeout)
	return &health.CheckResult{
		CheckName: ProbeCheckName,
		Status:    health.CheckError,
		Severity:  health.SeverityWarning,
		Message:   "probe request error: " + err.Error(),
		Duration:  time.Since(start),
		Timestamp: time.Now(),
	}

}

// buildProbeRequest constructs the probe request.
func (c *ProbeCheck) buildProbeRequest() *providers.Request {
	if c.ProbeRequest != nil {
		return c.ProbeRequest
	}
	// Construct the lightest possible probe request
	maxTokens := 1
	return &providers.Request{
		Messages: []providers.Message{
			{Role: providers.RoleUser, Content: "hi, please only reply `Hi`"},
		},
		GenerationConfig: providers.GenerationConfig{
			MaxTokens: &maxTokens,
		},
	}
}
