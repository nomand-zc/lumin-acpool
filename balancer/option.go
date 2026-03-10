package balancer

import (
	"github.com/nomand-zc/lumin-acpool/circuitbreaker"
	"github.com/nomand-zc/lumin-acpool/cooldown"
	"github.com/nomand-zc/lumin-acpool/resolver"
	"github.com/nomand-zc/lumin-acpool/selector"
	groupstrategies "github.com/nomand-zc/lumin-acpool/selector/strategies"
	accountstrategies "github.com/nomand-zc/lumin-acpool/selector/strategies/account"
	"github.com/nomand-zc/lumin-acpool/storage"
)

var defaultOptions = Options{
	DefaultMaxRetries:     0,
	DefaultEnableFailover: false,
	Selector:              accountstrategies.NewRoundRobin(),
	GroupSelector:         groupstrategies.NewGroupPriority(),
}

// Option is a functional option for the load balancer.
type Option func(*Options)

// Options holds the complete configuration of the load balancer.
type Options struct {
	// AccountStorage is the account storage (required, used by ReportSuccess/ReportFailure to update account status).
	AccountStorage storage.AccountStorage
	// ProviderStorage is the provider storage (required when Resolver is not injected, used to build the default StorageResolver).
	ProviderStorage storage.ProviderStorage
	// Resolver is the resolver (optional, defaults to a Storage-based implementation).
	Resolver resolver.Resolver
	// Selector is the account-level selection strategy (default: RoundRobin).
	Selector selector.Selector
	// GroupSelector is the provider-level selection strategy (default: Priority).
	GroupSelector selector.GroupSelector
	// CircuitBreaker is the circuit breaker (optional).
	CircuitBreaker circuitbreaker.CircuitBreaker
	// CooldownManager is the cooldown manager (optional).
	CooldownManager cooldown.CooldownManager
	// DefaultMaxRetries is the default maximum retry count.
	DefaultMaxRetries int
	// DefaultEnableFailover indicates whether failover is enabled by default.
	DefaultEnableFailover bool
}

// WithAccountStorage sets the account storage.
func WithAccountStorage(s storage.AccountStorage) Option {
	return func(o *Options) { o.AccountStorage = s }
}

// WithProviderStorage sets the provider storage.
func WithProviderStorage(s storage.ProviderStorage) Option {
	return func(o *Options) { o.ProviderStorage = s }
}

// WithResolver sets the resolver.
func WithResolver(r resolver.Resolver) Option {
	return func(o *Options) { o.Resolver = r }
}

// WithSelector sets the account-level selection strategy.
func WithSelector(s selector.Selector) Option {
	return func(o *Options) { o.Selector = s }
}

// WithGroupSelector sets the provider-level selection strategy.
func WithGroupSelector(s selector.GroupSelector) Option {
	return func(o *Options) { o.GroupSelector = s }
}

// WithCircuitBreaker sets the circuit breaker.
func WithCircuitBreaker(cb circuitbreaker.CircuitBreaker) Option {
	return func(o *Options) { o.CircuitBreaker = cb }
}

// WithCooldownManager sets the cooldown manager.
func WithCooldownManager(cm cooldown.CooldownManager) Option {
	return func(o *Options) { o.CooldownManager = cm }
}

// WithDefaultMaxRetries sets the default maximum retry count.
func WithDefaultMaxRetries(n int) Option {
	return func(o *Options) { o.DefaultMaxRetries = n }
}

// WithDefaultFailover sets whether failover is enabled by default.
func WithDefaultFailover(enable bool) Option {
	return func(o *Options) { o.DefaultEnableFailover = enable }
}
