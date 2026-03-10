package stats

import (
	"context"
	"time"
)

// AccountStats 账号运行时统计数据。
// 从 Account 结构体中独立出来，支持高频原子更新，避免与 AccountStorage 的全量覆盖竞争。
type AccountStats struct {
	// AccountID 关联的账号 ID。
	AccountID string
	// TotalCalls 总调用次数。
	TotalCalls int64
	// SuccessCalls 成功调用次数。
	SuccessCalls int64
	// FailedCalls 失败调用次数。
	FailedCalls int64
	// ConsecutiveFailures 当前连续失败次数（成功时重置为 0）。
	ConsecutiveFailures int
	// LastUsedAt 最后一次被选中使用的时间。
	LastUsedAt *time.Time
	// LastErrorAt 最后一次调用失败的时间。
	LastErrorAt *time.Time
	// LastErrorMsg 最后一次调用失败的错误消息。
	LastErrorMsg string
}

// SuccessRate 计算成功率；无调用时返回 1.0。
func (s *AccountStats) SuccessRate() float64 {
	if s.TotalCalls == 0 {
		return 1.0
	}
	return float64(s.SuccessCalls) / float64(s.TotalCalls)
}

// StatsStore 运行时统计存储接口。
// 支持高频原子更新，避免与 AccountStorage 的全量覆盖竞争。
type StatsStore interface {
	// Get 获取指定账号的运行统计。
	// 如果账号不存在统计记录，返回零值的 AccountStats（不返回错误）。
	Get(ctx context.Context, accountID string) (*AccountStats, error)

	// IncrSuccess 原子递增成功计数，重置连续失败计数，更新 LastUsedAt。
	IncrSuccess(ctx context.Context, accountID string) error

	// IncrFailure 原子递增失败计数和连续失败计数，更新错误信息。
	IncrFailure(ctx context.Context, accountID string, errMsg string) error

	// UpdateLastUsed 更新最后使用时间。
	UpdateLastUsed(ctx context.Context, accountID string, t time.Time) error

	// GetConsecutiveFailures 获取连续失败次数（供 CircuitBreaker 使用）。
	GetConsecutiveFailures(ctx context.Context, accountID string) (int, error)

	// ResetConsecutiveFailures 重置连续失败次数（成功时调用）。
	ResetConsecutiveFailures(ctx context.Context, accountID string) error

	// Remove 删除统计数据（账号注销时调用）。
	Remove(ctx context.Context, accountID string) error
}
