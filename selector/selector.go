package selector

import (
	"github.com/nomand-zc/lumin-acpool/account"
)

// Selector 账号级选择策略接口
// 从一组已过滤的候选账号中，按策略选出最优的一个
type Selector interface {
	// Name 返回策略名称，用于日志和调试
	Name() string

	// Select 从候选账号中选择一个
	//
	// 参数:
	//   candidates - 已过滤的候选账号列表，状态均为 Available
	//   req        - 本次请求的上下文信息，策略可参考其中字段辅助决策
	//
	// 返回:
	//   被选中的账号；如果无可选账号，返回 ErrNoAvailableAccount
	Select(candidates []*account.Account, req *SelectRequest) (*account.Account, error)
}
