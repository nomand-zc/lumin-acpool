package balancer

import (
	"context"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/provider"
)

// Balancer is the load balancer interface.
// It orchestrates the complete "filter providers → select provider → filter accounts → select account" flow
// and manages call result reporting (driving circuit breaker/cooldown state transitions).
type Balancer interface {

	// Pick selects an available account from candidates, returning the selected account and provider info.
	//
	// Dispatch modes (determined by PickRequest.ProviderKey):
	//   Mode 1 - Exact provider (both ProviderKey.Type and Name are non-empty)
	//     → Directly select from available accounts under the specified provider.
	//   Mode 2 - Filter by type (only ProviderKey.Type is non-empty)
	//     → Use GroupSelector to pick a provider from all active providers of that type supporting the Model, then select an account.
	//   Mode 3 - Fully automatic (ProviderKey is nil)
	//     → Use GroupSelector to pick a provider from all active providers supporting the Model, then select an account.
	//
	// Failover:
	//   When EnableFailover=true and no available accounts exist under the current provider,
	//   automatically exclude that provider and re-select from remaining candidates until success or candidates exhausted.
	//
	// Retry:
	//   When MaxRetries>0, on selection failure, exclude already-tried account IDs and re-select,
	//   until success or retries exhausted.
	Pick(ctx context.Context, req *PickRequest) (*PickResult, error)

	// ReportSuccess reports a successful call.
	//
	// Behavior:
	//   1. Update account stats: TotalCalls++, SuccessCalls++, ConsecutiveFailures=0
	//   2. Notify CircuitBreaker.RecordSuccess (if configured)
	//   3. Persist to AccountStorage
	ReportSuccess(ctx context.Context, accountID string) error

	// ReportFailure reports a failed call.
	//
	// Behavior:
	//   1. Update account stats: TotalCalls++, FailedCalls++, ConsecutiveFailures++
	//   2. Notify CircuitBreaker.RecordFailure (if configured)
	//      - If circuit trips → status switches to CircuitOpen, CircuitOpenUntil is set
	//   3. Check if it's a rate limit error → Notify CooldownManager.StartCooldown (if configured)
	//      - Status switches to CoolingDown, CooldownUntil is set
	//   4. Persist to AccountStorage
	//
	// callErr: the actual call error, used to determine error type (e.g., rate limit vs server error)
	ReportFailure(ctx context.Context, accountID string, callErr error) error
}

// PickRequest represents a selection request.
type PickRequest struct {
	// Model is the requested model name (required).
	Model string

	// ProviderKey is the provider locator (optional, pointer type).
	//   - nil: fully automatic selection
	//   - Type only: restrict to provider type
	//   - Both Type + Name: exact provider specification
	ProviderKey *provider.ProviderKey

	// Tags is for tag-based filtering (optional).
	Tags map[string]string

	// MaxRetries is the maximum retry count for this request (overrides global config, 0 = no retry).
	MaxRetries int

	// EnableFailover indicates whether to enable failover.
	// When no available accounts exist under a provider, automatically try the next candidate provider.
	EnableFailover bool
}

// PickResult represents the selection result.
type PickResult struct {
	// Account is the selected account (deep copy).
	Account *account.Account

	// ProviderKey is the identifier of the selected provider.
	ProviderKey provider.ProviderKey

	// Attempts is the total number of attempts (including retries).
	Attempts int
}
