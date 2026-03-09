package selector

import (
	"github.com/nomand-zc/lumin-acpool/account"
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

// SelectRequest 选号请求的上下文信息
// 携带本次请求的约束条件，供选择策略参考决策
type SelectRequest struct {
	// Model 请求的模型名称（必填）
	Model string

	// ProviderKey 供应商定位（可选，指针类型）
	//   - nil: 不限制供应商，从所有支持 Model 的活跃供应商中自动选择
	//   - 仅填 Type: 限定供应商类型范围（Name 为空）
	//   - Type + Name 都填: 精确指定供应商
	ProviderKey *provider.ProviderKey

	// Tags 标签过滤（可选）
	// 只选择包含这些标签的账号/供应商
	Tags map[string]string

	// ExcludeAccountIDs 需要排除的账号 ID
	// 用于重试场景跳过已经失败的账号
	ExcludeAccountIDs []string
}

// IsExactProvider 是否精确指定了供应商（Type + Name 都非空）
func (r *SelectRequest) IsExactProvider() bool {
	return r.ProviderKey != nil && r.ProviderKey.Type != "" && r.ProviderKey.Name != ""
}

// IsProviderTypeOnly 是否仅指定了供应商类型（Type 非空，Name 为空）
func (r *SelectRequest) IsProviderTypeOnly() bool {
	return r.ProviderKey != nil && r.ProviderKey.Type != "" && r.ProviderKey.Name == ""
}

// IsAutoSelect 是否全自动选择（ProviderKey 为 nil）
func (r *SelectRequest) IsAutoSelect() bool {
	return r.ProviderKey == nil
}
