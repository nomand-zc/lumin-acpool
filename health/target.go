package health

import (
	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-client/credentials"
	"github.com/nomand-zc/lumin-client/providers"
)

// defaultCheckTarget is the default implementation of the CheckTarget interface.
// It wraps Account + Provider SDK Client into a unified check target.
type defaultCheckTarget struct {
	acct   *account.Account
	client providers.Provider
}

// NewCheckTarget creates a CheckTarget instance.
// acct: the account to be checked.
// client: the provider SDK client for making API calls.
func NewCheckTarget(acct *account.Account, client providers.Provider) CheckTarget {
	return &defaultCheckTarget{
		acct:   acct,
		client: client,
	}
}

func (t *defaultCheckTarget) Credential() credentials.Credential {
	return t.acct.Credential
}

func (t *defaultCheckTarget) Client() providers.Provider {
	return t.client
}

func (t *defaultCheckTarget) Account() *account.Account {
	return t.acct
}
