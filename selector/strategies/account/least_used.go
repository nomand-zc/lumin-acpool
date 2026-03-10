package account

import (
	"context"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/selector"
	"github.com/nomand-zc/lumin-acpool/storage"
)

// LeastUsed 最少使用选择策略。
// 从 StatsStore 获取各候选账号的 TotalCalls，选择调用次数最少的账号。
// 调用次数相同时，按 Priority 降序选择（优先级高的优先）。
//
// 如果未注入 StatsStore，则退化为基于 Priority 的选择。
type LeastUsed struct {
	statsStore storage.StatsStore
}

// NewLeastUsed 创建一个最少使用策略实例。
// statsStore 为可选参数，传 nil 时退化为 Priority 选择。
func NewLeastUsed(statsStore storage.StatsStore) *LeastUsed {
	return &LeastUsed{statsStore: statsStore}
}

// Name returns the strategy name.
func (l *LeastUsed) Name() string {
	return "least_used"
}

// Select 选择调用次数最少的账号。
// 从 StatsStore 获取 TotalCalls，选择最少使用的账号；
// 调用次数相同时按 Priority 降序选择。
func (l *LeastUsed) Select(candidates []*account.Account, _ *selector.SelectRequest) (*account.Account, error) {
	if len(candidates) == 0 {
		return nil, selector.ErrEmptyCandidates
	}

	// 无 StatsStore 时退化为 Priority 选择
	if l.statsStore == nil {
		return l.selectByPriority(candidates), nil
	}

	ctx := context.Background()
	best := candidates[0]
	bestCalls := l.getTotalCalls(ctx, best.ID)

	for _, acct := range candidates[1:] {
		calls := l.getTotalCalls(ctx, acct.ID)
		if calls < bestCalls || (calls == bestCalls && acct.Priority > best.Priority) {
			best = acct
			bestCalls = calls
		}
	}

	return best, nil
}

// getTotalCalls 从 StatsStore 获取账号的总调用次数。
// 获取失败时返回 0，不影响选择流程。
func (l *LeastUsed) getTotalCalls(ctx context.Context, accountID string) int64 {
	stats, err := l.statsStore.Get(ctx, accountID)
	if err != nil {
		return 0
	}
	return stats.TotalCalls
}

// selectByPriority 按 Priority 降序选择（退化行为）。
func (l *LeastUsed) selectByPriority(candidates []*account.Account) *account.Account {
	best := candidates[0]
	for _, acct := range candidates[1:] {
		if acct.Priority > best.Priority {
			best = acct
		}
	}
	return best
}
