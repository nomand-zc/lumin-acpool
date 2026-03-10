package group

import (
	"github.com/nomand-zc/lumin-acpool/provider"
	"github.com/nomand-zc/lumin-acpool/selector"
)

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
