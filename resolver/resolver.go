package resolver

import (
	"context"

	"github.com/nomand-zc/lumin-acpool/account"
)

// ResolveAccountsRequest 是 ResolveAccounts 方法的请求参数。
type ResolveAccountsRequest struct {
	// Key 是 provider 标识符（Type + Name）。
	Key account.ProviderKey

	// Tags 是标签过滤条件（nil 表示不限制）。
	Tags map[string]string

	// ExcludeIDs 是需要排除的账号 ID 列表。
	ExcludeIDs []string
}

// Resolver is the resolver interface.
// Responsible for resolving available providers and accounts from storage.
// Analogous to the Service Discovery / Resolver layer in microservice architectures.
type Resolver interface {
	// ResolveProvider resolves a specific provider exactly.
	//
	// Parameters:
	//   key   - provider identifier (Type + Name)
	//   model - the requested model name, used to verify provider support for the model
	//
	// Returns:
	//   matching provider info; returns corresponding error if provider doesn't exist, is inactive, or doesn't support the model
	ResolveProvider(ctx context.Context, key account.ProviderKey, model string) (*account.ProviderInfo, error)

	// ResolveProviders resolves active providers that support the specified model.
	//
	// Parameters:
	//   model        - the requested model name
	//   providerType - provider type filter (empty string means no restriction)
	//
	// Returns:
	//   list of matching providers; returns empty slice when no matches
	ResolveProviders(ctx context.Context, model string, providerType string) ([]*account.ProviderInfo, error)

	// ResolveAccounts resolves available accounts under the specified provider.
	//
	// Parameters:
	//   req - account resolve options including provider key, tags filter and exclude IDs
	//
	// Returns:
	//   list of matching accounts; returns empty slice when no matches
	ResolveAccounts(ctx context.Context, req ResolveAccountsRequest) ([]*account.Account, error)
}
