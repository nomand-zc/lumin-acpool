package resolver

import (
	"context"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/provider"
)

// Resolver 解析器接口
// 负责从存储中解析出可用的供应商和账号列表
// 对标微服务架构中的 Service Discovery / Resolver 层
type Resolver interface {
	// ResolveProviders 解析出支持指定模型的活跃供应商
	//
	// 参数:
	//   model        - 请求的模型名称
	//   providerType - 供应商类型过滤（空字符串表示不限制）
	//
	// 返回:
	//   匹配条件的供应商列表；无匹配时返回空切片
	ResolveProviders(ctx context.Context, model string, providerType string) ([]*provider.ProviderInfo, error)

	// ResolveAccounts 解析出指定供应商下的可用账号
	//
	// 参数:
	//   key  - 供应商标识
	//   tags - 标签过滤条件（nil 表示不限制）
	//   excludeIDs - 需要排除的账号 ID 列表
	//
	// 返回:
	//   匹配条件的账号列表；无匹配时返回空切片
	ResolveAccounts(ctx context.Context, key provider.ProviderKey, tags map[string]string, excludeIDs []string) ([]*account.Account, error)
}
