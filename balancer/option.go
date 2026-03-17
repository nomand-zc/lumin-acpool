package balancer

import (
	"github.com/nomand-zc/lumin-acpool/balancer/occupancy"
	"github.com/nomand-zc/lumin-acpool/circuitbreaker"
	"github.com/nomand-zc/lumin-acpool/cooldown"
	"github.com/nomand-zc/lumin-acpool/resolver"
	"github.com/nomand-zc/lumin-acpool/selector"
	accountstrategies "github.com/nomand-zc/lumin-acpool/selector/strategies/account"
	groupstrategies "github.com/nomand-zc/lumin-acpool/selector/strategies/group"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-acpool/usagetracker"
)

var defaultOptions = Options{
	DefaultMaxRetries:     3,
	DefaultEnableFailover: false,
	Selector:              accountstrategies.NewRoundRobin(),
	GroupSelector:         groupstrategies.NewGroupPriority(),
	OccupancyController:   occupancy.NewUnlimited(),
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
	// StatsStore is the runtime statistics store (optional, used for recording call statistics).
	StatsStore storage.StatsStore
	// UsageTracker is the usage tracker (optional, used for quota pre-filtering and usage recording).
	UsageTracker usagetracker.UsageTracker
	// OccupancyController 账号占用控制器（可选）。
	// 控制单个账号的并发使用数量，防止多个请求同时选中同一个账号导致超过其限额或触发限流。
	// 默认为 nil（不限制并发，等价于 occupancy.Unlimited）。
	OccupancyController occupancy.Controller
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

// WithStatsStore sets the runtime statistics store.
func WithStatsStore(ss storage.StatsStore) Option {
	return func(o *Options) { o.StatsStore = ss }
}

// WithUsageTracker sets the usage tracker.
func WithUsageTracker(ut usagetracker.UsageTracker) Option {
	return func(o *Options) { o.UsageTracker = ut }
}

// WithOccupancyController 设置账号占用控制器。
// 用于控制单个账号的并发使用数量，内置策略：
//   - occupancy.NewUnlimited(): 不限制（默认行为）
//   - occupancy.NewFixedLimit(limit): 固定并发上限
//   - occupancy.NewAdaptiveLimit(tracker): 基于配额动态调整
func WithOccupancyController(oc occupancy.Controller) Option {
	return func(o *Options) { o.OccupancyController = oc }
}

// WithDefaultMaxRetries sets the default maximum retry count.
func WithDefaultMaxRetries(n int) Option {
	return func(o *Options) { o.DefaultMaxRetries = n }
}

// WithDefaultFailover sets whether failover is enabled by default.
func WithDefaultFailover(enable bool) Option {
	return func(o *Options) { o.DefaultEnableFailover = enable }
}
