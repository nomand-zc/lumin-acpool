package strategies

import (
	"sync/atomic"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/selector"
)

// RoundRobin 轮询选择策略
// 按顺序依次选择候选账号，保证各账号被均匀使用
type RoundRobin struct {
	counter uint64
}

// NewRoundRobin 创建轮询策略实例
func NewRoundRobin() *RoundRobin {
	return &RoundRobin{}
}

// Name 返回策略名称
func (r *RoundRobin) Name() string {
	return "round_robin"
}

// Select 轮询选择一个账号
func (r *RoundRobin) Select(candidates []*account.Account, _ *selector.SelectRequest) (*account.Account, error) {
	if len(candidates) == 0 {
		return nil, selector.ErrEmptyCandidates
	}
	idx := atomic.AddUint64(&r.counter, 1) - 1
	return candidates[idx%uint64(len(candidates))], nil
}
