package pool

import (
	"context"
	"testing"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/balancer"
	"github.com/nomand-zc/lumin-acpool/cooldown"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-client/credentials"
	"github.com/nomand-zc/lumin-client/providers"
	"github.com/nomand-zc/lumin-client/queue"
	"github.com/nomand-zc/lumin-client/usagerule"
)

// --- Mock Provider ---

type mockProviderClient struct {
	typeName string
	name     string
}

func (m *mockProviderClient) Type() string { return m.typeName }
func (m *mockProviderClient) Name() string { return m.name }
func (m *mockProviderClient) GenerateContent(_ context.Context, _ credentials.Credential, _ providers.Request) (*providers.Response, error) {
	return &providers.Response{}, nil
}
func (m *mockProviderClient) GenerateContentStream(_ context.Context, _ credentials.Credential, _ providers.Request) (queue.Consumer[*providers.Response], error) {
	return nil, nil
}
func (m *mockProviderClient) Refresh(_ context.Context, _ credentials.Credential) error { return nil }
func (m *mockProviderClient) CheckAvailability(_ context.Context, _ credentials.Credential) (credentials.CredentialStatus, error) {
	return 0, nil
}
func (m *mockProviderClient) Models(_ context.Context) ([]string, error) { return nil, nil }
func (m *mockProviderClient) ListModels(_ context.Context, _ credentials.Credential) ([]string, error) {
	return nil, nil
}
func (m *mockProviderClient) GetUsageRules(_ context.Context, _ credentials.Credential) ([]*usagerule.UsageRule, error) {
	return nil, nil
}
func (m *mockProviderClient) GetUsageStats(_ context.Context, _ credentials.Credential) ([]*usagerule.UsageStats, error) {
	return nil, nil
}

var _ providers.Provider = (*mockProviderClient)(nil)

// --- Pool 构造函数测试 ---

func TestNew_DefaultOptions(t *testing.T) {
	p, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	if p == nil {
		t.Fatal("Pool should not be nil")
	}
	if p.Balancer() == nil {
		t.Error("Balancer should not be nil")
	}
	if p.HealthChecker() == nil {
		t.Error("HealthChecker should not be nil")
	}
	if p.Manager() == nil {
		t.Error("Manager should not be nil")
	}
}

func TestNew_WithOptions(t *testing.T) {
	cm := cooldown.NewCooldownManager()
	p, err := New(
		WithCooldownManager(cm),
		WithDefaultMaxRetries(3),
		WithDefaultFailover(true),
	)
	if err != nil {
		t.Fatalf("New() with options failed: %v", err)
	}
	if p.opts.CooldownManager == nil {
		t.Error("CooldownManager should be set")
	}
}

// --- 注册与选号集成测试 ---

func TestPool_RegisterAndPick(t *testing.T) {
	ctx := context.Background()
	p, _ := New()

	// 注册 Provider
	info := &account.ProviderInfo{
		ProviderType:    "kiro",
		ProviderName:    "team-a",
		Status:          account.ProviderStatusActive,
		SupportedModels: []string{"gpt-4"},
	}
	client := &mockProviderClient{typeName: "kiro", name: "team-a"}
	if err := p.RegisterProvider(ctx, info, client); err != nil {
		t.Fatalf("RegisterProvider failed: %v", err)
	}

	// 注册 Account
	acct := &account.Account{
		ID:           "acct-1",
		ProviderType: "kiro",
		ProviderName: "team-a",
		Status:       account.StatusAvailable,
	}
	if err := p.RegisterAccount(ctx, acct); err != nil {
		t.Fatalf("RegisterAccount failed: %v", err)
	}

	// Pick
	result, err := p.Pick(ctx, &balancer.PickRequest{
		Model: "gpt-4",
	})
	if err != nil {
		t.Fatalf("Pick failed: %v", err)
	}
	if result.Account.ID != "acct-1" {
		t.Errorf("Pick should return acct-1, got %s", result.Account.ID)
	}
	if result.Client == nil {
		t.Error("PickResult.Client should not be nil when using Pool")
	}
	if result.Client != client {
		t.Error("PickResult.Client should be the registered client")
	}
}

func TestPool_Pick_WithProviderKey(t *testing.T) {
	ctx := context.Background()
	p, _ := New()

	// 注册两个 Provider
	for _, name := range []string{"team-a", "team-b"} {
		info := &account.ProviderInfo{
			ProviderType:    "kiro",
			ProviderName:    name,
			Status:          account.ProviderStatusActive,
			SupportedModels: []string{"gpt-4"},
		}
		client := &mockProviderClient{typeName: "kiro", name: name}
		_ = p.RegisterProvider(ctx, info, client)

		_ = p.RegisterAccount(ctx, &account.Account{
			ID:           "acct-" + name,
			ProviderType: "kiro",
			ProviderName: name,
			Status:       account.StatusAvailable,
		})
	}

	// 指定 Provider Pick
	key := account.BuildProviderKey("kiro", "team-b")
	result, err := p.Pick(ctx, &balancer.PickRequest{
		Model:       "gpt-4",
		ProviderKey: &key,
	})
	if err != nil {
		t.Fatalf("Pick with ProviderKey failed: %v", err)
	}
	if result.Account.ProviderName != "team-b" {
		t.Errorf("Pick should return team-b account, got %s", result.Account.ProviderName)
	}
}

func TestPool_Pick_NoProvider(t *testing.T) {
	ctx := context.Background()
	p, _ := New()

	_, err := p.Pick(ctx, &balancer.PickRequest{
		Model: "gpt-4",
	})
	if err == nil {
		t.Error("Pick should fail when no providers registered")
	}
}

func TestPool_Pick_NoAccount(t *testing.T) {
	ctx := context.Background()
	p, _ := New()

	info := &account.ProviderInfo{
		ProviderType:    "kiro",
		ProviderName:    "team-a",
		Status:          account.ProviderStatusActive,
		SupportedModels: []string{"gpt-4"},
	}
	client := &mockProviderClient{typeName: "kiro", name: "team-a"}
	_ = p.RegisterProvider(ctx, info, client)

	_, err := p.Pick(ctx, &balancer.PickRequest{
		Model: "gpt-4",
	})
	if err == nil {
		t.Error("Pick should fail when no accounts registered")
	}
}

// --- Report 测试 ---

func TestPool_ReportSuccess(t *testing.T) {
	ctx := context.Background()
	p, _ := New()

	info := &account.ProviderInfo{
		ProviderType:    "kiro",
		ProviderName:    "team-a",
		Status:          account.ProviderStatusActive,
		SupportedModels: []string{"gpt-4"},
	}
	client := &mockProviderClient{typeName: "kiro", name: "team-a"}
	_ = p.RegisterProvider(ctx, info, client)
	_ = p.RegisterAccount(ctx, &account.Account{
		ID:           "acct-1",
		ProviderType: "kiro",
		ProviderName: "team-a",
		Status:       account.StatusAvailable,
	})

	err := p.ReportSuccess(ctx, "acct-1")
	if err != nil {
		t.Errorf("ReportSuccess failed: %v", err)
	}
}

func TestPool_ReportFailure(t *testing.T) {
	ctx := context.Background()
	p, _ := New()

	info := &account.ProviderInfo{
		ProviderType:    "kiro",
		ProviderName:    "team-a",
		Status:          account.ProviderStatusActive,
		SupportedModels: []string{"gpt-4"},
	}
	client := &mockProviderClient{typeName: "kiro", name: "team-a"}
	_ = p.RegisterProvider(ctx, info, client)
	_ = p.RegisterAccount(ctx, &account.Account{
		ID:           "acct-1",
		ProviderType: "kiro",
		ProviderName: "team-a",
		Status:       account.StatusAvailable,
	})

	err := p.ReportFailure(ctx, "acct-1", nil)
	if err != nil {
		t.Errorf("ReportFailure failed: %v", err)
	}
}

// --- 管理操作测试 ---

func TestPool_UnregisterProvider(t *testing.T) {
	ctx := context.Background()
	p, _ := New()

	info := &account.ProviderInfo{
		ProviderType: "kiro",
		ProviderName: "team-a",
		Status:       account.ProviderStatusActive,
	}
	client := &mockProviderClient{typeName: "kiro", name: "team-a"}
	_ = p.RegisterProvider(ctx, info, client)
	_ = p.RegisterAccount(ctx, &account.Account{
		ID:           "acct-1",
		ProviderType: "kiro",
		ProviderName: "team-a",
	})

	key := account.BuildProviderKey("kiro", "team-a")
	err := p.UnregisterProvider(ctx, key)
	if err != nil {
		t.Fatalf("UnregisterProvider failed: %v", err)
	}

	// 验证 Provider 和 Account 都已删除
	_, err = p.GetProvider(ctx, key)
	if err != storage.ErrNotFound {
		t.Error("Provider should be removed")
	}
	_, err = p.GetAccount(ctx, "acct-1")
	if err != storage.ErrNotFound {
		t.Error("Account should be removed")
	}

	// 验证全局注册表已清理
	if providers.GetProvider(key) != nil {
		t.Error("Global registry should be cleaned")
	}
}

func TestPool_UnregisterAccount(t *testing.T) {
	ctx := context.Background()
	p, _ := New()

	_ = p.RegisterAccount(ctx, &account.Account{
		ID:           "acct-1",
		ProviderType: "kiro",
		ProviderName: "team-a",
	})

	err := p.UnregisterAccount(ctx, "acct-1")
	if err != nil {
		t.Fatalf("UnregisterAccount failed: %v", err)
	}

	_, err = p.GetAccount(ctx, "acct-1")
	if err != storage.ErrNotFound {
		t.Error("Account should be removed")
	}
}

func TestPool_ListOperations(t *testing.T) {
	ctx := context.Background()
	p, _ := New()

	// 注册多个 Provider 和 Account
	for _, name := range []string{"team-a", "team-b"} {
		client := &mockProviderClient{typeName: "kiro", name: name}
		_ = p.RegisterProvider(ctx, &account.ProviderInfo{
			ProviderType: "kiro",
			ProviderName: name,
			Status:       account.ProviderStatusActive,
		}, client)

		_ = p.RegisterAccount(ctx, &account.Account{
			ID:           "acct-" + name,
			ProviderType: "kiro",
			ProviderName: name,
		})
	}

	providers, _ := p.ListProviders(ctx)
	if len(providers) != 2 {
		t.Errorf("Expected 2 providers, got %d", len(providers))
	}

	accounts, _ := p.ListAccounts(ctx)
	if len(accounts) != 2 {
		t.Errorf("Expected 2 accounts, got %d", len(accounts))
	}
}

// --- 生命周期管理测试 ---

func TestPool_StartAndClose(t *testing.T) {
	ctx := context.Background()
	p, _ := New()

	// 注册 Provider 和 Account，确保 TargetProvider 有数据
	client := &mockProviderClient{typeName: "kiro", name: "team-a"}
	_ = p.RegisterProvider(ctx, &account.ProviderInfo{
		ProviderType: "kiro",
		ProviderName: "team-a",
		Status:       account.ProviderStatusActive,
	}, client)

	err := p.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// 重复启动应报错
	err = p.Start(ctx)
	if err == nil {
		t.Error("Start again should fail")
	}

	// 关闭
	err = p.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// 重复关闭应该是幂等的
	err = p.Close()
	if err != nil {
		t.Errorf("Close again should be idempotent, got: %v", err)
	}
}

func TestPool_CloseWithoutStart(t *testing.T) {
	p, _ := New()

	err := p.Close()
	if err != nil {
		t.Errorf("Close without Start should be fine, got: %v", err)
	}
}

// --- 端到端集成测试 ---

func TestPool_E2E_FullWorkflow(t *testing.T) {
	ctx := context.Background()

	// 1. 创建 Pool
	p, err := New(
		WithCooldownManager(cooldown.NewCooldownManager()),
		WithDefaultMaxRetries(2),
		WithDefaultFailover(true),
		// 使用较长的检查间隔，避免测试中触发后台任务
		WithRecoveryCheckInterval(1*time.Hour),
		WithCredentialCheckInterval(1*time.Hour),
		WithUsageCheckInterval(1*time.Hour),
	)
	if err != nil {
		t.Fatalf("New Pool failed: %v", err)
	}

	// 2. 注册 Provider
	client := &mockProviderClient{typeName: "kiro", name: "team-a"}
	err = p.RegisterProvider(ctx, &account.ProviderInfo{
		ProviderType:    "kiro",
		ProviderName:    "team-a",
		Status:          account.ProviderStatusActive,
		SupportedModels: []string{"gpt-4", "gpt-3.5"},
		Weight:          1,
		Priority:        10,
	}, client)
	if err != nil {
		t.Fatalf("RegisterProvider failed: %v", err)
	}

	// 3. 注册多个 Account
	for _, id := range []string{"acct-1", "acct-2", "acct-3"} {
		err = p.RegisterAccount(ctx, &account.Account{
			ID:           id,
			ProviderType: "kiro",
			ProviderName: "team-a",
			Status:       account.StatusAvailable,
		})
		if err != nil {
			t.Fatalf("RegisterAccount %s failed: %v", id, err)
		}
	}

	// 4. Pick
	result, err := p.Pick(ctx, &balancer.PickRequest{Model: "gpt-4"})
	if err != nil {
		t.Fatalf("Pick failed: %v", err)
	}
	if result.Account == nil {
		t.Fatal("Pick result account should not be nil")
	}
	if result.Client == nil {
		t.Fatal("Pick result client should not be nil")
	}

	// 5. ReportSuccess
	err = p.ReportSuccess(ctx, result.Account.ID)
	if err != nil {
		t.Fatalf("ReportSuccess failed: %v", err)
	}

	// 6. ReportFailure
	err = p.ReportFailure(ctx, result.Account.ID, nil)
	if err != nil {
		t.Fatalf("ReportFailure failed: %v", err)
	}

	// 7. 验证 list 操作
	providersList, _ := p.ListProviders(ctx)
	if len(providersList) != 1 {
		t.Errorf("Expected 1 provider, got %d", len(providersList))
	}
	accountsList, _ := p.ListAccounts(ctx)
	if len(accountsList) != 3 {
		t.Errorf("Expected 3 accounts, got %d", len(accountsList))
	}

	// 8. 注销一个 Account
	err = p.UnregisterAccount(ctx, "acct-2")
	if err != nil {
		t.Fatalf("UnregisterAccount failed: %v", err)
	}
	accountsList, _ = p.ListAccounts(ctx)
	if len(accountsList) != 2 {
		t.Errorf("Expected 2 accounts after unregister, got %d", len(accountsList))
	}

	// 9. 注销 Provider（级联删除所有 Account）
	key := account.BuildProviderKey("kiro", "team-a")
	err = p.UnregisterProvider(ctx, key)
	if err != nil {
		t.Fatalf("UnregisterProvider failed: %v", err)
	}
	accountsList, _ = p.ListAccounts(ctx)
	if len(accountsList) != 0 {
		t.Errorf("Expected 0 accounts after provider unregister, got %d", len(accountsList))
	}
	providersList, _ = p.ListProviders(ctx)
	if len(providersList) != 0 {
		t.Errorf("Expected 0 providers after unregister, got %d", len(providersList))
	}

	// 10. 关闭
	err = p.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestPool_HealthCheckerDefaultChecks(t *testing.T) {
	p, _ := New()

	// 验证默认注册了 6 个 check 项
	checks := p.HealthChecker().ListChecks()
	if len(checks) != 6 {
		t.Errorf("Expected 6 default checks, got %d", len(checks))
	}

	// 验证 check 名称
	checkNames := make(map[string]bool)
	for _, c := range checks {
		checkNames[c.Check.Name()] = true
	}
	expected := []string{"recovery", "credential_validity", "credential_refresh", "usage_quota", "model_discovery", "usage_rules_refresh"}
	for _, name := range expected {
		if !checkNames[name] {
			t.Errorf("Missing default check: %s", name)
		}
	}
}

func TestPool_MultipleProviders_Pick(t *testing.T) {
	ctx := context.Background()
	p, _ := New(WithDefaultFailover(true))

	// 注册两个不同类型的 Provider
	for _, info := range []*account.ProviderInfo{
		{ProviderType: "kiro", ProviderName: "team-a", Status: account.ProviderStatusActive, SupportedModels: []string{"gpt-4"}, Priority: 10},
		{ProviderType: "openai", ProviderName: "team-b", Status: account.ProviderStatusActive, SupportedModels: []string{"gpt-4"}, Priority: 5},
	} {
		client := &mockProviderClient{typeName: info.ProviderType, name: info.ProviderName}
		_ = p.RegisterProvider(ctx, info, client)
		_ = p.RegisterAccount(ctx, &account.Account{
			ID:           "acct-" + info.ProviderName,
			ProviderType: info.ProviderType,
			ProviderName: info.ProviderName,
			Status:       account.StatusAvailable,
		})
	}

	// 自动选号（应选到高优先级的 kiro/team-a）
	result, err := p.Pick(ctx, &balancer.PickRequest{Model: "gpt-4"})
	if err != nil {
		t.Fatalf("Pick failed: %v", err)
	}
	if result.ProviderKey.Type != "kiro" {
		t.Errorf("Should pick higher priority provider (kiro), got %s", result.ProviderKey.Type)
	}

	// 按类型筛选
	key := account.ProviderKey{Type: "openai"}
	result, err = p.Pick(ctx, &balancer.PickRequest{
		Model:       "gpt-4",
		ProviderKey: &key,
	})
	if err != nil {
		t.Fatalf("Pick by type failed: %v", err)
	}
	if result.ProviderKey.Type != "openai" {
		t.Errorf("Should pick openai provider, got %s", result.ProviderKey.Type)
	}
}
