package manager

import (
	"context"
	"testing"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-acpool/storage/memory/accountstore"
	"github.com/nomand-zc/lumin-acpool/storage/memory/providerstore"
	"github.com/nomand-zc/lumin-acpool/storage/memory/statsstore"
	"github.com/nomand-zc/lumin-acpool/usagetracker"
	"github.com/nomand-zc/lumin-client/credentials"
	"github.com/nomand-zc/lumin-client/providers"
	"github.com/nomand-zc/lumin-client/queue"
	"github.com/nomand-zc/lumin-client/usagerule"
)

// mockProviderClient 实现 providers.Provider 接口（仅供测试使用）。
type mockProviderClient struct {
	typeName string
	name     string
}

func (m *mockProviderClient) Type() string { return m.typeName }
func (m *mockProviderClient) Name() string { return m.name }
func (m *mockProviderClient) GenerateContent(_ context.Context, _ credentials.Credential, _ providers.Request) (*providers.Response, error) {
	return nil, nil
}
func (m *mockProviderClient) GenerateContentStream(_ context.Context, _ credentials.Credential, _ providers.Request) (queue.Consumer[*providers.Response], error) {
	return nil, nil
}
func (m *mockProviderClient) Refresh(_ context.Context, _ credentials.Credential) error {
	return nil
}
func (m *mockProviderClient) CheckAvailability(_ context.Context, _ credentials.Credential) (credentials.CredentialStatus, error) {
	return 0, nil
}
func (m *mockProviderClient) Models(_ context.Context) ([]string, error) {
	return nil, nil
}
func (m *mockProviderClient) ListModels(_ context.Context, _ credentials.Credential) ([]string, error) {
	return nil, nil
}
func (m *mockProviderClient) GetUsageRules(_ context.Context, _ credentials.Credential) ([]*usagerule.UsageRule, error) {
	return nil, nil
}
func (m *mockProviderClient) GetUsageStats(_ context.Context, _ credentials.Credential) ([]*usagerule.UsageStats, error) {
	return nil, nil
}

// 编译期检查
var _ providers.Provider = (*mockProviderClient)(nil)

// --- 构造函数测试 ---

func TestNewAccountManager_RequiredDeps(t *testing.T) {
	accountStore := accountstore.NewStore()
	providerStore := providerstore.NewStore()

	tests := []struct {
		name    string
		opts    []ManagerOption
		wantErr bool
	}{
		{
			name:    "缺少 AccountStorage 应报错",
			opts:    []ManagerOption{WithProviderStorage(providerStore)},
			wantErr: true,
		},
		{
			name:    "缺少 ProviderStorage 应报错",
			opts:    []ManagerOption{WithAccountStorage(accountStore)},
			wantErr: true,
		},
		{
			name: "必选依赖齐全应成功",
			opts: []ManagerOption{
				WithAccountStorage(accountStore),
				WithProviderStorage(providerStore),
			},
			wantErr: false,
		},
		{
			name:    "无任何选项应报错",
			opts:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewAccountManager(tt.opts...)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewAccountManager() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// --- RegisterProvider 测试 ---

func TestRegisterProvider_Basic(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)

	info := &account.ProviderInfo{
		ProviderType:    "kiro",
		ProviderName:    "team-a",
		Status:          account.ProviderStatusActive,
		SupportedModels: []string{"gpt-4"},
	}
	client := &mockProviderClient{typeName: "kiro", name: "team-a"}

	err := mgr.RegisterProvider(ctx, info, client)
	if err != nil {
		t.Fatalf("RegisterProvider failed: %v", err)
	}

	// 验证 ProviderStorage 中已注册
	key := account.BuildProviderKey("kiro", "team-a")
	got, err := mgr.GetProvider(ctx, key)
	if err != nil {
		t.Fatalf("GetProvider failed: %v", err)
	}
	if got.ProviderType != "kiro" || got.ProviderName != "team-a" {
		t.Errorf("Provider info mismatch: got %+v", got)
	}
}

func TestRegisterProvider_NilInput(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	client := &mockProviderClient{typeName: "kiro", name: "team-a"}

	// nil info
	err := mgr.RegisterProvider(ctx, nil, client)
	if err == nil {
		t.Error("expected error for nil ProviderInfo")
	}

	// nil client
	info := &account.ProviderInfo{ProviderType: "kiro", ProviderName: "team-a"}
	err = mgr.RegisterProvider(ctx, info, nil)
	if err == nil {
		t.Error("expected error for nil client")
	}
}

func TestRegisterProvider_UpdateExisting(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)

	info1 := &account.ProviderInfo{
		ProviderType:    "kiro",
		ProviderName:    "team-a",
		Status:          account.ProviderStatusActive,
		SupportedModels: []string{"gpt-4"},
		Weight:          1,
	}
	client := &mockProviderClient{typeName: "kiro", name: "team-a"}

	_ = mgr.RegisterProvider(ctx, info1, client)

	// 再次注册同一个 Provider，应自动更新
	info2 := &account.ProviderInfo{
		ProviderType:    "kiro",
		ProviderName:    "team-a",
		Status:          account.ProviderStatusActive,
		SupportedModels: []string{"gpt-4", "gpt-3.5"},
		Weight:          5,
	}
	err := mgr.RegisterProvider(ctx, info2, client)
	if err != nil {
		t.Fatalf("RegisterProvider (update) failed: %v", err)
	}

	key := account.BuildProviderKey("kiro", "team-a")
	got, _ := mgr.GetProvider(ctx, key)
	if got.Weight != 5 {
		t.Errorf("Weight not updated: got %d, want 5", got.Weight)
	}
	if len(got.SupportedModels) != 2 {
		t.Errorf("SupportedModels not updated: got %v", got.SupportedModels)
	}
}

func TestRegisterProvider_InstanceRegistry(t *testing.T) {
	ctx := context.Background()
	providers.Reset()
	defer providers.Reset()
	mgr := newTestManager(t)

	info := &account.ProviderInfo{
		ProviderType: "kiro",
		ProviderName: "team-a",
		Status:       account.ProviderStatusActive,
	}
	client := &mockProviderClient{typeName: "kiro", name: "team-a"}

	_ = mgr.RegisterProvider(ctx, info, client)

	// 验证全局注册表中已注册
	key := account.BuildProviderKey("kiro", "team-a")
	gotClient := providers.GetProvider(key)
	if gotClient == nil {
		t.Fatal("Global registry should have the provider client")
	}
	if gotClient != client {
		t.Error("Global registry client mismatch")
	}
}

// --- RegisterAccount 测试 ---

func TestRegisterAccount_Basic(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)

	acct := &account.Account{
		ID:           "acct-1",
		ProviderType: "kiro",
		ProviderName: "team-a",
	}

	err := mgr.RegisterAccount(ctx, acct)
	if err != nil {
		t.Fatalf("RegisterAccount failed: %v", err)
	}

	// 验证已注册
	got, err := mgr.GetAccount(ctx, "acct-1")
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if got.ID != "acct-1" {
		t.Errorf("Account ID mismatch: got %s", got.ID)
	}
	// 验证默认状态
	if got.Status != account.StatusAvailable {
		t.Errorf("Account status should be Available, got %v", got.Status)
	}
	// 验证时间戳已设置
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
}

func TestRegisterAccount_NilInput(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)

	err := mgr.RegisterAccount(ctx, nil)
	if err == nil {
		t.Error("expected error for nil Account")
	}
}

func TestRegisterAccount_WithUsageRules(t *testing.T) {
	ctx := context.Background()
	ut := usagetracker.NewUsageTracker()
	mgr := newTestManagerWithUsageTracker(t, ut)

	acct := &account.Account{
		ID:           "acct-1",
		ProviderType: "kiro",
		ProviderName: "team-a",
		UsageRules: []*usagerule.UsageRule{
			{
				SourceType:      usagerule.SourceTypeRequest,
				TimeGranularity: usagerule.GranularityDay,
				WindowSize:      1,
				Total:           100,
			},
		},
	}

	err := mgr.RegisterAccount(ctx, acct)
	if err != nil {
		t.Fatalf("RegisterAccount with UsageRules failed: %v", err)
	}

	// 验证 UsageTracker 已初始化
	usages, err := ut.GetTrackedUsages(ctx, "acct-1")
	if err != nil {
		t.Fatalf("GetTrackedUsages failed: %v", err)
	}
	if len(usages) == 0 {
		t.Error("UsageTracker should have tracked usages after registration")
	}
}

func TestRegisterAccount_PreservesExistingStatus(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)

	acct := &account.Account{
		ID:           "acct-1",
		ProviderType: "kiro",
		ProviderName: "team-a",
		Status:       account.StatusDisabled,
	}

	_ = mgr.RegisterAccount(ctx, acct)

	got, _ := mgr.GetAccount(ctx, "acct-1")
	if got.Status != account.StatusDisabled {
		t.Errorf("Should preserve existing status, got %v", got.Status)
	}
}

// --- UnregisterAccount 测试 ---

func TestUnregisterAccount_Basic(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)

	acct := &account.Account{
		ID:           "acct-1",
		ProviderType: "kiro",
		ProviderName: "team-a",
	}
	_ = mgr.RegisterAccount(ctx, acct)

	err := mgr.UnregisterAccount(ctx, "acct-1")
	if err != nil {
		t.Fatalf("UnregisterAccount failed: %v", err)
	}

	// 验证已删除
	_, err = mgr.GetAccount(ctx, "acct-1")
	if err != storage.ErrNotFound {
		t.Errorf("Account should be removed, got err: %v", err)
	}
}

func TestUnregisterAccount_EmptyID(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)

	err := mgr.UnregisterAccount(ctx, "")
	if err == nil {
		t.Error("expected error for empty account ID")
	}
}

func TestUnregisterAccount_CleansUsageTracker(t *testing.T) {
	ctx := context.Background()
	ut := usagetracker.NewUsageTracker()
	mgr := newTestManagerWithUsageTracker(t, ut)

	acct := &account.Account{
		ID:           "acct-1",
		ProviderType: "kiro",
		ProviderName: "team-a",
		UsageRules: []*usagerule.UsageRule{
			{
				SourceType:      usagerule.SourceTypeRequest,
				TimeGranularity: usagerule.GranularityDay,
				WindowSize:      1,
				Total:           100,
			},
		},
	}
	_ = mgr.RegisterAccount(ctx, acct)

	_ = mgr.UnregisterAccount(ctx, "acct-1")

	// 验证 UsageTracker 数据已清理
	usages, _ := ut.GetTrackedUsages(ctx, "acct-1")
	if len(usages) != 0 {
		t.Error("UsageTracker should be cleaned after unregister")
	}
}

func TestUnregisterAccount_CleansStatsStore(t *testing.T) {
	ctx := context.Background()
	ss := statsstore.NewMemoryStatsStore()
	mgr := newTestManagerWithStatsStore(t, ss)

	acct := &account.Account{
		ID:           "acct-1",
		ProviderType: "kiro",
		ProviderName: "team-a",
	}
	_ = mgr.RegisterAccount(ctx, acct)
	// 模拟一些统计数据
	_ = ss.IncrSuccess(ctx, "acct-1")

	_ = mgr.UnregisterAccount(ctx, "acct-1")

	// 验证 StatsStore 数据已清理
	stats, _ := ss.Get(ctx, "acct-1")
	if stats.TotalCalls != 0 {
		t.Error("StatsStore should be cleaned after unregister")
	}
}

func TestUnregisterAccount_NotFound_NoError(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)

	// 注销不存在的账号不应报错
	err := mgr.UnregisterAccount(ctx, "non-existent")
	if err != nil {
		t.Errorf("UnregisterAccount for non-existent should not error, got: %v", err)
	}
}

// --- UnregisterProvider 测试 ---

func TestUnregisterProvider_Basic(t *testing.T) {
	ctx := context.Background()
	providers.Reset()
	defer providers.Reset()
	mgr := newTestManager(t)

	info := &account.ProviderInfo{
		ProviderType: "kiro",
		ProviderName: "team-a",
		Status:       account.ProviderStatusActive,
	}
	client := &mockProviderClient{typeName: "kiro", name: "team-a"}
	_ = mgr.RegisterProvider(ctx, info, client)

	// 注册几个 Account
	for _, id := range []string{"acct-1", "acct-2"} {
		_ = mgr.RegisterAccount(ctx, &account.Account{
			ID:           id,
			ProviderType: "kiro",
			ProviderName: "team-a",
		})
	}
	// 注册一个不属于该 Provider 的 Account
	_ = mgr.RegisterAccount(ctx, &account.Account{
		ID:           "acct-other",
		ProviderType: "openai",
		ProviderName: "team-b",
	})

	key := account.BuildProviderKey("kiro", "team-a")
	err := mgr.UnregisterProvider(ctx, key)
	if err != nil {
		t.Fatalf("UnregisterProvider failed: %v", err)
	}

	// 验证 Provider 已删除
	_, err = mgr.GetProvider(ctx, key)
	if err != storage.ErrNotFound {
		t.Errorf("Provider should be removed, got err: %v", err)
	}

	// 验证该 Provider 下的 Account 已全部删除
	_, err = mgr.GetAccount(ctx, "acct-1")
	if err != storage.ErrNotFound {
		t.Error("acct-1 should be removed")
	}
	_, err = mgr.GetAccount(ctx, "acct-2")
	if err != storage.ErrNotFound {
		t.Error("acct-2 should be removed")
	}

	// 验证其他 Provider 的 Account 不受影响
	other, err := mgr.GetAccount(ctx, "acct-other")
	if err != nil || other == nil {
		t.Error("acct-other should not be affected")
	}

	// 验证全局注册表已注销
	if providers.GetProvider(key) != nil {
		t.Error("Global registry should not have the provider after unregister")
	}
}

func TestUnregisterProvider_NotFound_NoError(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)

	key := account.BuildProviderKey("non", "existent")
	err := mgr.UnregisterProvider(ctx, key)
	if err != nil {
		t.Errorf("UnregisterProvider for non-existent should not error, got: %v", err)
	}
}

// --- List 测试 ---

func TestListProviders(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)

	// 空列表
	plist, _ := mgr.ListProviders(ctx)
	if len(plist) != 0 {
		t.Errorf("Expected 0 providers, got %d", len(plist))
	}

	// 注册后应有记录
	client := &mockProviderClient{typeName: "kiro", name: "team-a"}
	_ = mgr.RegisterProvider(ctx, &account.ProviderInfo{
		ProviderType: "kiro",
		ProviderName: "team-a",
		Status:       account.ProviderStatusActive,
	}, client)

	plist, _ = mgr.ListProviders(ctx)
	if len(plist) != 1 {
		t.Errorf("Expected 1 provider, got %d", len(plist))
	}
}

func TestListAccounts(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)

	accounts, _ := mgr.ListAccounts(ctx)
	if len(accounts) != 0 {
		t.Errorf("Expected 0 accounts, got %d", len(accounts))
	}

	_ = mgr.RegisterAccount(ctx, &account.Account{
		ID:           "acct-1",
		ProviderType: "kiro",
		ProviderName: "team-a",
	})
	_ = mgr.RegisterAccount(ctx, &account.Account{
		ID:           "acct-2",
		ProviderType: "kiro",
		ProviderName: "team-a",
	})

	accounts, _ = mgr.ListAccounts(ctx)
	if len(accounts) != 2 {
		t.Errorf("Expected 2 accounts, got %d", len(accounts))
	}
}

func TestListAccountsByProvider(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)

	_ = mgr.RegisterAccount(ctx, &account.Account{
		ID:           "acct-1",
		ProviderType: "kiro",
		ProviderName: "team-a",
	})
	_ = mgr.RegisterAccount(ctx, &account.Account{
		ID:           "acct-2",
		ProviderType: "kiro",
		ProviderName: "team-a",
	})
	_ = mgr.RegisterAccount(ctx, &account.Account{
		ID:           "acct-3",
		ProviderType: "openai",
		ProviderName: "team-b",
	})

	key := account.BuildProviderKey("kiro", "team-a")
	accounts, err := mgr.ListAccountsByProvider(ctx, key)
	if err != nil {
		t.Fatalf("ListAccountsByProvider failed: %v", err)
	}
	if len(accounts) != 2 {
		t.Errorf("Expected 2 accounts for kiro/team-a, got %d", len(accounts))
	}

	key2 := account.BuildProviderKey("openai", "team-b")
	accounts2, _ := mgr.ListAccountsByProvider(ctx, key2)
	if len(accounts2) != 1 {
		t.Errorf("Expected 1 account for openai/team-b, got %d", len(accounts2))
	}
}

// --- Helper functions ---

func newTestManager(t *testing.T) *AccountManager {
	t.Helper()
	mgr, err := NewAccountManager(
		WithAccountStorage(accountstore.NewStore()),
		WithProviderStorage(providerstore.NewStore()),
	)
	if err != nil {
		t.Fatalf("newTestManager failed: %v", err)
	}
	return mgr
}

func newTestManagerWithUsageTracker(t *testing.T, ut usagetracker.UsageTracker) *AccountManager {
	t.Helper()
	mgr, err := NewAccountManager(
		WithAccountStorage(accountstore.NewStore()),
		WithProviderStorage(providerstore.NewStore()),
		WithUsageTracker(ut),
	)
	if err != nil {
		t.Fatalf("newTestManagerWithUsageTracker failed: %v", err)
	}
	return mgr
}

func newTestManagerWithStatsStore(t *testing.T, ss *statsstore.MemoryStatsStore) *AccountManager {
	t.Helper()
	mgr, err := NewAccountManager(
		WithAccountStorage(accountstore.NewStore()),
		WithProviderStorage(providerstore.NewStore()),
		WithStatsStore(ss),
	)
	if err != nil {
		t.Fatalf("newTestManagerWithStatsStore failed: %v", err)
	}
	return mgr
}
