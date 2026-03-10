package usagetracker

import (
	"context"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-client/usagerule"
)

// UsageTracker 用量追踪器接口。
// 结合远端快照和本地计数，实现对账号用量的实时估算，
// 并支持 Resolver 层提前过滤已达限流阈值的账号。
type UsageTracker interface {
	// RecordUsage 记录一次使用。
	// amount: 使用量（请求次数为 1.0，token 类则为实际 token 数）。
	RecordUsage(ctx context.Context, accountID string, sourceType usagerule.SourceType, amount float64) error

	// IsQuotaAvailable 预判指定账号是否还有配额（所有规则都未触及安全阈值）。
	// 供 Resolver 层在选账号前调用，提前过滤已达限流阈值的账号。
	IsQuotaAvailable(ctx context.Context, accountID string) (bool, error)

	// Calibrate 用远端真实数据校准本地计数（HealthChecker 获取到最新 UsageStats 后调用）。
	Calibrate(ctx context.Context, accountID string, stats []*usagerule.UsageStats) error

	// CalibrateFromResponse 用请求响应中的限流信息做 double check。
	// 当实际请求返回 429 时调用，将对应规则标记为已耗尽。
	CalibrateFromResponse(ctx context.Context, accountID string, sourceType usagerule.SourceType) error

	// GetTrackedUsages 获取指定账号的所有追踪数据（供外部查询/调试）。
	GetTrackedUsages(ctx context.Context, accountID string) ([]*account.TrackedUsage, error)

	// MinRemainRatio 获取最小剩余比例（供 Selector 策略使用）。
	MinRemainRatio(ctx context.Context, accountID string) (float64, error)

	// InitRules 为账号初始化追踪规则（账号注册时调用）。
	InitRules(ctx context.Context, accountID string, rules []*usagerule.UsageRule) error

	// Remove 移除账号的追踪数据（账号注销时调用）。
	Remove(ctx context.Context, accountID string) error
}
