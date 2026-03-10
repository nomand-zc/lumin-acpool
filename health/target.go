package health

import (
	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-client/credentials"
)

// defaultCheckTarget is the default implementation of the CheckTarget interface.
// It wraps Account + ProviderInstance into a unified check target.
type defaultCheckTarget struct {
	acct     *account.Account
instance *account.ProviderInstance
}

// NewCheckTarget creates a CheckTarget instance.
// acct: the account to be checked.
// instance: the runtime instance of the provider the account belongs to.
func NewCheckTarget(acct *account.Account, instance *account.ProviderInstance) CheckTarget {
	return &defaultCheckTarget{
		acct:     acct,
		instance: instance,
	}
}

func (t *defaultCheckTarget) Credential() credentials.Credential {
	return t.acct.Credential
}

func (t *defaultCheckTarget) ProviderInstance() *account.ProviderInstance {
	return t.instance
}

func (t *defaultCheckTarget) Account() *account.Account {
	return t.acct
}
