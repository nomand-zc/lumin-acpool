package usagetracker

// Option 配置选项。
type Option func(*options)

type options struct {
	// safetyRatio 安全阈值比例，默认 0.95。
	// 当已用量占比超过该阈值时，IsQuotaAvailable 返回 false。
	safetyRatio float64
	// store 用量追踪数据存储后端。
	store UsageStore
}

var defaultOpts = options{
	safetyRatio: 0.95,
}

// WithSafetyRatio 设置安全阈值比例（0.0 ~ 1.0）。
// 默认 0.95，即剩余量 < 5% 时视为不可用。
func WithSafetyRatio(ratio float64) Option {
	return func(o *options) { o.safetyRatio = ratio }
}

// WithUsageStore 设置用量追踪数据存储后端。
func WithUsageStore(store UsageStore) Option {
	return func(o *options) { o.store = store }
}
