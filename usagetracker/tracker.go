package usagetracker

import (
	"context"
	"time"

	"github.com/nomand-zc/lumin-client/usagerule"
)

// TrackedUsage 单条规则的追踪数据。
// 结合远端快照与本地乐观计数，实现对用量的实时估算。
type TrackedUsage struct {
	// Rule 关联的用量规则。
	Rule *usagerule.UsageRule
	// LocalUsed 本地乐观计数（自上次校准以来的本地累加量）。
	LocalUsed float64
	// RemoteUsed 远端快照已用量。
	RemoteUsed float64
	// RemoteRemain 远端快照剩余量。
	RemoteRemain float64
	// WindowStart 窗口起始时间。
	WindowStart *time.Time
	// WindowEnd 窗口结束时间。
	WindowEnd *time.Time
	// LastSyncAt 上次校准时间。
	LastSyncAt time.Time
}

// EstimatedRemain 估算的实际剩余量。
func (t *TrackedUsage) EstimatedRemain() float64 {
	return t.RemoteRemain - t.LocalUsed
}

// EstimatedUsed 估算的实际已用量。
func (t *TrackedUsage) EstimatedUsed() float64 {
	return t.RemoteUsed + t.LocalUsed
}

// IsExhausted 判断是否已耗尽。
func (t *TrackedUsage) IsExhausted() bool {
	return t.EstimatedRemain() <= 0
}

// RemainRatio 剩余比例（0.0 ~ 1.0）。
func (t *TrackedUsage) RemainRatio() float64 {
	if t.Rule == nil || t.Rule.Total <= 0 {
		return 1.0
	}
	ratio := t.EstimatedRemain() / t.Rule.Total
	if ratio < 0 {
		return 0
	}
	return ratio
}

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
	GetTrackedUsages(ctx context.Context, accountID string) ([]*TrackedUsage, error)

	// MinRemainRatio 获取最小剩余比例（供 Selector 策略使用）。
	MinRemainRatio(ctx context.Context, accountID string) (float64, error)

	// InitRules 为账号初始化追踪规则（账号注册时调用）。
	InitRules(ctx context.Context, accountID string, rules []*usagerule.UsageRule) error

	// Remove 移除账号的追踪数据（账号注销时调用）。
	Remove(ctx context.Context, accountID string) error
}

// UsageStore 用量追踪数据存储接口。
// 支持内存（单机）和 Redis（集群）两种后端。
type UsageStore interface {
	// GetAll 获取指定账号所有规则的追踪数据。
	GetAll(ctx context.Context, accountID string) ([]*TrackedUsage, error)

	// Save 保存指定账号所有规则的追踪数据。
	Save(ctx context.Context, accountID string, usages []*TrackedUsage) error

	// IncrLocalUsed 原子递增指定规则的本地已用量。
	// ruleIndex: 规则索引，amount: 增量。
	IncrLocalUsed(ctx context.Context, accountID string, ruleIndex int, amount float64) error

	// Remove 删除指定账号的追踪数据。
	Remove(ctx context.Context, accountID string) error
}
