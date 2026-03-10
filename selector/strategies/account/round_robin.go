package account

import (
	"sync/atomic"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/selector"
)

// RoundRobin is the round-robin selection strategy.
// Selects candidate accounts in order, ensuring even usage across accounts.
type RoundRobin struct {
	counter uint64
}

// NewRoundRobin creates a round-robin strategy instance.
func NewRoundRobin() *RoundRobin {
	return &RoundRobin{}
}

// Name returns the strategy name.
func (r *RoundRobin) Name() string {
	return "round_robin"
}

// Select round-robin selects an account.
func (r *RoundRobin) Select(candidates []*account.Account, _ *selector.SelectRequest) (*account.Account, error) {
	if len(candidates) == 0 {
		return nil, selector.ErrEmptyCandidates
	}
	idx := atomic.AddUint64(&r.counter, 1) - 1
	return candidates[idx%uint64(len(candidates))], nil
}
