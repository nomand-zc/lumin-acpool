package group

import (
	rand "math/rand/v2"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/selector"
)

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
func (g *GroupWeighted) Select(candidates []*account.ProviderInfo, _ *selector.SelectRequest) (*account.ProviderInfo, error) {
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
