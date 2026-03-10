package account

import (
	"time"

	"github.com/nomand-zc/lumin-acpool/provider"
	"github.com/nomand-zc/lumin-client/credentials"
	"github.com/nomand-zc/lumin-client/usagerule"
)

// Account is the aggregate root representing a managed AI platform account.
type Account struct {
	// ID is the unique identifier of the account.
	ID string
	// ProviderType is the provider type, corresponding to lumin-client's Provider.Type(), e.g., "kiro".
	ProviderType string
	// ProviderName is the provider instance name, corresponding to lumin-client's Provider.Name(), e.g., "kiro-team-a".
	ProviderName string
	// Credential is the underlying credential, corresponding to lumin-client's credentials.Credential.
	Credential credentials.Credential
	// Status is the current account status.
	Status Status
	// Priority is the priority level; higher values indicate higher priority (default 0).
	Priority int
	// Tags is a set of tags for flexible categorization and filtering.
	Tags map[string]string
	// Metadata holds extended metadata for custom business fields.
	Metadata map[string]any

	// --- Usage Information ---

	// UsageStats is the current credential usage statistics snapshot,
	// periodically refreshed from lumin-client by the health checker.
	UsageStats []*usagerule.UsageStats

	// --- Runtime Statistics ---

	// TotalCalls is the total number of calls.
	TotalCalls int64
	// SuccessCalls is the number of successful calls.
	SuccessCalls int64
	// FailedCalls is the number of failed calls.
	FailedCalls int64
	// ConsecutiveFailures is the current consecutive failure count (reset to 0 on success).
	ConsecutiveFailures int
	// LastUsedAt is the last time the account was selected for use.
	LastUsedAt *time.Time
	// LastErrorAt is the last time a call failed.
	LastErrorAt *time.Time
	// LastErrorMsg is the error message of the last failed call.
	LastErrorMsg string

	// --- Cooldown / Circuit Breaker ---

	// CooldownUntil is the cooldown expiration time, effective only when status is StatusCoolingDown.
	CooldownUntil *time.Time
	// CircuitOpenUntil is the circuit breaker expiration time, effective only when status is StatusCircuitOpen.
	CircuitOpenUntil *time.Time

	// --- Timestamps ---

	// CreatedAt is the creation time.
	CreatedAt time.Time
	// UpdatedAt is the last update time.
	UpdatedAt time.Time
}

// SuccessRate calculates the success rate; returns 1.0 when there are no calls.
func (a *Account) SuccessRate() float64 {
	if a.TotalCalls == 0 {
		return 1.0
	}
	return float64(a.SuccessCalls) / float64(a.TotalCalls)
}

// IsUsageLimited returns whether any usage rule has been triggered.
func (a *Account) IsUsageLimited() bool {
	for _, s := range a.UsageStats {
		if s != nil && s.IsTriggered() {
			return true
		}
	}
	return false
}

// UsageRemainRatio returns the minimum remaining usage ratio (0.0 ~ 1.0),
// used in selection strategies to evaluate the account's remaining capacity.
func (a *Account) UsageRemainRatio() float64 {
	minRatio := 1.0
	for _, s := range a.UsageStats {
		if s == nil || s.Rule == nil || s.Rule.Total <= 0 {
			continue
		}
		ratio := s.Remain / s.Rule.Total
		if ratio < minRatio {
			minRatio = ratio
		}
	}
	return minRatio
}

// IsCooldownExpired returns whether the cooldown period has expired.
func (a *Account) IsCooldownExpired() bool {
	if a.CooldownUntil == nil {
		return true
	}
	return time.Now().After(*a.CooldownUntil)
}

// IsCircuitOpenExpired returns whether the circuit breaker timeout has expired (can enter half-open state).
func (a *Account) IsCircuitOpenExpired() bool {
	if a.CircuitOpenUntil == nil {
		return true
	}
	return time.Now().After(*a.CircuitOpenUntil)
}

// ProviderKey returns the composite key composed of ProviderType and ProviderName.
func (a *Account) ProviderKey() provider.ProviderKey {
	return provider.BuildProviderKey(a.ProviderType, a.ProviderName)
}
