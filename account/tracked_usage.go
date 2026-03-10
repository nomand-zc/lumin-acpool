package account

import (
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
