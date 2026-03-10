package manager

import (
	"context"
	"fmt"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-acpool/usagetracker"
	"github.com/nomand-zc/lumin-client/providers"
)

// AccountManager 账号管理器，封装 Provider 和 Account 的注册/注销生命周期。
// 将原本需要调用方手动编排的多步骤操作，合并为一步完成的原子操作。
//
// 用法示例：
//
//	mgr := manager.NewAccountManager(
//	    manager.WithAccountStorage(accountStore),
//	    manager.WithProviderStorage(providerStore),
//	)
//	mgr.RegisterProvider(ctx, providerInfo, client)
//	mgr.RegisterAccount(ctx, acct)
type AccountManager struct {
	opts ManagerOptions
}

// NewAccountManager 创建一个 AccountManager 实例。
func NewAccountManager(opts ...ManagerOption) (*AccountManager, error) {
	o := ManagerOptions{}
	for _, opt := range opts {
		opt(&o)
	}

	if o.AccountStorage == nil {
		return nil, fmt.Errorf("manager: AccountStorage is required")
	}
	if o.ProviderStorage == nil {
		return nil, fmt.Errorf("manager: ProviderStorage is required")
	}

	return &AccountManager{opts: o}, nil
}

// RegisterProvider 注册一个 Provider（元数据 + SDK Client）。
// 一步完成以下操作：
//  1. ProviderStorage.Add() 注册 Provider 元数据
//  2. providers.Register() 注册 Provider SDK Client 到全局注册表
//
// 如果 Provider 已存在，则更新 Provider 元数据和运行时实例。
func (m *AccountManager) RegisterProvider(ctx context.Context, info *account.ProviderInfo, client providers.Provider) error {
	if info == nil {
		return fmt.Errorf("manager: ProviderInfo is required")
	}
	if client == nil {
		return fmt.Errorf("manager: Provider client is required")
	}

	// 设置时间戳
	now := time.Now()
	if info.CreatedAt.IsZero() {
		info.CreatedAt = now
	}
	info.UpdatedAt = now

	// 1. 注册/更新 Provider 元数据到 ProviderStorage
	err := m.opts.ProviderStorage.Add(ctx, info)
	if err != nil {
		// 如果已存在，则更新
		if err == storage.ErrAlreadyExists {
			if updateErr := m.opts.ProviderStorage.Update(ctx, info); updateErr != nil {
				return fmt.Errorf("manager: update provider: %w", updateErr)
			}
		} else {
			return fmt.Errorf("manager: add provider: %w", err)
		}
	}

	// 2. 注册 Provider SDK Client 到全局注册表
	providers.Register(client)

	return nil
}

// UnregisterProvider 注销一个 Provider 及其下属的所有 Account。
// 一步完成以下操作：
//  1. 查询并注销该 Provider 下的所有 Account（包括 UsageTracker、StatsStore 清理）
//  2. ProviderStorage.Remove() 删除 Provider 元数据
//  3. providers.Unregister() 注销 SDK Client
func (m *AccountManager) UnregisterProvider(ctx context.Context, key account.ProviderKey) error {
	// 1. 查询该 Provider 下的所有 Account
	accounts, err := m.opts.AccountStorage.Search(ctx, nil)
	if err != nil {
		return fmt.Errorf("manager: search accounts: %w", err)
	}

	// 逐个注销 Account（包括清理 UsageTracker、StatsStore）
	for _, acct := range accounts {
		if acct.ProviderKey() == key {
			if err := m.unregisterAccountInternal(ctx, acct.ID); err != nil {
				return fmt.Errorf("manager: unregister account %s: %w", acct.ID, err)
			}
		}
	}

	// 2. 删除 Provider 元数据
	if err := m.opts.ProviderStorage.Remove(ctx, key); err != nil {
		// 忽略 NotFound 错误（可能已被删除）
		if err != storage.ErrNotFound {
			return fmt.Errorf("manager: remove provider: %w", err)
		}
	}

	// 3. 注销 Provider SDK Client
	providers.Unregister(key)

	return nil
}

// RegisterAccount 注册一个 Account。
// 一步完成以下操作：
//  1. AccountStorage.Add() 注册 Account
//  2. UsageTracker.InitRules() 初始化用量追踪规则（如果配置了 UsageTracker 且 Account 有 UsageRules）
//
// 注意：调用方应先调用 RegisterProvider 注册 Provider，再注册 Account。
func (m *AccountManager) RegisterAccount(ctx context.Context, acct *account.Account) error {
	if acct == nil {
		return fmt.Errorf("manager: Account is required")
	}

	// 设置时间戳
	now := time.Now()
	if acct.CreatedAt.IsZero() {
		acct.CreatedAt = now
	}
	acct.UpdatedAt = now

	// 设置默认状态
	if acct.Status == 0 {
		acct.Status = account.StatusAvailable
	}

	// 1. 注册 Account 到 AccountStorage
	if err := m.opts.AccountStorage.Add(ctx, acct); err != nil {
		return fmt.Errorf("manager: add account: %w", err)
	}

	// 2. 初始化 UsageTracker 规则
	if m.opts.UsageTracker != nil && len(acct.UsageRules) > 0 {
		if err := m.opts.UsageTracker.InitRules(ctx, acct.ID, acct.UsageRules); err != nil {
			// 初始化失败时回滚 Account 注册
			_ = m.opts.AccountStorage.Remove(ctx, acct.ID)
			return fmt.Errorf("manager: init usage rules: %w", err)
		}
	}

	return nil
}

// UnregisterAccount 注销一个 Account。
// 一步完成以下操作：
//  1. AccountStorage.Remove() 删除 Account
//  2. UsageTracker.Remove() 清理用量追踪数据（如果配置了 UsageTracker）
//  3. StatsStore.Remove() 清理运行时统计数据（如果配置了 StatsStore）
func (m *AccountManager) UnregisterAccount(ctx context.Context, accountID string) error {
	if accountID == "" {
		return fmt.Errorf("manager: account ID is required")
	}
	return m.unregisterAccountInternal(ctx, accountID)
}

// unregisterAccountInternal 内部注销账号的实现。
func (m *AccountManager) unregisterAccountInternal(ctx context.Context, accountID string) error {
	// 1. 删除 Account
	if err := m.opts.AccountStorage.Remove(ctx, accountID); err != nil {
		if err != storage.ErrNotFound {
			return fmt.Errorf("manager: remove account: %w", err)
		}
	}

	// 2. 清理 UsageTracker 数据
	if m.opts.UsageTracker != nil {
		_ = m.opts.UsageTracker.Remove(ctx, accountID)
	}

	// 3. 清理 StatsStore 数据
	if m.opts.StatsStore != nil {
		_ = m.opts.StatsStore.Remove(ctx, accountID)
	}

	return nil
}

// GetProvider 获取指定 Provider 的信息。
func (m *AccountManager) GetProvider(ctx context.Context, key account.ProviderKey) (*account.ProviderInfo, error) {
	return m.opts.ProviderStorage.Get(ctx, key)
}

// GetAccount 获取指定 Account 的信息。
func (m *AccountManager) GetAccount(ctx context.Context, accountID string) (*account.Account, error) {
	return m.opts.AccountStorage.Get(ctx, accountID)
}

// ListProviders 列出所有 Provider。
func (m *AccountManager) ListProviders(ctx context.Context) ([]*account.ProviderInfo, error) {
	return m.opts.ProviderStorage.Search(ctx, nil)
}

// ListAccounts 列出所有 Account。
func (m *AccountManager) ListAccounts(ctx context.Context) ([]*account.Account, error) {
	return m.opts.AccountStorage.Search(ctx, nil)
}

// ListAccountsByProvider 列出指定 Provider 下的所有 Account。
func (m *AccountManager) ListAccountsByProvider(ctx context.Context, key account.ProviderKey) ([]*account.Account, error) {
	allAccounts, err := m.opts.AccountStorage.Search(ctx, nil)
	if err != nil {
		return nil, err
	}
	var result []*account.Account
	for _, acct := range allAccounts {
		if acct.ProviderKey() == key {
			result = append(result, acct)
		}
	}
	return result, nil
}

// ManagerOption 是 AccountManager 的配置选项。
type ManagerOption func(*ManagerOptions)

// ManagerOptions 配置选项结构体。
type ManagerOptions struct {
	// AccountStorage 账号存储（必选）。
	AccountStorage storage.AccountStorage
	// ProviderStorage Provider 存储（必选）。
	ProviderStorage storage.ProviderStorage
	// UsageTracker 用量追踪器（可选）。
	UsageTracker usagetracker.UsageTracker
	// StatsStore 运行时统计存储（可选）。
	StatsStore storage.StatsStore
}

// WithAccountStorage 设置账号存储。
func WithAccountStorage(s storage.AccountStorage) ManagerOption {
	return func(o *ManagerOptions) { o.AccountStorage = s }
}

// WithProviderStorage 设置 Provider 存储。
func WithProviderStorage(s storage.ProviderStorage) ManagerOption {
	return func(o *ManagerOptions) { o.ProviderStorage = s }
}

// WithUsageTracker 设置用量追踪器。
func WithUsageTracker(ut usagetracker.UsageTracker) ManagerOption {
	return func(o *ManagerOptions) { o.UsageTracker = ut }
}

// WithStatsStore 设置运行时统计存储。
func WithStatsStore(ss storage.StatsStore) ManagerOption {
	return func(o *ManagerOptions) { o.StatsStore = ss }
}
