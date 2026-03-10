package checks

import (
	"context"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/health"
	"github.com/nomand-zc/lumin-client/utils"
)

const CredentialValidityCheckName = "credential_validity"

// CredentialValidityCheck validates the credential format and expiration status.
// A pure local check with no network overhead, typically the prerequisite for all other checks.
type CredentialValidityCheck struct{}

func (c *CredentialValidityCheck) Name() string {
	return CredentialValidityCheckName
}

func (c *CredentialValidityCheck) Severity() health.CheckSeverity {
	return health.SeverityCritical
}

func (c *CredentialValidityCheck) DependsOn() []string {
	return nil // No dependency, executes first
}

func (c *CredentialValidityCheck) Check(ctx context.Context, target health.CheckTarget) *health.CheckResult {
	start := time.Now()

	// Format validation
	if err := target.Credential().Validate(); err != nil {
		return &health.CheckResult{
			CheckName:       CredentialValidityCheckName,
			Status:          health.CheckFailed,
			Severity:        health.SeverityCritical,
			Message:         "credential validation failed: " + err.Error(),
			SuggestedStatus: utils.ToPtr(account.StatusInvalidated),
			Duration:        time.Since(start),
			Timestamp:       time.Now(),
		}
	}

	return &health.CheckResult{
		CheckName: CredentialValidityCheckName,
		Status:    health.CheckPassed,
		Severity:  health.SeverityCritical,
		Message:   "credential is valid",
		Duration:  time.Since(start),
		Timestamp: time.Now(),
	}
}
