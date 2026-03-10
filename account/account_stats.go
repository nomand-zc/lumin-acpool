package account

import "time"

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
