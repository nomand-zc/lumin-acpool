package group

import (
	"sync/atomic"

	"github.com/nomand-zc/lumin-acpool/provider"
	"github.com/nomand-zc/lumin-acpool/selector"
)

// GroupRoundRobin is the provider-level round-robin selection strategy.
type GroupRoundRobin struct {
	counter uint64
}

// NewGroupRoundRobin creates a provider-level round-robin strategy instance.
func NewGroupRoundRobin() *GroupRoundRobin {
	return &GroupRoundRobin{}
}

// Name returns the strategy name.
func (g *GroupRoundRobin) Name() string {
	return "group_round_robin"
}

// Select round-robin selects a provider.
func (g *GroupRoundRobin) Select(candidates []*provider.ProviderInfo, _ *selector.SelectRequest) (*provider.ProviderInfo, error) {
	if len(candidates) == 0 {
		return nil, selector.ErrEmptyCandidates
	}

	idx := atomic.AddUint64(&g.counter, 1) - 1
	return candidates[idx%uint64(len(candidates))], nil
}
