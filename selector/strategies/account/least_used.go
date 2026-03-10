package account

import (
	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/selector"
)

// LeastUsed is the least-used selection strategy.
// Selects the candidate account with the fewest total calls, balancing usage across accounts.
type LeastUsed struct{}

// NewLeastUsed creates a least-used strategy instance.
func NewLeastUsed() *LeastUsed {
	return &LeastUsed{}
}

// Name returns the strategy name.
func (l *LeastUsed) Name() string {
	return "least_used"
}

// Select selects the account with the fewest total calls.
// When call counts are equal, prefers higher priority.
func (l *LeastUsed) Select(candidates []*account.Account, _ *selector.SelectRequest) (*account.Account, error) {
	if len(candidates) == 0 {
		return nil, selector.ErrEmptyCandidates
	}

	best := candidates[0]
	for _, acct := range candidates[1:] {
		if acct.TotalCalls < best.TotalCalls {
			best = acct
		} else if acct.TotalCalls == best.TotalCalls && acct.Priority > best.Priority {
			best = acct
		}
	}

	return best, nil
}
