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

// CredentialRefreshCheck performs Token refresh checking.
// When the credential has expired or remaining validity is below RefreshThreshold, it attempts to call Provider.Refresh.
// Depends on CredentialValidityCheck: only needs to execute when the credential is expired or about to expire.
type CredentialRefreshCheck struct {
	// RefreshThreshold is the threshold for early refresh.
	// When the remaining credential validity is below this value, refresh is triggered early.
	// When set to 0, refresh only occurs after the credential has fully expired.
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

	// Credential is not expired and remaining validity exceeds threshold, no refresh needed
	if !cred.IsExpired() && start.Before(cred.GetExpiresAt().Add(c.RefreshThreshold)) {
		return &health.CheckResult{
			CheckName: CredentialRefreshCheckName,
			Status:    health.CheckPassed,
			Severity:  health.SeverityCritical,
			Message:   "credential validity is sufficient, no refresh needed",
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
			Message:         "credential refreshed successfully",
			SuggestedStatus: utils.ToPtr(account.StatusAvailable),
			Duration:        time.Since(start),
			Timestamp:       time.Now(),
		}
	}

	// Determine by error type
	if errors.Is(err, providers.ErrInvalidGrant) {
		return &health.CheckResult{
			CheckName:       CredentialRefreshCheckName,
			Status:          health.CheckFailed,
			Severity:        health.SeverityCritical,
			Message:         "credential permanently invalidated (invalid_grant), cannot be recovered",
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
				Message:         "account banned: " + httpErr.Message,
				SuggestedStatus: utils.ToPtr(account.StatusBanned),
				Duration:        time.Since(start),
				Timestamp:       time.Now(),
			}
		case providers.ErrorTypeRateLimit:
			return &health.CheckResult{
				CheckName:       CredentialRefreshCheckName,
				Status:          health.CheckFailed,
				Severity:        health.SeverityCritical,
				Message:         "refresh request triggered rate limit",
				SuggestedStatus: utils.ToPtr(account.StatusCoolingDown),
				Data: map[string]any{
					CooldownUntilKey: httpErr.CooldownUntil,
				},
				Duration:  time.Since(start),
				Timestamp: time.Now(),
			}
		}
	}

	// Network errors and other temporary failures
	result := &health.CheckResult{
		CheckName: CredentialRefreshCheckName,
		Status:    health.CheckError,
		Severity:  health.SeverityCritical,
		Message:   "refresh error: " + err.Error(),
		Duration:  time.Since(start),
		Timestamp: time.Now(),
	}
	// If the credential has expired and refresh failed, suggest marking the account as credential expired
	if cred.IsExpired() {
		result.SuggestedStatus = utils.ToPtr(account.StatusExpired)
	}
	return result
}
