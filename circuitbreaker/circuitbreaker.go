package circuitbreaker

import (
	"context"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/stats"
)

// CircuitBreaker is the circuit breaker interface.
// It determines whether to trip the circuit based on consecutive failure counts.
// 支持基于账号 UsageRules 动态计算阈值和超时时间。
type CircuitBreaker interface {
	// RecordSuccess records a successful call and resets the consecutive failure count.
	RecordSuccess(ctx context.Context, acct *account.Account) error

	// RecordFailure records a failed call.
	// Returns whether the circuit is tripped (true means the account should switch to CircuitOpen status).
	RecordFailure(ctx context.Context, acct *account.Account) (tripped bool, err error)

	// ShouldAllow checks whether a circuit-broken account can attempt a half-open probe,
	// i.e., whether the circuit breaker timeout window has elapsed.
	ShouldAllow(acct *account.Account) bool
}

// Option is a functional option for configuring the default CircuitBreaker.
type Option func(*options)

// options holds the circuit breaker configuration.
type options struct {
	// defaultThreshold 默认连续失败阈值（当账号无 UsageRules 时使用，默认 5）。
	defaultThreshold int
	// defaultTimeout 默认熔断恢复时间窗口（当账号无 UsageRules 时使用，默认 60s）。
	defaultTimeout time.Duration
	// thresholdRatio 动态阈值比例（取规则 Total 的比例，默认 0.5 即 50%）。
	thresholdRatio float64
	// minThreshold 最小阈值（动态计算后不低于此值，默认 3）。
	minThreshold int
	// statsStore 运行时统计存储，用于读写 ConsecutiveFailures。
	statsStore stats.StatsStore
}

var defaultOptions = options{
	defaultThreshold: 5,
	defaultTimeout:   60 * time.Second,
	thresholdRatio:   0.5,
	minThreshold:     3,
}

// WithDefaultThreshold sets the default consecutive failure count threshold.
func WithDefaultThreshold(n int) Option {
	return func(o *options) { o.defaultThreshold = n }
}

// WithDefaultTimeout sets the default circuit breaker recovery time window.
func WithDefaultTimeout(d time.Duration) Option {
	return func(o *options) { o.defaultTimeout = d }
}

// WithThresholdRatio sets the dynamic threshold ratio (proportion of rule Total).
func WithThresholdRatio(ratio float64) Option {
	return func(o *options) { o.thresholdRatio = ratio }
}

// WithMinThreshold sets the minimum threshold for dynamic calculation.
func WithMinThreshold(n int) Option {
	return func(o *options) { o.minThreshold = n }
}

// WithStatsStore sets the runtime statistics store for reading/writing ConsecutiveFailures.
func WithStatsStore(store stats.StatsStore) Option {
	return func(o *options) { o.statsStore = store }
}
