package pool

import (
	"time"

	"github.com/nomand-zc/lumin-acpool/circuitbreaker"
	"github.com/nomand-zc/lumin-acpool/cooldown"
	"github.com/nomand-zc/lumin-acpool/health"
	"github.com/nomand-zc/lumin-acpool/selector"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-acpool/usagetracker"
)

var defaultOptions = Options{
	RecoveryCheckInterval:      5 * time.Second,
	CredentialCheckInterval:    30 * time.Second,
	UsageCheckInterval:         5 * time.Minute,
	ModelDiscoveryInterval:     10 * time.Minute,
	UsageRulesRefreshInterval:  10 * time.Minute,
	CredentialRefreshThreshold: 5 * time.Minute,
	UsageWarningThreshold:      0.1,
}

// Option 是 Pool 的配置选项。
type Option func(*Options)

// Options 配置选项结构体。
type Options struct {
	// --- 存储层（可选，未提供时使用内存实现） ---

	// AccountStorage 账号存储。
	AccountStorage storage.AccountStorage
	// ProviderStorage Provider 存储。
	ProviderStorage storage.ProviderStorage
	// StatsStore 运行时统计存储。
	StatsStore storage.StatsStore

	// --- 可选组件 ---

	// CooldownManager 冷却管理器。
	CooldownManager cooldown.CooldownManager
	// CircuitBreaker 熔断器。
	CircuitBreaker circuitbreaker.CircuitBreaker
	// UsageTracker 用量追踪器。
	UsageTracker usagetracker.UsageTracker

	// --- 策略 ---

	// Selector 账号级选择策略（可选，默认 RoundRobin）。
	Selector selector.Selector
	// GroupSelector Provider 级选择策略（可选，默认 Priority）。
	GroupSelector selector.GroupSelector

	// --- Balancer 参数 ---

	// DefaultMaxRetries 默认最大重试次数。
	DefaultMaxRetries int
	// DefaultEnableFailover 是否默认开启故障转移。
	DefaultEnableFailover bool

	// --- HealthChecker ---

	// HealthChecker 外部注入的健康检查器（可选，未提供时自动创建）。
	HealthChecker health.HealthChecker

	// --- HealthCheck 间隔配置 ---

	// RecoveryCheckInterval 冷却/熔断恢复检查间隔（默认 5s）。
	RecoveryCheckInterval time.Duration
	// CredentialCheckInterval 凭证检查间隔（默认 30s）。
	CredentialCheckInterval time.Duration
	// UsageCheckInterval 用量检查间隔（默认 5m）。
	UsageCheckInterval time.Duration
	// CredentialRefreshThreshold 凭证提前刷新阈值（默认 5m）。
	CredentialRefreshThreshold time.Duration
	// UsageWarningThreshold 用量警告阈值比例（默认 0.1，即剩余 10% 时警告）。
	UsageWarningThreshold float64
	// ModelDiscoveryInterval 模型发现检查间隔（默认 10m）。
	ModelDiscoveryInterval time.Duration
	// UsageRulesRefreshInterval 用量规则刷新检查间隔（默认 10m）。
	UsageRulesRefreshInterval time.Duration
}

// WithAccountStorage 设置账号存储。
func WithAccountStorage(s storage.AccountStorage) Option {
	return func(o *Options) { o.AccountStorage = s }
}

// WithProviderStorage 设置 Provider 存储。
func WithProviderStorage(s storage.ProviderStorage) Option {
	return func(o *Options) { o.ProviderStorage = s }
}

// WithStatsStore 设置运行时统计存储。
func WithStatsStore(ss storage.StatsStore) Option {
	return func(o *Options) { o.StatsStore = ss }
}

// WithCooldownManager 设置冷却管理器。
func WithCooldownManager(cm cooldown.CooldownManager) Option {
	return func(o *Options) { o.CooldownManager = cm }
}

// WithCircuitBreaker 设置熔断器。
func WithCircuitBreaker(cb circuitbreaker.CircuitBreaker) Option {
	return func(o *Options) { o.CircuitBreaker = cb }
}

// WithUsageTracker 设置用量追踪器。
func WithUsageTracker(ut usagetracker.UsageTracker) Option {
	return func(o *Options) { o.UsageTracker = ut }
}

// WithSelector 设置账号级选择策略。
func WithSelector(s selector.Selector) Option {
	return func(o *Options) { o.Selector = s }
}

// WithGroupSelector 设置 Provider 级选择策略。
func WithGroupSelector(s selector.GroupSelector) Option {
	return func(o *Options) { o.GroupSelector = s }
}

// WithDefaultMaxRetries 设置默认最大重试次数。
func WithDefaultMaxRetries(n int) Option {
	return func(o *Options) { o.DefaultMaxRetries = n }
}

// WithDefaultFailover 设置是否默认开启故障转移。
func WithDefaultFailover(enable bool) Option {
	return func(o *Options) { o.DefaultEnableFailover = enable }
}

// WithHealthChecker 外部注入健康检查器（跳过自动创建和默认 check 项注册）。
func WithHealthChecker(hc health.HealthChecker) Option {
	return func(o *Options) { o.HealthChecker = hc }
}

// WithRecoveryCheckInterval 设置冷却/熔断恢复检查间隔。
func WithRecoveryCheckInterval(d time.Duration) Option {
	return func(o *Options) { o.RecoveryCheckInterval = d }
}

// WithCredentialCheckInterval 设置凭证检查间隔。
func WithCredentialCheckInterval(d time.Duration) Option {
	return func(o *Options) { o.CredentialCheckInterval = d }
}

// WithUsageCheckInterval 设置用量检查间隔。
func WithUsageCheckInterval(d time.Duration) Option {
	return func(o *Options) { o.UsageCheckInterval = d }
}

// WithCredentialRefreshThreshold 设置凭证提前刷新阈值。
func WithCredentialRefreshThreshold(d time.Duration) Option {
	return func(o *Options) { o.CredentialRefreshThreshold = d }
}

// WithUsageWarningThreshold 设置用量警告阈值比例（0.0~1.0）。
func WithUsageWarningThreshold(threshold float64) Option {
	return func(o *Options) { o.UsageWarningThreshold = threshold }
}

// WithModelDiscoveryInterval 设置模型发现检查间隔。
func WithModelDiscoveryInterval(d time.Duration) Option {
	return func(o *Options) { o.ModelDiscoveryInterval = d }
}

// WithUsageRulesRefreshInterval 设置用量规则刷新检查间隔。
func WithUsageRulesRefreshInterval(d time.Duration) Option {
	return func(o *Options) { o.UsageRulesRefreshInterval = d }
}
