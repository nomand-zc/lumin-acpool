package circuitbreaker

import (
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
)

// CircuitBreaker is the circuit breaker interface.
// It determines whether to trip the circuit based on consecutive failure counts.
type CircuitBreaker interface {
	// RecordSuccess records a successful call and resets the consecutive failure count.
	RecordSuccess(acct *account.Account)

	// RecordFailure records a failed call.
	// Returns whether the circuit is tripped (true means the account should switch to CircuitOpen status).
	RecordFailure(acct *account.Account) (tripped bool)

	// ShouldAllow checks whether a circuit-broken account can attempt a half-open probe,
	// i.e., whether the circuit breaker timeout window has elapsed.
	ShouldAllow(acct *account.Account) bool
}

// Config holds the circuit breaker configuration.
type Config struct {
	// Threshold is the consecutive failure count threshold to trip the circuit (default 5).
	Threshold int
	// Timeout is the circuit breaker recovery time window (default 60s).
	// After tripping, the circuit enters half-open state after Timeout, allowing one probe request.
	Timeout time.Duration
}

// DefaultConfig returns the default circuit breaker configuration.
func DefaultConfig() Config {
	return Config{
		Threshold: 5,
		Timeout:   60 * time.Second,
	}
}
