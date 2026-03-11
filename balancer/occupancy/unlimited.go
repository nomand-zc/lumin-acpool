package occupancy

import (
	"context"

	"github.com/nomand-zc/lumin-acpool/account"
)

// 编译期接口合规性检查。
var _ Controller = (*Unlimited)(nil)

// Unlimited 不限制并发的占用控制器。
// 所有操作均为空操作，等价于未配置 Controller 时的默认行为。
// 适用于不需要并发控制的场景。
type Unlimited struct{}

// NewUnlimited 创建一个不限制并发的占用控制器。
func NewUnlimited() *Unlimited {
	return &Unlimited{}
}

func (u *Unlimited) FilterAvailable(_ context.Context, accounts []*account.Account) []*account.Account {
	return accounts
}

func (u *Unlimited) Acquire(_ context.Context, _ *account.Account) bool {
	return true
}

func (u *Unlimited) Release(_ context.Context, _ string) {}
