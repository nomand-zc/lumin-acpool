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

// Option 负载均衡器功能选项
type Option func(*Options)

// Options 负载均衡器完整配置
type Options struct {
	// AccountStorage 账号存储（必选）
	AccountStorage storage.AccountStorage
	// ProviderStorage 供应商存储（必选）
	ProviderStorage storage.ProviderStorage
	// Resolver 解析器（可选，默认使用基于 Storage 的实现）
	Resolver resolver.Resolver
	// Selector 账号级选择策略（默认 RoundRobin）
	Selector selector.Selector
	// GroupSelector 供应商级选择策略（默认 Priority）
	GroupSelector selector.GroupSelector
	// CircuitBreaker 熔断器（可选）
	CircuitBreaker circuitbreaker.CircuitBreaker
	// CooldownManager 冷却管理器（可选）
	CooldownManager cooldown.CooldownManager
	// DefaultMaxRetries 默认最大重试次数
	DefaultMaxRetries int
	// DefaultEnableFailover 默认是否启用故障转移
	DefaultEnableFailover bool
}

// WithAccountStorage 设置账号存储
func WithAccountStorage(s storage.AccountStorage) Option {
	return func(o *Options) { o.AccountStorage = s }
}

// WithProviderStorage 设置供应商存储
func WithProviderStorage(s storage.ProviderStorage) Option {
	return func(o *Options) { o.ProviderStorage = s }
}

// WithResolver 设置解析器
func WithResolver(r resolver.Resolver) Option {
	return func(o *Options) { o.Resolver = r }
}

// WithSelector 设置账号级选择策略
func WithSelector(s selector.Selector) Option {
	return func(o *Options) { o.Selector = s }
}

// WithGroupSelector 设置供应商级选择策略
func WithGroupSelector(s selector.GroupSelector) Option {
	return func(o *Options) { o.GroupSelector = s }
}

// WithCircuitBreaker 设置熔断器
func WithCircuitBreaker(cb circuitbreaker.CircuitBreaker) Option {
	return func(o *Options) { o.CircuitBreaker = cb }
}

// WithCooldownManager 设置冷却管理器
func WithCooldownManager(cm cooldown.CooldownManager) Option {
	return func(o *Options) { o.CooldownManager = cm }
}

// WithDefaultMaxRetries 设置默认最大重试次数
func WithDefaultMaxRetries(n int) Option {
	return func(o *Options) { o.DefaultMaxRetries = n }
}

// WithDefaultFailover 设置默认是否启用故障转移
func WithDefaultFailover(enable bool) Option {
	return func(o *Options) { o.DefaultEnableFailover = enable }
}
