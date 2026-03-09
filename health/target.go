package health

import (
	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/provider"
	"github.com/nomand-zc/lumin-client/credentials"
	"github.com/nomand-zc/lumin-client/providers"
)

// defaultCheckTarget 是 CheckTarget 接口的默认实现
// 将 Account + ProviderInstance 封装为统一的检查目标
type defaultCheckTarget struct {
	acct     *account.Account
	instance *provider.ProviderInstance
}

// NewCheckTarget 创建一个 CheckTarget 实例
// acct: 被检查的账号
// instance: 该账号所属供应商的运行时实例
func NewCheckTarget(acct *account.Account, instance *provider.ProviderInstance) CheckTarget {
	return &defaultCheckTarget{
		acct:     acct,
		instance: instance,
	}
}

func (t *defaultCheckTarget) AccountID() string {
	return t.acct.ID
}

func (t *defaultCheckTarget) ProviderKey() provider.ProviderKey {
	return provider.ProviderKey{
		Type: t.acct.ProviderType,
		Name: t.acct.ProviderName,
	}
}

func (t *defaultCheckTarget) Credential() credentials.Credential {
	return t.acct.Credential
}

func (t *defaultCheckTarget) Provider() providers.Provider {
	return t.instance.Client
}

func (t *defaultCheckTarget) Account() *account.Account {
	return t.acct
}
