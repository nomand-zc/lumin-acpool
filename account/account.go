package account

import (
	"maps"
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

	// --- Usage Rules ---
	UsageRules []*usagerule.UsageRule

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

// Clone 创建 Account 的深拷贝。
func (a *Account) Clone() *Account {
	if a == nil {
		return nil
	}

	dst := *a

	// 拷贝 Tags
	if a.Tags != nil {
		dst.Tags = make(map[string]string, len(a.Tags))
		maps.Copy(dst.Tags, a.Tags)
	}

	// 拷贝 Metadata
	if a.Metadata != nil {
		dst.Metadata = make(map[string]any, len(a.Metadata))
		maps.Copy(dst.Metadata, a.Metadata)
	}

	// 拷贝 UsageRules
	if a.UsageRules != nil {
		dst.UsageRules = make([]*usagerule.UsageRule, len(a.UsageRules))
		for i, rule := range a.UsageRules {
			if rule != nil {
				r := *rule
				dst.UsageRules[i] = &r
			}
		}
	}

	// 拷贝时间指针
	if a.CooldownUntil != nil {
		t := *a.CooldownUntil
		dst.CooldownUntil = &t
	}
	if a.CircuitOpenUntil != nil {
		t := *a.CircuitOpenUntil
		dst.CircuitOpenUntil = &t
	}

	return &dst
}
