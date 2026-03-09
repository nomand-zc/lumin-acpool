package account

import (
	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/selector"
)

// LeastUsed 最少使用选择策略
// 选择总调用次数最少的候选账号，让各账号的调用量趋于均衡
type LeastUsed struct{}

// NewLeastUsed 创建最少使用策略实例
func NewLeastUsed() *LeastUsed {
	return &LeastUsed{}
}

// Name 返回策略名称
func (l *LeastUsed) Name() string {
	return "least_used"
}

// Select 选择总调用次数最少的账号
// 调用次数相同时，优先选择优先级更高的
func (l *LeastUsed) Select(candidates []*account.Account, _ *selector.SelectRequest) (*account.Account, error) {
	if len(candidates) == 0 {
		return nil, selector.ErrEmptyCandidates
	}

	best := candidates[0]
	for _, acct := range candidates[1:] {
		if acct.TotalCalls < best.TotalCalls {
			best = acct
		} else if acct.TotalCalls == best.TotalCalls && acct.Priority > best.Priority {
			best = acct
		}
	}

	return best, nil
}
