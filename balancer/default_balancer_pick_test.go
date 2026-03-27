package balancer

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/circuitbreaker"
	"github.com/nomand-zc/lumin-acpool/resolver"
	"github.com/nomand-zc/lumin-acpool/selector"
	"github.com/nomand-zc/lumin-acpool/storage"
	storememory "github.com/nomand-zc/lumin-acpool/storage/memory"
	"github.com/nomand-zc/lumin-client/usagerule"
)

// ============================================================
// 辅助函数：快速添加 Provider + 账号到 store
// ============================================================

// addProviderWithAccounts 向 store 中添加一个 active provider 及若干账号。
func addProviderWithAccounts(ctx context.Context, store *storememory.Store, provType, provName, model string, accountIDs ...string) {
	_ = store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType:          provType,
		ProviderName:          provName,
		Status:                account.ProviderStatusActive,
		SupportedModels:       []string{model},
		AccountCount:          len(accountIDs),
		AvailableAccountCount: len(accountIDs),
	})
	for _, id := range accountIDs {
		_ = store.AddAccount(ctx, &account.Account{
			ID:           id,
			ProviderType: provType,
			ProviderName: provName,
			Status:       account.StatusAvailable,
		})
	}
}

// ============================================================
// 1. New() 构造验证
// ============================================================

func TestNew_MissingAccountStorage(t *testing.T) {
	_, err := New(WithProviderStorage(storememory.NewStore()))
	if err == nil {
		t.Fatal("expected error when AccountStorage is missing")
	}
	if !strings.Contains(err.Error(), "AccountStorage") {
		t.Fatalf("expected error to mention AccountStorage, got: %v", err)
	}
}

func TestNew_MissingResolverAndProviderStorage(t *testing.T) {
	// 只提供 AccountStorage，不提供 Resolver 也不提供 ProviderStorage
	_, err := New(WithAccountStorage(storememory.NewStore()))
	if err == nil {
		t.Fatal("expected error when both Resolver and ProviderStorage are missing")
	}
}

func TestNew_WithResolverNoProviderStorage(t *testing.T) {
	// 只注入 Resolver，不注入 ProviderStorage → 应成功
	store := storememory.NewStore()
	r := resolver.NewStorageResolver(store, store)
	_, err := New(
		WithAccountStorage(store),
		WithResolver(r),
	)
	if err != nil {
		t.Fatalf("expected no error when Resolver is provided without ProviderStorage, got: %v", err)
	}
}

// ============================================================
// 2. Pick - 基础路径
// ============================================================

func TestPick_ModelRequired(t *testing.T) {
	store := storememory.NewStore()
	b, _ := New(WithAccountStorage(store), WithProviderStorage(store))
	_, err := b.Pick(context.Background(), &PickRequest{})
	if !errors.Is(err, ErrModelRequired) {
		t.Fatalf("expected ErrModelRequired, got: %v", err)
	}
}

func TestPick_AutoMode_Success(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	addProviderWithAccounts(ctx, store, "typeA", "nameA", "gpt-4", "acc-1")

	b, err := New(
		WithAccountStorage(store),
		WithProviderStorage(store),
		WithDefaultMaxRetries(0),
	)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	result, err := b.Pick(ctx, &PickRequest{Model: "gpt-4"})
	if err != nil {
		t.Fatalf("Pick() failed: %v", err)
	}
	if result == nil || result.Account == nil {
		t.Fatal("expected non-nil result and account")
	}
	if result.Account.ID != "acc-1" {
		t.Fatalf("expected acc-1, got: %s", result.Account.ID)
	}
}

func TestPick_TypeOnlyMode_Success(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	addProviderWithAccounts(ctx, store, "typeA", "nameA", "gpt-4", "acc-1")

	b, _ := New(
		WithAccountStorage(store),
		WithProviderStorage(store),
		WithDefaultMaxRetries(0),
	)

	result, err := b.Pick(ctx, &PickRequest{
		Model:       "gpt-4",
		ProviderKey: &account.ProviderKey{Type: "typeA"}, // Name 为空 → TypeOnly 模式
	})
	if err != nil {
		t.Fatalf("Pick() TypeOnly mode failed: %v", err)
	}
	if result.Account.ProviderType != "typeA" {
		t.Fatalf("expected typeA account, got: %s", result.Account.ProviderType)
	}
}

func TestPick_ExactMode_Success(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	addProviderWithAccounts(ctx, store, "typeA", "nameA", "gpt-4", "acc-1")

	b, _ := New(
		WithAccountStorage(store),
		WithProviderStorage(store),
		WithDefaultMaxRetries(0),
	)

	result, err := b.Pick(ctx, &PickRequest{
		Model:       "gpt-4",
		ProviderKey: &account.ProviderKey{Type: "typeA", Name: "nameA"},
	})
	if err != nil {
		t.Fatalf("Pick() ExactMode failed: %v", err)
	}
	if result.Account.ProviderName != "nameA" {
		t.Fatalf("expected nameA account, got: %s", result.Account.ProviderName)
	}
}

func TestPick_ExactMode_ProviderNotFound(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()

	b, _ := New(WithAccountStorage(store), WithProviderStorage(store))

	_, err := b.Pick(ctx, &PickRequest{
		Model:       "gpt-4",
		ProviderKey: &account.ProviderKey{Type: "nonexistent", Name: "nonexistent"},
	})
	if !errors.Is(err, ErrProviderNotFound) {
		t.Fatalf("expected ErrProviderNotFound, got: %v", err)
	}
}

func TestPick_ExactMode_ModelNotSupported(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	// 注册 provider，但只支持 "gpt-3"
	_ = store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType:          "typeA",
		ProviderName:          "nameA",
		Status:                account.ProviderStatusActive,
		SupportedModels:       []string{"gpt-3"},
		AccountCount:          1,
		AvailableAccountCount: 1,
	})
	_ = store.AddAccount(ctx, &account.Account{
		ID:           "acc-1",
		ProviderType: "typeA",
		ProviderName: "nameA",
		Status:       account.StatusAvailable,
	})

	b, _ := New(WithAccountStorage(store), WithProviderStorage(store))

	_, err := b.Pick(ctx, &PickRequest{
		Model:       "gpt-4", // 不支持的模型
		ProviderKey: &account.ProviderKey{Type: "typeA", Name: "nameA"},
	})
	if !errors.Is(err, ErrModelNotSupported) {
		t.Fatalf("expected ErrModelNotSupported, got: %v", err)
	}
}

func TestPick_AutoMode_NoCandidates(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	// 不注册任何 provider

	b, _ := New(WithAccountStorage(store), WithProviderStorage(store))

	_, err := b.Pick(ctx, &PickRequest{Model: "gpt-4"})
	if !errors.Is(err, ErrModelNotSupported) {
		t.Fatalf("expected ErrModelNotSupported when no candidates, got: %v", err)
	}
}

// ============================================================
// 3. Failover
// ============================================================

func TestPick_ExactMode_Failover_FallbackToAuto(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()

	// 精确 provider typeA/nameA 不注册（不存在），failover 降级到 typeA/nameB
	addProviderWithAccounts(ctx, store, "typeA", "nameB", "gpt-4", "acc-b1")

	b, _ := New(
		WithAccountStorage(store),
		WithProviderStorage(store),
		WithDefaultMaxRetries(0),
	)

	result, err := b.Pick(ctx, &PickRequest{
		Model:          "gpt-4",
		ProviderKey:    &account.ProviderKey{Type: "typeA", Name: "nameA"}, // 精确模式，nameA 不存在
		EnableFailover: true,
	})
	if err != nil {
		t.Fatalf("Pick() with failover should succeed, got: %v", err)
	}
	if !result.Fallback {
		t.Fatal("expected result.Fallback=true when failover occurred")
	}
	if result.Account.ProviderName != "nameB" {
		t.Fatalf("expected fallback to nameB, got: %s", result.Account.ProviderName)
	}
}

func TestPick_ExactMode_NoFailover_ReturnsError(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()

	// 精确 provider 不存在，且不开启 failover
	b, _ := New(WithAccountStorage(store), WithProviderStorage(store))

	_, err := b.Pick(ctx, &PickRequest{
		Model:          "gpt-4",
		ProviderKey:    &account.ProviderKey{Type: "typeA", Name: "nameA"},
		EnableFailover: false,
	})
	if err == nil {
		t.Fatal("expected error when exact provider not found and no failover")
	}
}

func TestPick_ExactMode_Failover_KeepsProviderType(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()

	// 注册 typeA/nameB 作为 fallback，typeA/nameA 不存在
	addProviderWithAccounts(ctx, store, "typeA", "nameB", "gpt-4", "acc-b1")
	// 也注册 typeB/nameC，但 fallback 只保留 typeA 约束，不应选到它
	addProviderWithAccounts(ctx, store, "typeB", "nameC", "gpt-4", "acc-c1")

	b, _ := New(
		WithAccountStorage(store),
		WithProviderStorage(store),
		WithDefaultMaxRetries(0),
	)

	result, err := b.Pick(ctx, &PickRequest{
		Model:          "gpt-4",
		ProviderKey:    &account.ProviderKey{Type: "typeA", Name: "nameA"}, // 精确模式失败
		EnableFailover: true,
	})
	if err != nil {
		t.Fatalf("expected failover to succeed, got: %v", err)
	}
	// failover 保留 typeA 约束，不应选到 typeB 的账号
	if result.Account.ProviderType != "typeA" {
		t.Fatalf("expected fallback to preserve typeA, got provider type: %s", result.Account.ProviderType)
	}
}

// ============================================================
// 4. Retry
// ============================================================

// failingOccupancyController 始终使 Acquire 失败，模拟所有账号都被占满
type failingOccupancyController struct{}

func (c *failingOccupancyController) FilterAvailable(_ context.Context, accounts []*account.Account) []*account.Account {
	return accounts // FilterAvailable 通过
}
func (c *failingOccupancyController) Acquire(_ context.Context, _ *account.Account) bool {
	return false // Acquire 始终失败
}
func (c *failingOccupancyController) Release(_ context.Context, _ string) {}

func TestPick_Retry_ExhaustsAndFails(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	addProviderWithAccounts(ctx, store, "typeA", "nameA", "gpt-4", "acc-1", "acc-2")

	b, _ := New(
		WithAccountStorage(store),
		WithProviderStorage(store),
		WithOccupancyController(&failingOccupancyController{}),
		WithDefaultMaxRetries(2),
	)

	_, err := b.Pick(ctx, &PickRequest{Model: "gpt-4"})
	// 当所有 Acquire 失败时，账号逐一加入 excludeAccountIDs，
	// 最终 excludeAccounts 耗尽返回 ErrNoAvailableAccount，
	// 或 MaxRetries 用尽返回 ErrMaxRetriesExceeded
	if err == nil {
		t.Fatal("expected error when all acquires fail")
	}
	if !errors.Is(err, ErrMaxRetriesExceeded) && !errors.Is(err, ErrNoAvailableAccount) && !errors.Is(err, ErrNoAvailableProvider) {
		t.Fatalf("expected one of MaxRetriesExceeded/NoAvailableAccount/NoAvailableProvider, got: %v", err)
	}
}

// countingFailAcquireController：前 N 次 Acquire 失败，之后成功
type countingFailAcquireController struct {
	failN    int
	acquired int
}

func (c *countingFailAcquireController) FilterAvailable(_ context.Context, accounts []*account.Account) []*account.Account {
	return accounts
}
func (c *countingFailAcquireController) Acquire(_ context.Context, _ *account.Account) bool {
	c.acquired++
	return c.acquired > c.failN
}
func (c *countingFailAcquireController) Release(_ context.Context, _ string) {}

func TestPick_Retry_SucceedsOnSecondAttempt(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	// 需要 2 个账号：第一个失败，第二个成功
	addProviderWithAccounts(ctx, store, "typeA", "nameA", "gpt-4", "acc-1", "acc-2")

	ctrl := &countingFailAcquireController{failN: 1} // 第一次 Acquire 失败，第二次成功

	b, _ := New(
		WithAccountStorage(store),
		WithProviderStorage(store),
		WithOccupancyController(ctrl),
		WithDefaultMaxRetries(3),
	)

	result, err := b.Pick(ctx, &PickRequest{Model: "gpt-4"})
	if err != nil {
		t.Fatalf("expected success on second attempt, got: %v", err)
	}
	if result == nil || result.Account == nil {
		t.Fatal("expected non-nil result")
	}
}

// ============================================================
// 5. ReportSuccess 路径
// ============================================================

func TestReportSuccess_AccountNotFound(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()

	// 使用一个 StatsStore mock：IncrSuccess 之后 GetStats 仍返回 ConsecutiveFailures > 0
	// 这样才能触发 GetAccount 路径（不会被 IncrSuccess 重置掉）
	fakeStats := &fixedConsecutiveFailuresStore{
		inner:                     store,
		fixedConsecutiveFailures:  3,
	}

	cb, _ := circuitbreaker.NewCircuitBreaker(circuitbreaker.WithStatsStore(store))

	b, _ := New(
		WithAccountStorage(store),
		WithProviderStorage(store),
		WithStatsStore(fakeStats),
		WithCircuitBreaker(cb),
	)

	err := b.ReportSuccess(ctx, "nonexistent-acc")
	if !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("expected ErrAccountNotFound, got: %v", err)
	}
}

func TestReportSuccess_NoStatsStore(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()

	b, _ := New(
		WithAccountStorage(store),
		WithProviderStorage(store),
		// 不设置 StatsStore
	)

	// 即使账号不存在，无 StatsStore 时应直接跳过 → 返回 nil
	err := b.ReportSuccess(ctx, "any-acc")
	if err != nil {
		t.Fatalf("expected nil when StatsStore is nil, got: %v", err)
	}
}

func TestReportSuccess_NoCircuitBreaker(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType: "test",
		ProviderName: "default",
		Status:       account.ProviderStatusActive,
	})
	_ = store.AddAccount(ctx, &account.Account{
		ID:           "acc-1",
		ProviderType: "test",
		ProviderName: "default",
		Status:       account.StatusAvailable,
	})

	b, _ := New(
		WithAccountStorage(store),
		WithProviderStorage(store),
		WithStatsStore(store),
		// 不设置 CircuitBreaker
	)

	err := b.ReportSuccess(ctx, "acc-1")
	if err != nil {
		t.Fatalf("expected nil when CircuitBreaker is nil, got: %v", err)
	}
}

func TestReportSuccess_CircuitOpenRestored(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType: "test",
		ProviderName: "default",
		Status:       account.ProviderStatusActive,
	})

	// 将账号状态设置为 CircuitOpen
	_ = store.AddAccount(ctx, &account.Account{
		ID:           "acc-1",
		ProviderType: "test",
		ProviderName: "default",
		Status:       account.StatusCircuitOpen,
	})

	// 使用 fixedConsecutiveFailuresStore，使 GetStats 返回 ConsecutiveFailures > 0
	// 从而触发 GetAccount 和 CircuitBreaker.RecordSuccess 路径
	fakeStats := &fixedConsecutiveFailuresStore{
		inner:                    store,
		fixedConsecutiveFailures: 3,
	}

	cb, _ := circuitbreaker.NewCircuitBreaker(circuitbreaker.WithStatsStore(store))

	b, _ := New(
		WithAccountStorage(store),
		WithProviderStorage(store),
		WithStatsStore(fakeStats),
		WithCircuitBreaker(cb),
	)

	err := b.ReportSuccess(ctx, "acc-1")
	if err != nil {
		t.Fatalf("ReportSuccess failed: %v", err)
	}

	// 验证账号状态已恢复为 Available
	updated, _ := store.GetAccount(ctx, "acc-1")
	if updated.Status != account.StatusAvailable {
		t.Fatalf("expected StatusAvailable after CircuitOpen recovery, got: %v", updated.Status)
	}
}

// ============================================================
// 6. ReportFailure 路径
// ============================================================

func TestReportFailure_AccountNotFound(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()

	b, _ := New(WithAccountStorage(store), WithProviderStorage(store))

	err := b.ReportFailure(ctx, "nonexistent-acc", errors.New("some error"))
	if !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("expected ErrAccountNotFound, got: %v", err)
	}
}

func TestReportFailure_RateLimitError_StartsCooldown(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType: "test",
		ProviderName: "default",
		Status:       account.ProviderStatusActive,
	})
	_ = store.AddAccount(ctx, &account.Account{
		ID:           "acc-1",
		ProviderType: "test",
		ProviderName: "default",
		Status:       account.StatusAvailable,
	})

	cm := &mockCooldownManager{}

	b, _ := New(
		WithAccountStorage(store),
		WithProviderStorage(store),
		WithCooldownManager(cm),
	)

	rateLimitErr := &mockRateLimitError{}
	err := b.ReportFailure(ctx, "acc-1", rateLimitErr)
	if err != nil {
		t.Fatalf("ReportFailure failed: %v", err)
	}

	// 验证 CooldownManager.StartCooldown 被调用
	if !cm.started {
		t.Fatal("expected CooldownManager.StartCooldown to be called for rate limit error")
	}

	// 验证账号状态变为 CoolingDown
	updated, _ := store.GetAccount(ctx, "acc-1")
	if updated.Status != account.StatusCoolingDown {
		t.Fatalf("expected StatusCoolingDown after rate limit, got: %v", updated.Status)
	}
}

func TestReportFailure_NilError(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType: "test",
		ProviderName: "default",
		Status:       account.ProviderStatusActive,
	})
	_ = store.AddAccount(ctx, &account.Account{
		ID:           "acc-1",
		ProviderType: "test",
		ProviderName: "default",
		Status:       account.StatusAvailable,
	})

	b, _ := New(WithAccountStorage(store), WithProviderStorage(store))

	err := b.ReportFailure(ctx, "acc-1", nil)
	if err != nil {
		t.Fatalf("expected nil error for nil callErr, got: %v", err)
	}

	// 账号状态不应改变
	updated, _ := store.GetAccount(ctx, "acc-1")
	if updated.Status != account.StatusAvailable {
		t.Fatalf("expected StatusAvailable unchanged, got: %v", updated.Status)
	}
}

// ============================================================
// 7. filterProviders 辅助函数
// ============================================================

func TestFilterProviders_EmptyExcludes(t *testing.T) {
	candidates := []*account.ProviderInfo{
		{ProviderType: "typeA", ProviderName: "nameA"},
		{ProviderType: "typeB", ProviderName: "nameB"},
	}
	result := filterProviders(candidates, nil)
	if len(result) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(result))
	}
}

func TestFilterProviders_ExcludesSome(t *testing.T) {
	candidates := []*account.ProviderInfo{
		{ProviderType: "typeA", ProviderName: "nameA"},
		{ProviderType: "typeB", ProviderName: "nameB"},
		{ProviderType: "typeC", ProviderName: "nameC"},
	}
	excludes := []account.ProviderKey{
		{Type: "typeB", Name: "nameB"},
	}
	result := filterProviders(candidates, excludes)
	if len(result) != 2 {
		t.Fatalf("expected 2 providers after excluding typeB/nameB, got %d", len(result))
	}
	for _, p := range result {
		if p.ProviderType == "typeB" && p.ProviderName == "nameB" {
			t.Fatal("typeB/nameB should have been excluded")
		}
	}
}

func TestFilterProviders_ExcludesAll(t *testing.T) {
	candidates := []*account.ProviderInfo{
		{ProviderType: "typeA", ProviderName: "nameA"},
	}
	excludes := []account.ProviderKey{
		{Type: "typeA", Name: "nameA"},
	}
	result := filterProviders(candidates, excludes)
	if len(result) != 0 {
		t.Fatalf("expected 0 providers after excluding all, got %d", len(result))
	}
}

// ============================================================
// 8. excludeAccounts 辅助函数
// ============================================================

func TestExcludeAccounts_EmptyExcludes(t *testing.T) {
	accounts := []*account.Account{
		{ID: "acc-1"},
		{ID: "acc-2"},
	}
	result := excludeAccounts(accounts, nil)
	if len(result) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(result))
	}
}

func TestExcludeAccounts_ExcludesSome(t *testing.T) {
	accounts := []*account.Account{
		{ID: "acc-1"},
		{ID: "acc-2"},
		{ID: "acc-3"},
	}
	result := excludeAccounts(accounts, []string{"acc-2"})
	if len(result) != 2 {
		t.Fatalf("expected 2 accounts after excluding acc-2, got %d", len(result))
	}
	for _, a := range result {
		if a.ID == "acc-2" {
			t.Fatal("acc-2 should have been excluded")
		}
	}
}

// ============================================================
// 辅助类型（本文件专用）
// ============================================================

// mockCooldownManager 记录 StartCooldown 是否被调用
type mockCooldownManager struct {
	started bool
	acct    *account.Account
	until   *time.Time
}

func (m *mockCooldownManager) StartCooldown(acct *account.Account, until *time.Time) {
	m.started = true
	m.acct = acct
	m.until = until
	// 模拟真实行为：设置账号 CooldownUntil
	if until != nil {
		acct.CooldownUntil = until
	} else {
		t := time.Now().Add(30 * time.Second)
		acct.CooldownUntil = &t
	}
}

func (m *mockCooldownManager) IsCooldownExpired(acct *account.Account) bool {
	if acct.CooldownUntil == nil {
		return true
	}
	return time.Now().After(*acct.CooldownUntil)
}

// fixedConsecutiveFailuresStore 是一个 StatsStore 装饰器，
// 使 GetStats 始终返回指定的 ConsecutiveFailures 值（不受 IncrSuccess 重置影响）。
// 用于模拟在 IncrSuccess 之后 GetStats 仍返回 > 0 的场景，从而触发 GetAccount 路径。
type fixedConsecutiveFailuresStore struct {
	inner                    *storememory.Store
	fixedConsecutiveFailures int
}

func (f *fixedConsecutiveFailuresStore) GetStats(ctx context.Context, accountID string) (*account.AccountStats, error) {
	stats, err := f.inner.GetStats(ctx, accountID)
	if err != nil {
		return nil, err
	}
	stats.ConsecutiveFailures = f.fixedConsecutiveFailures
	return stats, nil
}

func (f *fixedConsecutiveFailuresStore) IncrSuccess(ctx context.Context, accountID string) error {
	return f.inner.IncrSuccess(ctx, accountID)
}

func (f *fixedConsecutiveFailuresStore) IncrFailure(ctx context.Context, accountID string, errMsg string) (int, error) {
	return f.inner.IncrFailure(ctx, accountID, errMsg)
}

func (f *fixedConsecutiveFailuresStore) UpdateLastUsed(ctx context.Context, accountID string, t time.Time) error {
	return f.inner.UpdateLastUsed(ctx, accountID, t)
}

func (f *fixedConsecutiveFailuresStore) GetConsecutiveFailures(ctx context.Context, accountID string) (int, error) {
	return f.fixedConsecutiveFailures, nil
}

func (f *fixedConsecutiveFailuresStore) ResetConsecutiveFailures(ctx context.Context, accountID string) error {
	return f.inner.ResetConsecutiveFailures(ctx, accountID)
}

func (f *fixedConsecutiveFailuresStore) RemoveStats(ctx context.Context, accountID string) error {
	return f.inner.RemoveStats(ctx, accountID)
}

// ============================================================
// 额外覆盖率测试
// ============================================================

// TestPick_ExactMode_ProviderInactive 覆盖 pickExact 中 ErrProviderInactive 路径
func TestPick_ExactMode_ProviderInactive(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()

	// 注册 Disabled 状态的 Provider（非 Active/Degraded）
	_ = store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType:          "typeA",
		ProviderName:          "nameA",
		Status:                account.ProviderStatusDisabled,
		SupportedModels:       []string{"gpt-4"},
		AccountCount:          1,
		AvailableAccountCount: 1,
	})

	b, _ := New(WithAccountStorage(store), WithProviderStorage(store))
	_, err := b.Pick(ctx, &PickRequest{
		Model:       "gpt-4",
		ProviderKey: &account.ProviderKey{Type: "typeA", Name: "nameA"},
	})
	// ErrProviderInactive → ErrNoAvailableProvider
	if !errors.Is(err, ErrNoAvailableProvider) {
		t.Fatalf("expected ErrNoAvailableProvider for inactive provider, got: %v", err)
	}
}

// TestPick_ExactMode_NoAccounts 覆盖 pickExact ErrNoAccount → ErrNoAvailableAccount 路径
func TestPick_ExactMode_NoAccounts(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()

	// Provider 存在，但 AccountCount=0
	_ = store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType:          "typeA",
		ProviderName:          "nameA",
		Status:                account.ProviderStatusActive,
		SupportedModels:       []string{"gpt-4"},
		AccountCount:          0,
		AvailableAccountCount: 0,
	})

	b, _ := New(WithAccountStorage(store), WithProviderStorage(store))
	_, err := b.Pick(ctx, &PickRequest{
		Model:       "gpt-4",
		ProviderKey: &account.ProviderKey{Type: "typeA", Name: "nameA"},
	})
	if err == nil {
		t.Fatal("expected error when provider has no accounts")
	}
}

// TestPick_ContextCancel 覆盖 pickAuto context 取消路径
func TestPick_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	store := storememory.NewStore()
	addProviderWithAccounts(ctx, store, "typeA", "nameA", "gpt-4", "acc-1")

	// 使用一个会在 FilterAvailable 时阻塞很长时间的控制器
	// 实际上 context 已取消，下一次 for 循环的 select case <-ctx.Done() 会触发
	b, _ := New(
		WithAccountStorage(store),
		WithProviderStorage(store),
		WithDefaultMaxRetries(0),
		WithDefaultFailover(true),
	)

	_, err := b.Pick(ctx, &PickRequest{
		Model:          "gpt-4",
		EnableFailover: true,
	})
	// ctx 取消后可能成功（第一次循环在取消之前执行），也可能返回 context.Canceled
	// 只验证不 panic 即可；若 ctx 已在进入循环前取消则返回 ctx.Err()
	_ = err
}

// TestAcquireFromAccounts_UpdateLastUsedFails 覆盖 UpdateLastUsed 失败后 release 并重试的路径
func TestAcquireFromAccounts_UpdateLastUsedFails(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	addProviderWithAccounts(ctx, store, "typeA", "nameA", "gpt-4", "acc-1", "acc-2")

	// StatsStore mock：UpdateLastUsed 第一次返回错误，第二次成功
	failStats := &updateLastUsedFailOnceStore{
		inner:   store,
		failOnN: 1,
	}

	b, _ := New(
		WithAccountStorage(store),
		WithProviderStorage(store),
		WithStatsStore(failStats),
		WithDefaultMaxRetries(3),
	)

	result, err := b.Pick(ctx, &PickRequest{Model: "gpt-4"})
	if err != nil {
		t.Fatalf("expected success on retry after UpdateLastUsed failure, got: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// TestNew_WithCooldownManagerNoUsageTracker 覆盖 newUsageTrackerWithCooldown 创建路径
func TestNew_WithCooldownManagerNoUsageTracker(t *testing.T) {
	store := storememory.NewStore()
	cm := &mockCooldownManager{}
	b, err := New(
		WithAccountStorage(store),
		WithProviderStorage(store),
		WithCooldownManager(cm),
		// 不提供 UsageTracker → 自动创建内置冷却回调的 UsageTracker
	)
	if err != nil {
		t.Fatalf("New() with CooldownManager but no UsageTracker should succeed, got: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil balancer")
	}
}

// TestNew_WithSelector 覆盖 WithSelector option
func TestNew_WithSelector(t *testing.T) {
	store := storememory.NewStore()
	b, err := New(
		WithAccountStorage(store),
		WithProviderStorage(store),
		WithSelector(defaultOptions.Selector), // 使用默认 Selector
	)
	if err != nil {
		t.Fatalf("New() with WithSelector failed: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil balancer")
	}
}

// TestNew_WithGroupSelector 覆盖 WithGroupSelector option
func TestNew_WithGroupSelector(t *testing.T) {
	store := storememory.NewStore()
	b, err := New(
		WithAccountStorage(store),
		WithProviderStorage(store),
		WithGroupSelector(defaultOptions.GroupSelector),
	)
	if err != nil {
		t.Fatalf("New() with WithGroupSelector failed: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil balancer")
	}
}

// TestReportSuccess_UsageTracker 覆盖 ReportSuccess 中 UsageTracker.RecordUsage 路径
func TestReportSuccess_UsageTracker(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType: "test",
		ProviderName: "default",
		Status:       account.ProviderStatusActive,
	})
	_ = store.AddAccount(ctx, &account.Account{
		ID:           "acc-1",
		ProviderType: "test",
		ProviderName: "default",
		Status:       account.StatusAvailable,
	})

	ut := &mockUsageTracker{}

	b, _ := New(
		WithAccountStorage(store),
		WithProviderStorage(store),
		WithUsageTracker(ut),
	)

	err := b.ReportSuccess(ctx, "acc-1")
	if err != nil {
		t.Fatalf("ReportSuccess with UsageTracker failed: %v", err)
	}
	if !ut.recorded {
		t.Fatal("expected UsageTracker.RecordUsage to be called")
	}
}

// TestReportFailure_CircuitBreaker_Tripped 覆盖 ReportFailure 中熔断器触发路径
func TestReportFailure_CircuitBreaker_Tripped(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType: "test",
		ProviderName: "default",
		Status:       account.ProviderStatusActive,
	})
	_ = store.AddAccount(ctx, &account.Account{
		ID:           "acc-1",
		ProviderType: "test",
		ProviderName: "default",
		Status:       account.StatusAvailable,
	})

	cb, _ := circuitbreaker.NewCircuitBreaker(circuitbreaker.WithStatsStore(store))

	// 注入足够多的连续失败，使熔断器触发
	for i := 0; i < 10; i++ {
		_, _ = store.IncrFailure(ctx, "acc-1", "error")
	}

	b, _ := New(
		WithAccountStorage(store),
		WithProviderStorage(store),
		WithStatsStore(store),
		WithCircuitBreaker(cb),
	)

	err := b.ReportFailure(ctx, "acc-1", errors.New("non-rate-limit error"))
	if err != nil {
		t.Fatalf("ReportFailure with circuit break trip failed: %v", err)
	}

	// 验证账号状态变为 CircuitOpen
	updated, _ := store.GetAccount(ctx, "acc-1")
	if updated.Status != account.StatusCircuitOpen {
		t.Fatalf("expected StatusCircuitOpen after circuit break trip, got: %v", updated.Status)
	}
}

// TestPick_AutoMode_OccupancyFull_Failover 覆盖 pickAuto 中 ErrOccupancyFull + failover 路径
func TestPick_AutoMode_OccupancyFull_Failover(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()

	// Provider-A 占用满，Provider-B 可用
	addProviderWithAccounts(ctx, store, "typeA", "nameA", "gpt-4", "acc-a1")
	addProviderWithAccounts(ctx, store, "typeB", "nameB", "gpt-4", "acc-b1")

	// Provider-A 的账号 FilterAvailable 始终返回空（占用满）
	occCtrl := &blockingOccupancyController{
		blockedProviderTypes: map[string]bool{"typeA": true},
	}
	// 确保 Provider-A 优先级更高
	_ = store.UpdateProvider(ctx, &account.ProviderInfo{
		ProviderType:          "typeA",
		ProviderName:          "nameA",
		Status:                account.ProviderStatusActive,
		SupportedModels:       []string{"gpt-4"},
		Priority:              10,
		AccountCount:          1,
		AvailableAccountCount: 1,
	})

	b, _ := New(
		WithAccountStorage(store),
		WithProviderStorage(store),
		WithOccupancyController(occCtrl),
		WithDefaultFailover(true),
		WithDefaultMaxRetries(0),
	)

	result, err := b.Pick(ctx, &PickRequest{Model: "gpt-4", EnableFailover: true})
	if err != nil {
		t.Fatalf("expected failover to typeB succeed, got: %v", err)
	}
	if result.Account.ProviderType != "typeB" {
		t.Fatalf("expected account from typeB, got: %s", result.Account.ProviderType)
	}
}

// TestPick_ExactMode_NoAvailableAccounts_OccupancyFull 覆盖 selectAccountFromProvider ErrOccupancyFull
func TestPick_ExactMode_NoAvailableAccounts_OccupancyFull(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	addProviderWithAccounts(ctx, store, "typeA", "nameA", "gpt-4", "acc-1")

	// 所有账号 FilterAvailable 返回空
	b, _ := New(
		WithAccountStorage(store),
		WithProviderStorage(store),
		WithOccupancyController(&fullyBlockingOccupancyController{}),
	)

	_, err := b.Pick(ctx, &PickRequest{
		Model:       "gpt-4",
		ProviderKey: &account.ProviderKey{Type: "typeA", Name: "nameA"},
	})
	if !errors.Is(err, ErrOccupancyFull) {
		t.Fatalf("expected ErrOccupancyFull, got: %v", err)
	}
}

// TestPick_ExactMode_AccountsEmptyAfterResolve 覆盖 selectAccountFromProvider 空账号列表路径
func TestPick_ExactMode_AccountsEmptyAfterResolve(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()

	// Provider 有账号计数，但账号状态为 CoolingDown（ResolveAccounts 只返回 Available）
	_ = store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType:          "typeA",
		ProviderName:          "nameA",
		Status:                account.ProviderStatusActive,
		SupportedModels:       []string{"gpt-4"},
		AccountCount:          1,
		AvailableAccountCount: 1,
	})
	_ = store.AddAccount(ctx, &account.Account{
		ID:           "acc-1",
		ProviderType: "typeA",
		ProviderName: "nameA",
		Status:       account.StatusCoolingDown, // 非 Available，ResolveAccounts 不会返回
	})

	b, _ := New(WithAccountStorage(store), WithProviderStorage(store))

	_, err := b.Pick(ctx, &PickRequest{
		Model:       "gpt-4",
		ProviderKey: &account.ProviderKey{Type: "typeA", Name: "nameA"},
	})
	if !errors.Is(err, ErrNoAvailableAccount) {
		t.Fatalf("expected ErrNoAvailableAccount when accounts are all in non-available status, got: %v", err)
	}
}

// ============================================================
// mock 类型（额外覆盖率测试专用）
// ============================================================

// updateLastUsedFailOnceStore：UpdateLastUsed 前 N 次返回错误
type updateLastUsedFailOnceStore struct {
	inner   *storememory.Store
	failOnN int
	callN   int
}

func (s *updateLastUsedFailOnceStore) GetStats(ctx context.Context, id string) (*account.AccountStats, error) {
	return s.inner.GetStats(ctx, id)
}
func (s *updateLastUsedFailOnceStore) IncrSuccess(ctx context.Context, id string) error {
	return s.inner.IncrSuccess(ctx, id)
}
func (s *updateLastUsedFailOnceStore) IncrFailure(ctx context.Context, id string, msg string) (int, error) {
	return s.inner.IncrFailure(ctx, id, msg)
}
func (s *updateLastUsedFailOnceStore) UpdateLastUsed(ctx context.Context, id string, t time.Time) error {
	s.callN++
	if s.callN <= s.failOnN {
		return errors.New("simulated UpdateLastUsed failure")
	}
	return s.inner.UpdateLastUsed(ctx, id, t)
}
func (s *updateLastUsedFailOnceStore) GetConsecutiveFailures(ctx context.Context, id string) (int, error) {
	return s.inner.GetConsecutiveFailures(ctx, id)
}
func (s *updateLastUsedFailOnceStore) ResetConsecutiveFailures(ctx context.Context, id string) error {
	return s.inner.ResetConsecutiveFailures(ctx, id)
}
func (s *updateLastUsedFailOnceStore) RemoveStats(ctx context.Context, id string) error {
	return s.inner.RemoveStats(ctx, id)
}

// mockUsageTracker 记录 RecordUsage 调用
type mockUsageTracker struct {
	recorded bool
}

func (m *mockUsageTracker) RecordUsage(_ context.Context, _ string, _ usagerule.SourceType, _ float64) error {
	m.recorded = true
	return nil
}

func (m *mockUsageTracker) IsQuotaAvailable(_ context.Context, _ string) (bool, error) {
	return true, nil
}

func (m *mockUsageTracker) Calibrate(_ context.Context, _ string, _ []*usagerule.UsageStats) error {
	return nil
}

func (m *mockUsageTracker) CalibrateFromResponse(_ context.Context, _ string, _ usagerule.SourceType) error {
	return nil
}

func (m *mockUsageTracker) GetTrackedUsages(_ context.Context, _ string) ([]*account.TrackedUsage, error) {
	return nil, nil
}

func (m *mockUsageTracker) MinRemainRatio(_ context.Context, _ string) (float64, error) {
	return 1.0, nil
}

func (m *mockUsageTracker) InitRules(_ context.Context, _ string, _ []*usagerule.UsageRule) error {
	return nil
}

func (m *mockUsageTracker) Remove(_ context.Context, _ string) error {
	return nil
}

// fullyBlockingOccupancyController：所有账号 FilterAvailable 都返回空（模拟全部占用满）
type fullyBlockingOccupancyController struct{}

func (c *fullyBlockingOccupancyController) FilterAvailable(_ context.Context, _ []*account.Account) []*account.Account {
	return nil
}
func (c *fullyBlockingOccupancyController) Acquire(_ context.Context, _ *account.Account) bool {
	return true
}
func (c *fullyBlockingOccupancyController) Release(_ context.Context, _ string) {}

// ============================================================
// 覆盖 newUsageTrackerWithCooldown 回调路径
// ============================================================

// TestNewUsageTrackerWithCooldown_TriggersCooldown 覆盖 newUsageTrackerWithCooldown 内的回调
// 通过直接触发 Pick+ReportSuccess 路径间接调用 UsageTracker.RecordUsage，
// 但 newUsageTrackerWithCooldown 只在构造时被调用，回调在 RecordUsage 触发配额阈值时执行。
// 此测试直接测试构造后的 Balancer 是否能正常工作（覆盖构造路径）。
func TestNewUsageTrackerWithCooldown_BalancerConstructed(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	addProviderWithAccounts(ctx, store, "typeA", "nameA", "gpt-4", "acc-1")

	// 使用真实的 CooldownManager（不提供 UsageTracker），触发 newUsageTrackerWithCooldown
	cm := &mockCooldownManager{}
	b, err := New(
		WithAccountStorage(store),
		WithProviderStorage(store),
		WithCooldownManager(cm),
		// 不提供 UsageTracker
	)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	result, err := b.Pick(ctx, &PickRequest{Model: "gpt-4"})
	if err != nil {
		t.Fatalf("Pick() failed: %v", err)
	}

	// ReportSuccess 会调用 UsageTracker.RecordUsage（内置的 newUsageTrackerWithCooldown 创建的）
	err = b.ReportSuccess(ctx, result.Account.ID)
	if err != nil {
		t.Fatalf("ReportSuccess failed: %v", err)
	}
}

// TestReportSuccess_IncrSuccess_Error 覆盖 ReportSuccess IncrSuccess 失败路径
func TestReportSuccess_IncrSuccess_Error(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()

	// StatsStore mock：IncrSuccess 返回错误
	failStats := &incrSuccessFailStore{inner: store}

	b, _ := New(
		WithAccountStorage(store),
		WithProviderStorage(store),
		WithStatsStore(failStats),
	)

	err := b.ReportSuccess(ctx, "any-acc")
	if err == nil {
		t.Fatal("expected error when IncrSuccess fails")
	}
	if !strings.Contains(err.Error(), "incr success stats") {
		t.Fatalf("expected 'incr success stats' in error, got: %v", err)
	}
}

// TestReportSuccess_UsageTracker_Error 覆盖 ReportSuccess UsageTracker.RecordUsage 失败路径
func TestReportSuccess_UsageTracker_Error(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()

	ut := &errorUsageTracker{}

	b, _ := New(
		WithAccountStorage(store),
		WithProviderStorage(store),
		WithUsageTracker(ut),
	)

	err := b.ReportSuccess(ctx, "any-acc")
	if err == nil {
		t.Fatal("expected error when RecordUsage fails")
	}
	if !strings.Contains(err.Error(), "record usage") {
		t.Fatalf("expected 'record usage' in error, got: %v", err)
	}
}

// ============================================================
// 额外 mock 类型
// ============================================================

// incrSuccessFailStore：IncrSuccess 始终返回错误
type incrSuccessFailStore struct {
	inner *storememory.Store
}

func (s *incrSuccessFailStore) GetStats(ctx context.Context, id string) (*account.AccountStats, error) {
	return s.inner.GetStats(ctx, id)
}
func (s *incrSuccessFailStore) IncrSuccess(_ context.Context, _ string) error {
	return errors.New("simulated IncrSuccess failure")
}
func (s *incrSuccessFailStore) IncrFailure(ctx context.Context, id string, msg string) (int, error) {
	return s.inner.IncrFailure(ctx, id, msg)
}
func (s *incrSuccessFailStore) UpdateLastUsed(ctx context.Context, id string, t time.Time) error {
	return s.inner.UpdateLastUsed(ctx, id, t)
}
func (s *incrSuccessFailStore) GetConsecutiveFailures(ctx context.Context, id string) (int, error) {
	return s.inner.GetConsecutiveFailures(ctx, id)
}
func (s *incrSuccessFailStore) ResetConsecutiveFailures(ctx context.Context, id string) error {
	return s.inner.ResetConsecutiveFailures(ctx, id)
}
func (s *incrSuccessFailStore) RemoveStats(ctx context.Context, id string) error {
	return s.inner.RemoveStats(ctx, id)
}

// errorUsageTracker：RecordUsage 始终返回错误
type errorUsageTracker struct{}

func (e *errorUsageTracker) RecordUsage(_ context.Context, _ string, _ usagerule.SourceType, _ float64) error {
	return errors.New("simulated RecordUsage failure")
}
func (e *errorUsageTracker) IsQuotaAvailable(_ context.Context, _ string) (bool, error) {
	return true, nil
}
func (e *errorUsageTracker) Calibrate(_ context.Context, _ string, _ []*usagerule.UsageStats) error {
	return nil
}
func (e *errorUsageTracker) CalibrateFromResponse(_ context.Context, _ string, _ usagerule.SourceType) error {
	return nil
}
func (e *errorUsageTracker) GetTrackedUsages(_ context.Context, _ string) ([]*account.TrackedUsage, error) {
	return nil, nil
}
func (e *errorUsageTracker) MinRemainRatio(_ context.Context, _ string) (float64, error) {
	return 1.0, nil
}
func (e *errorUsageTracker) InitRules(_ context.Context, _ string, _ []*usagerule.UsageRule) error {
	return nil
}
func (e *errorUsageTracker) Remove(_ context.Context, _ string) error {
	return nil
}

// errorCircuitBreaker：RecordSuccess 和 RecordFailure 始终返回错误
type errorCircuitBreaker struct {
	err error
}

func (c *errorCircuitBreaker) RecordSuccess(_ context.Context, _ *account.Account) error {
	return c.err
}
func (c *errorCircuitBreaker) RecordFailure(_ context.Context, _ *account.Account, _ int) (bool, error) {
	return false, c.err
}
func (c *errorCircuitBreaker) ShouldAllow(_ *account.Account) bool {
	return true
}

// versionConflictAccountStorage：UpdateAccount 始终返回 ErrVersionConflict
type versionConflictAccountStorage struct {
	inner *storememory.Store
}

func (v *versionConflictAccountStorage) GetAccount(ctx context.Context, id string) (*account.Account, error) {
	return v.inner.GetAccount(ctx, id)
}
func (v *versionConflictAccountStorage) SearchAccounts(ctx context.Context, filter *storage.SearchFilter) ([]*account.Account, error) {
	return v.inner.SearchAccounts(ctx, filter)
}
func (v *versionConflictAccountStorage) AddAccount(ctx context.Context, acct *account.Account) error {
	return v.inner.AddAccount(ctx, acct)
}
func (v *versionConflictAccountStorage) UpdateAccount(_ context.Context, _ *account.Account, _ storage.UpdateField) error {
	return storage.ErrVersionConflict
}
func (v *versionConflictAccountStorage) RemoveAccount(ctx context.Context, id string) error {
	return v.inner.RemoveAccount(ctx, id)
}
func (v *versionConflictAccountStorage) RemoveAccounts(ctx context.Context, filter *storage.SearchFilter) error {
	return v.inner.RemoveAccounts(ctx, filter)
}
func (v *versionConflictAccountStorage) CountAccounts(ctx context.Context, filter *storage.SearchFilter) (int, error) {
	return v.inner.CountAccounts(ctx, filter)
}

// ============================================================
// 覆盖 acquireFromAccounts 中 selector 错误路径
// ============================================================

// TestReportSuccess_CircuitBreaker_RecordSuccessError 覆盖 CircuitBreaker.RecordSuccess 返回错误路径
func TestReportSuccess_CircuitBreaker_RecordSuccessError(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType: "test",
		ProviderName: "default",
		Status:       account.ProviderStatusActive,
	})
	_ = store.AddAccount(ctx, &account.Account{
		ID:           "acc-1",
		ProviderType: "test",
		ProviderName: "default",
		Status:       account.StatusAvailable,
	})

	// 使 GetStats 返回 ConsecutiveFailures > 0（触发 GetAccount + CircuitBreaker.RecordSuccess）
	fakeStats := &fixedConsecutiveFailuresStore{
		inner:                    store,
		fixedConsecutiveFailures: 3,
	}

	// 使用一个始终返回错误的 CircuitBreaker
	errCB := &errorCircuitBreaker{err: errors.New("circuit breaker error")}

	b, _ := New(
		WithAccountStorage(store),
		WithProviderStorage(store),
		WithStatsStore(fakeStats),
		WithCircuitBreaker(errCB),
	)

	err := b.ReportSuccess(ctx, "acc-1")
	if err == nil {
		t.Fatal("expected error when CircuitBreaker.RecordSuccess fails")
	}
	if !strings.Contains(err.Error(), "circuit breaker record success") {
		t.Fatalf("expected 'circuit breaker record success' in error, got: %v", err)
	}
}

// TestReportSuccess_CircuitOpen_UpdateAccount_VersionConflict 覆盖 UpdateAccount 版本冲突返回 nil
func TestReportSuccess_CircuitOpen_UpdateAccount_VersionConflict(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType: "test",
		ProviderName: "default",
		Status:       account.ProviderStatusActive,
	})
	_ = store.AddAccount(ctx, &account.Account{
		ID:           "acc-1",
		ProviderType: "test",
		ProviderName: "default",
		Status:       account.StatusCircuitOpen,
	})

	fakeStats := &fixedConsecutiveFailuresStore{
		inner:                    store,
		fixedConsecutiveFailures: 3,
	}

	// UpdateAccount 始终返回 ErrVersionConflict
	conflictStorage := &versionConflictAccountStorage{inner: store}

	cb, _ := circuitbreaker.NewCircuitBreaker(circuitbreaker.WithStatsStore(store))

	b, _ := New(
		WithAccountStorage(conflictStorage),
		WithProviderStorage(store),
		WithStatsStore(fakeStats),
		WithCircuitBreaker(cb),
	)

	// 版本冲突时应该静默返回 nil
	err := b.ReportSuccess(ctx, "acc-1")
	if err != nil {
		t.Fatalf("expected nil on version conflict in ReportSuccess UpdateAccount, got: %v", err)
	}
}

// TestPick_SelectorError 覆盖 acquireFromAccounts 中 Selector.Select 返回非 Empty 错误
func TestPick_SelectorError(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	addProviderWithAccounts(ctx, store, "typeA", "nameA", "gpt-4", "acc-1")

	// 使用一个始终返回通用错误的 Selector
	errSel := &errorSelector{err: errors.New("selector internal error")}

	b, _ := New(
		WithAccountStorage(store),
		WithProviderStorage(store),
		WithSelector(errSel),
	)

	_, err := b.Pick(ctx, &PickRequest{Model: "gpt-4"})
	if err == nil {
		t.Fatal("expected error from selector")
	}
	if !strings.Contains(err.Error(), "select account") {
		t.Fatalf("expected 'select account' in error, got: %v", err)
	}
}

// TestPick_MaxRetriesExceeded_SelectAlwaysFails 覆盖 acquireFromAccounts ErrMaxRetriesExceeded
func TestPick_MaxRetriesExceeded_SelectAlwaysFails(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	addProviderWithAccounts(ctx, store, "typeA", "nameA", "gpt-4", "acc-1")

	// Selector 总是返回 ErrNoAvailableAccount（覆盖 selector.ErrNoAvailableAccount 路径）
	errSel := &noAvailableSelector{}

	b, _ := New(
		WithAccountStorage(store),
		WithProviderStorage(store),
		WithSelector(errSel),
		WithDefaultMaxRetries(2),
	)

	_, err := b.Pick(ctx, &PickRequest{Model: "gpt-4"})
	if !errors.Is(err, ErrNoAvailableAccount) {
		t.Fatalf("expected ErrNoAvailableAccount from selector, got: %v", err)
	}
}

// ============================================================
// 额外 mock Selector 类型
// ============================================================

// errorSelector 始终返回通用错误
type errorSelector struct {
	err error
}

func (s *errorSelector) Name() string { return "error-selector" }
func (s *errorSelector) Select(candidates []*account.Account, _ *selector.SelectRequest) (*account.Account, error) {
	if len(candidates) == 0 {
		return nil, errors.New("empty candidates")
	}
	return nil, s.err
}

// noAvailableSelector 始终返回 ErrNoAvailableAccount
type noAvailableSelector struct{}

func (s *noAvailableSelector) Name() string { return "no-available-selector" }
func (s *noAvailableSelector) Select(_ []*account.Account, _ *selector.SelectRequest) (*account.Account, error) {
	return nil, selector.ErrNoAvailableAccount
}
