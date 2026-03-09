package strategies

import (
	"sort"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/selector"
)

// Priority 优先级选择策略
// 选择优先级最高的候选账号；优先级相同时选第一个
type Priority struct{}

// NewPriority 创建优先级策略实例
func NewPriority() *Priority {
	return &Priority{}
}

// Name 返回策略名称
func (p *Priority) Name() string {
	return "priority"
}

// Select 选择优先级最高的账号
func (p *Priority) Select(candidates []*account.Account, _ *selector.SelectRequest) (*account.Account, error) {
	if len(candidates) == 0 {
		return nil, selector.ErrEmptyCandidates
	}

	// 按优先级降序排列
	sorted := make([]*account.Account, len(candidates))
	copy(sorted, candidates)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Priority > sorted[j].Priority
	})

	return sorted[0], nil
}
