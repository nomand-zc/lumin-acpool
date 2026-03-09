package selector

import (
	"github.com/nomand-zc/lumin-acpool/provider"
)

// GroupSelector 供应商级选择策略接口
// 从一组候选供应商中，按策略选出最优的一个
type GroupSelector interface {
	// Name 返回策略名称
	Name() string

	// Select 从候选供应商中选择一个
	//
	// 参数:
	//   candidates - 已过滤的候选供应商列表，状态均为 Active 或 Degraded
	//   req        - 本次请求的上下文信息
	//
	// 返回:
	//   被选中的供应商；如果无可选供应商，返回 ErrNoAvailableProvider
	Select(candidates []*provider.ProviderInfo, req *SelectRequest) (*provider.ProviderInfo, error)
}
