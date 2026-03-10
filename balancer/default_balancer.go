package balancer

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/provider"
	"github.com/nomand-zc/lumin-acpool/resolver"
	"github.com/nomand-zc/lumin-acpool/selector"
	"github.com/nomand-zc/lumin-acpool/storage"
)

// defaultBalancer is the default implementation of the Balancer interface.
type defaultBalancer struct {
	opts Options
}

// New creates a load balancer instance.
func New(opts ...Option) (Balancer, error) {
	o := defaultOptions
	for _, opt := range opts {
		opt(&o)
	}

	// Validate required dependencies: AccountStorage is always needed (for ReportSuccess/ReportFailure)
	if o.AccountStorage == nil {
		return nil, fmt.Errorf("balancer: AccountStorage is required")
	}

	// If Resolver is not set, use the default Storage-based implementation (requires ProviderStorage)
	if o.Resolver == nil {
		if o.ProviderStorage == nil {
			return nil, fmt.Errorf("balancer: either Resolver or ProviderStorage is required")
		}
		o.Resolver = resolver.NewStorageResolver(o.ProviderStorage, o.AccountStorage)
	}

	return &defaultBalancer{opts: o}, nil
}

// Pick performs a single selection.
func (b *defaultBalancer) Pick(ctx context.Context, req *PickRequest) (*PickResult, error) {
	if req.Model == "" {
		return nil, ErrModelRequired
	}

	// Build SelectRequest (to be passed to Selector/GroupSelector)
	selReq := &selector.SelectRequest{
		Model:       req.Model,
		ProviderKey: req.ProviderKey,
		Tags:        req.Tags,
	}

	// Determine the maximum retry count
	maxRetries := req.MaxRetries
	if maxRetries <= 0 {
		maxRetries = b.opts.DefaultMaxRetries
	}

	enableFailover := req.EnableFailover || b.opts.DefaultEnableFailover

	// Determine dispatch mode based on the three states of ProviderKey
	switch {
	case selReq.IsExactProvider():
		return b.pickExact(ctx, selReq, maxRetries)
	default:
		return b.pickAuto(ctx, selReq, maxRetries, enableFailover)
	}
}

// pickExact handles Mode 1: exact provider specification.
func (b *defaultBalancer) pickExact(ctx context.Context, selReq *selector.SelectRequest, maxRetries int) (*PickResult, error) {
	// Resolve provider via Resolver (unified validation of existence, active status, and model support)
	provInfo, err := b.opts.Resolver.ResolveProvider(ctx, *selReq.ProviderKey, selReq.Model)
	if err != nil {
		switch {
		case errors.Is(err, resolver.ErrProviderNotFound):
			return nil, ErrProviderNotFound
		case errors.Is(err, resolver.ErrProviderInactive):
			return nil, ErrNoAvailableProvider
		case errors.Is(err, resolver.ErrModelNotSupported):
			return nil, ErrModelNotSupported
		default:
			return nil, fmt.Errorf("balancer: resolve provider: %w", err)
		}
	}

	// Select an account from this provider (with retry)
	return b.selectAccountFromProvider(ctx, provInfo, selReq, maxRetries)
}

// pickAuto handles Mode 2/3: selection by type or fully automatic.
func (b *defaultBalancer) pickAuto(ctx context.Context, selReq *selector.SelectRequest, maxRetries int, enableFailover bool) (*PickResult, error) {
	// Resolve candidate provider list via Resolver
	providerType := ""
	if selReq.IsProviderTypeOnly() {
		providerType = selReq.ProviderKey.Type
	}
	candidates, err := b.opts.Resolver.ResolveProviders(ctx, selReq.Model, providerType)
	if err != nil {
		return nil, fmt.Errorf("balancer: resolve providers: %w", err)
	}

	if len(candidates) == 0 {
		return nil, ErrModelNotSupported
	}

	// Exclude already-tried providers (failover scenario)
	var excludeProviderKeys []provider.ProviderKey

	for {
		// Filter out excluded providers
		filtered := filterProviders(candidates, excludeProviderKeys)
		if len(filtered) == 0 {
			return nil, ErrNoAvailableProvider
		}

		// Use GroupSelector to select a provider
		chosen, err := b.opts.GroupSelector.Select(filtered, selReq)
		if err != nil {
			return nil, fmt.Errorf("balancer: group select: %w", err)
		}

		// Select an account from this provider
		result, err := b.selectAccountFromProvider(ctx, chosen, selReq, maxRetries)
		if err == nil {
			return result, nil
		}

		// If selection failed and failover is enabled, exclude this provider and retry
		if enableFailover && (errors.Is(err, ErrNoAvailableAccount) || errors.Is(err, ErrMaxRetriesExceeded)) {
			excludeProviderKeys = append(excludeProviderKeys, chosen.ProviderKey())
			// Reset ExcludeAccountIDs (no need to exclude accounts from previous provider after switching)
			selReq.ExcludeAccountIDs = nil
			continue
		}

		return nil, err
	}
}

// selectAccountFromProvider selects an account from the specified provider (with retry).
func (b *defaultBalancer) selectAccountFromProvider(
	ctx context.Context,
	provInfo *provider.ProviderInfo,
	selReq *selector.SelectRequest,
	maxRetries int,
) (*PickResult, error) {
	// Save the original ExcludeAccountIDs
	originalExclude := selReq.ExcludeAccountIDs

	for i := 0; i <= maxRetries; i++ {
		// Resolve available accounts under this provider via Resolver
		accounts, err := b.opts.Resolver.ResolveAccounts(ctx, provInfo.ProviderKey(), selReq.Tags, selReq.ExcludeAccountIDs)
		if err != nil {
			selReq.ExcludeAccountIDs = originalExclude
			return nil, fmt.Errorf("balancer: resolve accounts: %w", err)
		}

		if len(accounts) == 0 {
			// Restore the original exclude list
			selReq.ExcludeAccountIDs = originalExclude
			return nil, ErrNoAvailableAccount
		}

		// Use Selector to select an account
		chosen, err := b.opts.Selector.Select(accounts, selReq)
		if err != nil {
			if errors.Is(err, selector.ErrEmptyCandidates) || errors.Is(err, selector.ErrNoAvailableAccount) {
				selReq.ExcludeAccountIDs = originalExclude
				return nil, ErrNoAvailableAccount
			}
			selReq.ExcludeAccountIDs = originalExclude
			return nil, fmt.Errorf("balancer: select account: %w", err)
		}

		// Update LastUsedAt
		now := time.Now()
		chosen.LastUsedAt = &now
		chosen.UpdatedAt = now
		if err := b.opts.AccountStorage.Update(ctx, chosen); err != nil {
			// 更新失败，将该账号加入排除列表后重试
			selReq.ExcludeAccountIDs = append(selReq.ExcludeAccountIDs, chosen.ID)
			continue
		}

		// Restore the exclude list
		selReq.ExcludeAccountIDs = originalExclude

		return &PickResult{
			Account:     deepCopyAccount(chosen),
			ProviderKey: provInfo.ProviderKey(),
			Attempts:    i,
		}, nil
	}

	// 重试次数耗尽
	selReq.ExcludeAccountIDs = originalExclude
	return nil, ErrMaxRetriesExceeded
}

// ReportSuccess reports a successful call.
func (b *defaultBalancer) ReportSuccess(ctx context.Context, accountID string) error {
	acct, err := b.opts.AccountStorage.Get(ctx, accountID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return ErrAccountNotFound
		}
		return fmt.Errorf("balancer: get account: %w", err)
	}

	// Update statistics
	acct.TotalCalls++
	acct.SuccessCalls++
	acct.ConsecutiveFailures = 0
	now := time.Now()
	acct.UpdatedAt = now

	// Notify circuit breaker
	if b.opts.CircuitBreaker != nil {
		b.opts.CircuitBreaker.RecordSuccess(acct)
	}

	// Persist
	if err := b.opts.AccountStorage.Update(ctx, acct); err != nil {
		return fmt.Errorf("balancer: update account: %w", err)
	}

	return nil
}

// ReportFailure reports a failed call.
func (b *defaultBalancer) ReportFailure(ctx context.Context, accountID string, callErr error) error {
	acct, err := b.opts.AccountStorage.Get(ctx, accountID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return ErrAccountNotFound
		}
		return fmt.Errorf("balancer: get account: %w", err)
	}

	// Update statistics
	acct.TotalCalls++
	acct.FailedCalls++
	acct.ConsecutiveFailures++
	now := time.Now()
	acct.LastErrorAt = &now
	acct.UpdatedAt = now
	if callErr != nil {
		acct.LastErrorMsg = callErr.Error()
	}

	// Check if it's a rate limit error, prioritize cooldown handling
	if isRateLimitError(callErr) && b.opts.CooldownManager != nil {
		retryAfter := extractRetryAfter(callErr)
		b.opts.CooldownManager.StartCooldown(acct, retryAfter)
		acct.Status = account.StatusCoolingDown
	} else if b.opts.CircuitBreaker != nil {
		// Notify circuit breaker
		tripped := b.opts.CircuitBreaker.RecordFailure(acct)
		if tripped {
			acct.Status = account.StatusCircuitOpen
		}
	}

	// Persist
	if err := b.opts.AccountStorage.Update(ctx, acct); err != nil {
		return fmt.Errorf("balancer: update account: %w", err)
	}

	return nil
}

// --- Helper functions ---

// filterProviders filters out excluded providers.
func filterProviders(candidates []*provider.ProviderInfo, excludeKeys []provider.ProviderKey) []*provider.ProviderInfo {
	if len(excludeKeys) == 0 {
		return candidates
	}
	var result []*provider.ProviderInfo
	for _, p := range candidates {
		excluded := false
		for _, ek := range excludeKeys {
			if p.ProviderType == ek.Type && p.ProviderName == ek.Name {
				excluded = true
				break
			}
		}
		if !excluded {
			result = append(result, p)
		}
	}
	return result
}

// deepCopyAccount creates a deep copy of an account object.
func deepCopyAccount(src *account.Account) *account.Account {
	if src == nil {
		return nil
	}

	dst := *src

	// Copy Tags
	if src.Tags != nil {
		dst.Tags = make(map[string]string, len(src.Tags))
		for k, v := range src.Tags {
			dst.Tags[k] = v
		}
	}

	// Copy Metadata
	if src.Metadata != nil {
		dst.Metadata = make(map[string]any, len(src.Metadata))
		for k, v := range src.Metadata {
			dst.Metadata[k] = v
		}
	}

	// Copy time pointers
	if src.LastUsedAt != nil {
		t := *src.LastUsedAt
		dst.LastUsedAt = &t
	}
	if src.LastErrorAt != nil {
		t := *src.LastErrorAt
		dst.LastErrorAt = &t
	}
	if src.CooldownUntil != nil {
		t := *src.CooldownUntil
		dst.CooldownUntil = &t
	}
	if src.CircuitOpenUntil != nil {
		t := *src.CircuitOpenUntil
		dst.CircuitOpenUntil = &t
	}

	return &dst
}

// rateLimitError is the rate limit error interface.
// Error types implementing this interface will be recognized as rate limit errors by the balancer.
type rateLimitError interface {
	IsRateLimit() bool
}

// httpStatusError is the HTTP status code error interface.
// Used to extract status codes from HTTP errors to determine rate limiting (429).
type httpStatusError interface {
	StatusCode() int
}

// retryAfterError is the error interface carrying retry-after time.
type retryAfterError interface {
	RetryAfter() *time.Time
}

// isRateLimitError checks whether the error is a rate limit error.
func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}

	// Check rateLimitError interface first
	var rle rateLimitError
	if errors.As(err, &rle) {
		return rle.IsRateLimit()
	}

	// Then check for HTTP status code 429
	var hse httpStatusError
	if errors.As(err, &hse) {
		return hse.StatusCode() == http.StatusTooManyRequests
	}

	return false
}

// extractRetryAfter extracts the retry-after time from the error.
func extractRetryAfter(err error) *time.Time {
	if err == nil {
		return nil
	}

	var rae retryAfterError
	if errors.As(err, &rae) {
		return rae.RetryAfter()
	}

	return nil
}
