package strategies

import (
	"math/rand/v2"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/selector"
)

// Weighted 加权随机选择策略
// 根据账号的优先级作为权重，按权重概率随机选择账号
// Priority 越高的账号被选中概率越大
type Weighted struct{}

// NewWeighted 创建加权随机策略实例
func NewWeighted() *Weighted {
	return &Weighted{}
}

// Name 返回策略名称
func (w *Weighted) Name() string {
	return "weighted"
}

// Select 按权重随机选择一个账号
// 使用 Priority 字段作为权重值，Priority <= 0 的按 1 处理
func (w *Weighted) Select(candidates []*account.Account, _ *selector.SelectRequest) (*account.Account, error) {
	if len(candidates) == 0 {
		return nil, selector.ErrEmptyCandidates
	}

	if len(candidates) == 1 {
		return candidates[0], nil
	}

	// 计算权重总和
	totalWeight := 0
	for _, acct := range candidates {
		weight := acct.Priority
		if weight <= 0 {
			weight = 1
		}
		totalWeight += weight
	}

	// 生成随机数并按权重选择
	r := rand.IntN(totalWeight)
	cumulative := 0
	for _, acct := range candidates {
		weight := acct.Priority
		if weight <= 0 {
			weight = 1
		}
		cumulative += weight
		if r < cumulative {
			return acct, nil
		}
	}

	// 理论上不会到这里，兜底返回最后一个
	return candidates[len(candidates)-1], nil
}
