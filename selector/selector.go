package selector

import (
	"github.com/nomand-zc/lumin-acpool/account"
)

// GroupSelector is the provider-level selection strategy interface.
// Selects the optimal provider from a set of candidates based on strategy.
type GroupSelector interface {
	// Name returns the strategy name.
	Name() string

	// Select selects a provider from the candidates.
	//
	// Parameters:
	//   candidates - filtered candidate providers, all with Active or Degraded status
	//   req        - context information for this request
	//
	// Returns:
	//   the selected provider; returns ErrNoAvailableProvider if none available
	Select(candidates []*account.ProviderInfo, req *SelectRequest) (*account.ProviderInfo, error)
}

// Selector is the account-level selection strategy interface.
// Selects the optimal account from a set of filtered candidates based on strategy.
type Selector interface {
	// Name returns the strategy name, used for logging and debugging.
	Name() string

	// Select selects an account from the candidates.
	//
	// Parameters:
	//   candidates - filtered candidate accounts, all with Available status
	//   req        - context information for this request, strategies can reference its fields for decision making
	//
	// Returns:
	//   the selected account; returns ErrNoAvailableAccount if none available
	Select(candidates []*account.Account, req *SelectRequest) (*account.Account, error)
}

// SelectRequest holds the context information for a selection request.
// Carries constraint conditions for this request, for selection strategies to reference.
type SelectRequest struct {
	// UserID 是当前请求的用户标识（可选）。
	// 供亲和策略（Affinity/GroupAffinity）使用，将同一用户的请求绑定到同一个账号/供应商，
	// 以充分利用 LLM 的 system prompt caching 能力。
	// 为空时亲和策略会退化为 fallback 策略。
	UserID string

	// Model is the requested model name (required).
	Model string

	// ProviderKey is the provider locator (optional, pointer type).
	//   - nil: no restriction, auto-select from all active providers supporting the Model
	//   - Type only: restrict to provider type range (Name is empty)
	//   - Both Type + Name: exact provider specification
	ProviderKey *account.ProviderKey

	// Tags is for tag-based filtering (optional).
	// Only selects accounts/providers containing these tags.
	Tags map[string]string

	// ExcludeAccountIDs contains account IDs to exclude.
	// Used in retry scenarios to skip already-failed accounts.
	ExcludeAccountIDs []string
}

// IsExactProvider returns whether an exact provider is specified (both Type and Name are non-empty).
func (r *SelectRequest) IsExactProvider() bool {
	return r.ProviderKey != nil && r.ProviderKey.Type != "" && r.ProviderKey.Name != ""
}

// IsProviderTypeOnly returns whether only the provider type is specified (Type non-empty, Name empty).
func (r *SelectRequest) IsProviderTypeOnly() bool {
	return r.ProviderKey != nil && r.ProviderKey.Type != "" && r.ProviderKey.Name == ""
}

// IsAutoSelect returns whether selection is fully automatic (ProviderKey is nil).
func (r *SelectRequest) IsAutoSelect() bool {
	return r.ProviderKey == nil
}
