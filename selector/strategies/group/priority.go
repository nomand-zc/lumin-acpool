package group

import (
	"sort"

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
