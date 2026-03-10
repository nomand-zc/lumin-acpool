package account

import (
	"maps"
	"slices"
	"time"

	"github.com/nomand-zc/lumin-client/providers"
	"github.com/nomand-zc/lumin-client/usagerule"
)

// ProviderKey is the two-level identifier for a provider; Type + Name uniquely identifies a provider group.
type ProviderKey struct {
	// Type is the provider type, corresponding to lumin-client's Provider.Type(), e.g., "kiro".
	Type string
	// Name is the provider instance name, corresponding to lumin-client's Provider.Name(), e.g., "kiro-team-a".
	Name string
}

// String returns a "type/name" formatted string representation.
func (pk ProviderKey) String() string {
	return pk.Type + "/" + pk.Name
}

// BuildProviderKey creates a ProviderKey instance.
// Provides a unified construction entry point to avoid direct literal construction externally.
func BuildProviderKey(providerType, providerName string) ProviderKey {
	return ProviderKey{
		Type: providerType,
		Name: providerName,
	}
}

// ProviderStatus represents the provider status.
type ProviderStatus int

const (
	// ProviderStatusActive means the provider is active and enabled.
	ProviderStatusActive ProviderStatus = 1
	// ProviderStatusDisabled means the provider is manually disabled.
	ProviderStatusDisabled ProviderStatus = 2
	// ProviderStatusDegraded means the provider is degraded (some models unavailable or usage tight).
	ProviderStatusDegraded ProviderStatus = 3
)

// ProviderInfo holds provider metadata, describing the static info and runtime status of a Provider instance.
type ProviderInfo struct {
	// ProviderType is the provider type, corresponding to lumin-client's Provider.Type(), e.g., "kiro".
	ProviderType string
	// ProviderName is the provider instance name, corresponding to lumin-client's Provider.Name(), e.g., "kiro-team-a".
	ProviderName string
	// Status is the current status.
	Status ProviderStatus
	// Priority is the priority level; higher values indicate higher priority (default 0).
	Priority int
	// Weight is used for weighted selection (default 1).
	Weight int
	// Tags is a set of tags for categorization and filtering.
	Tags map[string]string
	// SupportedModels is the list of models supported by this provider.
	SupportedModels []string
	// UsageRules are the usage rules associated with this provider (obtained from lumin-client and stored).
	UsageRules []*usagerule.UsageRule
	// Metadata holds extended metadata.
	Metadata map[string]any

	// --- Runtime Statistics ---

	// AccountCount is the total number of accounts in this group.
	AccountCount int
	// AvailableAccountCount is the number of available accounts.
	AvailableAccountCount int

	// --- Timestamps ---

	// CreatedAt is the creation time.
	CreatedAt time.Time
	// UpdatedAt is the last update time.
	UpdatedAt time.Time
}

// ProviderKey returns the composite key composed of ProviderType and ProviderName.
func (p *ProviderInfo) ProviderKey() ProviderKey {
	return BuildProviderKey(p.ProviderType, p.ProviderName)
}

// SupportsModel returns whether this provider supports the specified model.
func (p *ProviderInfo) SupportsModel(model string) bool {
	return slices.Contains(p.SupportedModels, model)
}

// IsActive returns whether the provider is in an active state.
func (p *ProviderInfo) IsActive() bool {
	return p.Status == ProviderStatusActive || p.Status == ProviderStatusDegraded
}

// Clone creates a deep copy of ProviderInfo to prevent external modification of internal stored data.
func (p *ProviderInfo) Clone() *ProviderInfo {
	dst := *p

	// Deep copy Tags
	if p.Tags != nil {
		dst.Tags = make(map[string]string, len(p.Tags))
		for k, v := range p.Tags {
			dst.Tags[k] = v
		}
	}

	// Deep copy SupportedModels
	if p.SupportedModels != nil {
		dst.SupportedModels = make([]string, len(p.SupportedModels))
		copy(dst.SupportedModels, p.SupportedModels)
	}

	// Deep copy UsageRules
	if p.UsageRules != nil {
		dst.UsageRules = make([]*usagerule.UsageRule, len(p.UsageRules))
		copy(dst.UsageRules, p.UsageRules)
	}

	// Deep copy Metadata
	if p.Metadata != nil {
		dst.Metadata = make(map[string]any, len(p.Metadata))
		maps.Copy(dst.Metadata, p.Metadata)
	}

	return &dst
}

// ProviderInstance is a provider runtime instance.
// It binds the metadata (ProviderInfo) with the underlying SDK instance (providers.Provider).
type ProviderInstance struct {
	// Info holds the provider metadata.
	Info *ProviderInfo
	// Client is the underlying lumin-client Provider instance, used for actual API calls.
	Client providers.Provider
}
