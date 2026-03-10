package cooldown

import (
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
)

// CooldownManager is the cooldown management interface.
// It manages accounts that need to cool down due to rate limiting or other reasons.
type CooldownManager interface {
	// StartCooldown sets the account to cooling down status.
	// until is the cooldown expiration time; nil means using the default cooldown duration.
	StartCooldown(acct *account.Account, until *time.Time)

	// IsCooldownExpired returns whether the cooldown period has expired.
	IsCooldownExpired(acct *account.Account) bool
}

// Option is a functional option for configuring the default CooldownManager.
type Option func(*Options)

// Options holds the cooldown manager configuration.
type Options struct {
	// DefaultDuration is the default cooldown duration, used when the rate limit response
	// does not provide a cooldown expiration time (default 30s).
	DefaultDuration time.Duration
}

var defaultOptions = Options{
	DefaultDuration: 30 * time.Second,
}

// WithDefaultDuration sets the default cooldown duration.
func WithDefaultDuration(d time.Duration) Option {
	return func(o *Options) { o.DefaultDuration = d }
}
