package account

import (
	"math/rand/v2"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/selector"
)

// Weighted is the weighted random selection strategy.
// Randomly selects accounts based on their priority as weight.
// Accounts with higher Priority have a greater probability of being selected.
type Weighted struct{}

// NewWeighted creates a weighted random strategy instance.
func NewWeighted() *Weighted {
	return &Weighted{}
}

// Name returns the strategy name.
func (w *Weighted) Name() string {
	return "weighted"
}

// Select randomly selects an account by weight.
// Uses the Priority field as the weight value; Priority <= 0 is treated as 1.
func (w *Weighted) Select(candidates []*account.Account, _ *selector.SelectRequest) (*account.Account, error) {
	if len(candidates) == 0 {
		return nil, selector.ErrEmptyCandidates
	}

	if len(candidates) == 1 {
		return candidates[0], nil
	}

	// Calculate total weight
	totalWeight := 0
	for _, acct := range candidates {
		weight := acct.Priority
		if weight <= 0 {
			weight = 1
		}
		totalWeight += weight
	}

	// Generate random number and select by weight
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

	// Theoretically unreachable, fallback to the last one
	return candidates[len(candidates)-1], nil
}
