package account

import (
	"sort"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/selector"
)

// Priority is the priority selection strategy.
// Selects the candidate account with the highest priority; when priorities are equal, selects the first one.
type Priority struct{}

// NewPriority creates a priority strategy instance.
func NewPriority() *Priority {
	return &Priority{}
}

// Name returns the strategy name.
func (p *Priority) Name() string {
	return "priority"
}

// Select selects the account with the highest priority.
func (p *Priority) Select(candidates []*account.Account, _ *selector.SelectRequest) (*account.Account, error) {
	if len(candidates) == 0 {
		return nil, selector.ErrEmptyCandidates
	}

	if len(candidates) == 1 {
		return candidates[0], nil
	}

	// Sort by priority descending
	sorted := make([]*account.Account, len(candidates))
	copy(sorted, candidates)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Priority > sorted[j].Priority
	})

	return sorted[0], nil
}
