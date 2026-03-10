package group

import (
	rand "math/rand/v2"
	"sort"
	"sync/atomic"

	"github.com/nomand-zc/lumin-acpool/provider"
	"github.com/nomand-zc/lumin-acpool/selector"
)

// GroupPriority is the provider-level priority selection strategy.
// Selects the provider with the highest priority; when priorities are equal, selects the one with the most available accounts.
type GroupPriority struct{}

// NewGroupPriority creates a provider-level priority strategy instance.
func NewGroupPriority() *GroupPriority {
	return &GroupPriority{}
}

// Name returns the strategy name.
func (g *GroupPriority) Name() string {
	return "group_priority"
}

// Select selects the provider with the highest priority.
func (g *GroupPriority) Select(candidates []*provider.ProviderInfo, _ *selector.SelectRequest) (*provider.ProviderInfo, error) {
	if len(candidates) == 0 {
		return nil, selector.ErrEmptyCandidates
	}

	sorted := make([]*provider.ProviderInfo, len(candidates))
	copy(sorted, candidates)
	sort.SliceStable(sorted, func(i, j int) bool {
		// Sort by priority descending first
		if sorted[i].Priority != sorted[j].Priority {
			return sorted[i].Priority > sorted[j].Priority
		}
		// When priority is equal, sort by available account count descending
		return sorted[i].AvailableAccountCount > sorted[j].AvailableAccountCount
	})

	return sorted[0], nil
}

// GroupMostAvailable is the provider-level most-available selection strategy.
// Selects the provider with the most available accounts, suitable for load balancing scenarios.
type GroupMostAvailable struct{}

// NewGroupMostAvailable creates a provider-level most-available strategy instance.
func NewGroupMostAvailable() *GroupMostAvailable {
	return &GroupMostAvailable{}
}

// Name returns the strategy name.
func (g *GroupMostAvailable) Name() string {
	return "group_most_available"
}

// Select selects the provider with the most available accounts.
func (g *GroupMostAvailable) Select(candidates []*provider.ProviderInfo, _ *selector.SelectRequest) (*provider.ProviderInfo, error) {
	if len(candidates) == 0 {
		return nil, selector.ErrEmptyCandidates
	}

	best := candidates[0]
	for _, p := range candidates[1:] {
		if p.AvailableAccountCount > best.AvailableAccountCount {
			best = p
		} else if p.AvailableAccountCount == best.AvailableAccountCount && p.Priority > best.Priority {
			best = p
		}
	}

	return best, nil
}

// GroupWeighted is the provider-level weighted random selection strategy.
// Randomly selects by provider weight.
type GroupWeighted struct{}

// NewGroupWeighted creates a provider-level weighted random strategy instance.
func NewGroupWeighted() *GroupWeighted {
	return &GroupWeighted{}
}

// Name returns the strategy name.
func (g *GroupWeighted) Name() string {
	return "group_weighted"
}

// Select randomly selects a provider by weight.
func (g *GroupWeighted) Select(candidates []*provider.ProviderInfo, _ *selector.SelectRequest) (*provider.ProviderInfo, error) {
	if len(candidates) == 0 {
		return nil, selector.ErrEmptyCandidates
	}

	if len(candidates) == 1 {
		return candidates[0], nil
	}

	totalWeight := 0
	for _, p := range candidates {
		w := p.Weight
		if w <= 0 {
			w = 1
		}
		totalWeight += w
	}

	r := rand.IntN(totalWeight)
	cumulative := 0
	for _, p := range candidates {
		w := p.Weight
		if w <= 0 {
			w = 1
		}
		cumulative += w
		if r < cumulative {
			return p, nil
		}
	}

	return candidates[len(candidates)-1], nil
}

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
