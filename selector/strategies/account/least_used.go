package account

import (
	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/selector"
)

// LeastUsed is the least-used selection strategy.
// Selects the candidate account with the highest priority as default behavior.
// NOTE: 原先基于 Account.TotalCalls 的逻辑已移至 StatsStore，
// 当需要基于调用次数选择时，应使用支持 StatsStore 的增强版本。
// 当前实现退化为基于 Priority 的选择。
type LeastUsed struct{}

// NewLeastUsed creates a least-used strategy instance.
func NewLeastUsed() *LeastUsed {
	return &LeastUsed{}
}

// Name returns the strategy name.
func (l *LeastUsed) Name() string {
	return "least_used"
}

// Select selects the account with the highest priority.
// TODO: 后续增强为从 StatsStore 获取 TotalCalls 做最少使用选择。
func (l *LeastUsed) Select(candidates []*account.Account, _ *selector.SelectRequest) (*account.Account, error) {
	if len(candidates) == 0 {
		return nil, selector.ErrEmptyCandidates
	}

	best := candidates[0]
	for _, acct := range candidates[1:] {
		if acct.Priority > best.Priority {
			best = acct
		}
	}

	return best, nil
}
