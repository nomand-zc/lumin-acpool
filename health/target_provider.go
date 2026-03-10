package health

import (
	"context"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-client/providers"
)

// NewStorageTargetProvider 创建一个基于 Storage 的 TargetProvider。
// 该函数从 AccountStorage 中查询所有账号，并通过 clientLookup 获取对应的 Provider SDK Client，
// 组装为 CheckTarget 列表供 HealthChecker 后台扫描使用。
//
// clientLookup 根据 ProviderKey 查找对应的 Provider SDK Client。
// 若某个账号对应的 Client 未注册，则该账号会被跳过。
//
// 用法示例：
//
//	providers.Register(client)
//	tp := health.NewStorageTargetProvider(accountStorage, providers.GetProvider)
//	checker := health.NewHealthChecker(health.WithTargetProvider(tp))
func NewStorageTargetProvider(
	accountStorage storage.AccountStorage,
	clientLookup func(key account.ProviderKey) providers.Provider,
) TargetProvider {
	return func(ctx context.Context) []CheckTarget {
		accounts, err := accountStorage.Search(ctx, nil)
		if err != nil {
			return nil
		}

		targets := make([]CheckTarget, 0, len(accounts))
		for _, acct := range accounts {
			client := clientLookup(acct.ProviderKey())
			if client == nil {
				continue // 跳过未注册 Client 的账号
			}
			targets = append(targets, NewCheckTarget(acct, client))
		}
		return targets
	}
}
