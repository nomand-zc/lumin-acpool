package usagetracker

import (
	"context"
	"fmt"

	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-client/usagerule"
)

// QuotaExhaustedCallback 配额耗尽时的回调函数。
// 当 RecordUsage 检测到某条规则的用量达到安全阈值时触发。
// 可通过 WithCallback 选项注入，上层可在此回调中触发冷却，将账号标记为 CoolingDown。
type QuotaExhaustedCallback = func(ctx context.Context, accountID string, rule *usagerule.UsageRule)

// Option 配置选项。
type Option func(*Options)

// Options 配置选项结构体。
type Options struct {
	// SafetyRatio 安全阈值比例，默认 0.95。
	// 当已用量占比超过该阈值时，IsQuotaAvailable 返回 false。
	SafetyRatio float64
	// Store 用量追踪数据存储后端。
	Store storage.UsageStore
	// OnQuotaExhausted 配额耗尽时的回调函数。
	// 当 RecordUsage 检测到某条规则的用量达到安全阈值时触发。
	OnQuotaExhausted QuotaExhaustedCallback
}

var defaultOpts = Options{
	SafetyRatio: 0.95,
}

// WithSafetyRatio 设置安全阈值比例（0.0 ~ 1.0）。
// 默认 0.95，即剩余量 < 5% 时视为不可用。
func WithSafetyRatio(ratio float64) Option {
	return func(o *Options) { o.SafetyRatio = ratio }
}

// WithUsageStore 设置用量追踪数据存储后端。
func WithUsageStore(store storage.UsageStore) Option {
	return func(o *Options) { o.Store = store }
}

// WithCallback 统一的回调注册函数（泛型）。
// 通过传入不同类型的回调函数来配置不同的事件处理。
// 当前支持的回调类型：
//   - QuotaExhaustedCallback: 配额耗尽时触发，上层可在此回调中触发冷却
//
// 新增回调类型时，在类型约束中用 | 追加即可。
//
// 示例:
//
//	usagetracker.WithCallback(usagetracker.QuotaExhaustedCallback(func(ctx context.Context, accountID string, rule *usagerule.UsageRule) {
//	    // 处理配额耗尽
//	}))
func WithCallback[T QuotaExhaustedCallback](cb T) Option {
	return func(o *Options) {
		switch fn := any(cb).(type) {
		case QuotaExhaustedCallback:
			o.OnQuotaExhausted = fn
		default:
			panic(fmt.Sprintf("usagetracker: unsupported callback type: %T", cb))
		}
	}
}
