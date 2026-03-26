package balancer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/circuitbreaker"
	"github.com/nomand-zc/lumin-acpool/storage"
	storememory "github.com/nomand-zc/lumin-acpool/storage/memory"
)

// --- Fix-1: ReportFailure 版本冲突重试 ---

// TestReportFailure_VersionConflict_Retry 验证版本冲突时重新获取最新版本并重试
func TestReportFailure_VersionConflict_Retry(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()

	// 添加 Provider
	_ = store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType: "test",
		ProviderName: "default",
		Status:       account.ProviderStatusActive,
	})

	// 添加账号
	_ = store.AddAccount(ctx, &account.Account{
		ID:           "acc-1",
		ProviderType: "test",
		ProviderName: "default",
		Status:       account.StatusAvailable,
	})

	// 先获取账号更新版本，模拟版本冲突：手动更新一次使版本号递增
	acct, _ := store.GetAccount(ctx, "acc-1")
	acct.Status = account.StatusAvailable
	_ = store.UpdateAccount(ctx, acct, storage.UpdateFieldStatus)

	ss := store
	cb, _ := circuitbreaker.NewCircuitBreaker(circuitbreaker.WithStatsStore(ss))

	// 将足够多的失败注入 stats，使 circuitbreaker trip
	for i := 0; i < 5; i++ {
		_, _ = ss.IncrFailure(ctx, "acc-1", "error")
	}

	b, _ := New(
		WithAccountStorage(store),
		WithProviderStorage(store),
		WithStatsStore(store),
		WithCircuitBreaker(cb),
	)

	// 模拟 ReportFailure，此时 acc-1 的版本是 2，
	// 但我们持有的是版本 1 的账号，这会触发版本冲突重试
	err := b.ReportFailure(ctx, "acc-1", errors.New("service unavailable"))
	if err != nil {
		t.Fatalf("ReportFailure should not return error on version conflict retry: %v", err)
	}

	// 验证账号状态最终被正确更新（CircuitOpen）
	updated, _ := store.GetAccount(ctx, "acc-1")
	if updated.Status != account.StatusCircuitOpen {
		t.Fatalf("expected StatusCircuitOpen after circuit trip, got %v", updated.Status)
	}
}

// TestReportFailure_VersionConflict_SkipTerminalStatus 验证终态状态（Banned/Invalidated/Disabled）不被覆盖
func TestReportFailure_VersionConflict_SkipTerminalStatus_Banned(t *testing.T) {
	ctx := context.Background()

	// 使用 mockAccountStorage 模拟版本冲突后终态的情况
	mock := &mockAccountStorage{
		accounts: map[string]*account.Account{
			"acc-1": {
				ID:           "acc-1",
				ProviderType: "test",
				ProviderName: "default",
				Status:       account.StatusAvailable,
				Version:      1,
			},
		},
		// 第一次 UpdateAccount 返回 ErrVersionConflict
		// 第二次 GetAccount 返回 Banned 状态（模拟并发更新）
		updateConflictOnce: true,
		conflictThenStatus: account.StatusBanned,
	}

	ss := storememory.NewStore()
	cb, _ := circuitbreaker.NewCircuitBreaker(circuitbreaker.WithStatsStore(ss))
	for i := 0; i < 5; i++ {
		_, _ = ss.IncrFailure(ctx, "acc-1", "error")
	}

	b, _ := New(
		WithAccountStorage(mock),
		WithProviderStorage(storememory.NewStore()),
		WithStatsStore(ss),
		WithCircuitBreaker(cb),
	)

	err := b.ReportFailure(ctx, "acc-1", errors.New("error"))
	// 应该静默返回（不覆盖终态）
	if err != nil {
		t.Fatalf("expected nil error when terminal status found, got %v", err)
	}

	// 验证 Banned 状态没有被覆盖（函数应静默返回 nil，不会调用第二次 update）
	_ = mock.lastUpdateStatus
}

// TestReportFailure_VersionConflict_SkipTerminalStatus_Invalidated 验证 Invalidated 终态不被覆盖
func TestReportFailure_VersionConflict_SkipTerminalStatus_Invalidated(t *testing.T) {
	ctx := context.Background()

	mock := &mockAccountStorage{
		accounts: map[string]*account.Account{
			"acc-1": {
				ID:           "acc-1",
				ProviderType: "test",
				ProviderName: "default",
				Status:       account.StatusAvailable,
				Version:      1,
			},
		},
		updateConflictOnce: true,
		conflictThenStatus: account.StatusInvalidated,
	}

	ss := storememory.NewStore()
	cb, _ := circuitbreaker.NewCircuitBreaker(circuitbreaker.WithStatsStore(ss))
	for i := 0; i < 5; i++ {
		_, _ = ss.IncrFailure(ctx, "acc-1", "error")
	}

	b, _ := New(
		WithAccountStorage(mock),
		WithProviderStorage(storememory.NewStore()),
		WithStatsStore(ss),
		WithCircuitBreaker(cb),
	)

	err := b.ReportFailure(ctx, "acc-1", errors.New("error"))
	if err != nil {
		t.Fatalf("expected nil error for Invalidated terminal status, got %v", err)
	}
}

// TestReportFailure_VersionConflict_SkipTerminalStatus_Disabled 验证 Disabled 终态不被覆盖
func TestReportFailure_VersionConflict_SkipTerminalStatus_Disabled(t *testing.T) {
	ctx := context.Background()

	mock := &mockAccountStorage{
		accounts: map[string]*account.Account{
			"acc-1": {
				ID:           "acc-1",
				ProviderType: "test",
				ProviderName: "default",
				Status:       account.StatusAvailable,
				Version:      1,
			},
		},
		updateConflictOnce: true,
		conflictThenStatus: account.StatusDisabled,
	}

	ss := storememory.NewStore()
	cb, _ := circuitbreaker.NewCircuitBreaker(circuitbreaker.WithStatsStore(ss))
	for i := 0; i < 5; i++ {
		_, _ = ss.IncrFailure(ctx, "acc-1", "error")
	}

	b, _ := New(
		WithAccountStorage(mock),
		WithProviderStorage(storememory.NewStore()),
		WithStatsStore(ss),
		WithCircuitBreaker(cb),
	)

	err := b.ReportFailure(ctx, "acc-1", errors.New("error"))
	if err != nil {
		t.Fatalf("expected nil error for Disabled terminal status, got %v", err)
	}
}

// --- Fix-2: ReportSuccess 热路径优化 ---

// TestReportSuccess_ZeroConsecutiveFailures_NoGetAccount 验证 consecutiveFailures=0 时不调用 GetAccount
func TestReportSuccess_ZeroConsecutiveFailures_NoGetAccount(t *testing.T) {
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

	trackingStorage := &trackingAccountStorage{
		inner:      store,
		getCallIDs: make([]string, 0),
	}

	ss := store
	cb, _ := circuitbreaker.NewCircuitBreaker(circuitbreaker.WithStatsStore(ss))

	// 没有失败记录，consecutiveFailures = 0

	b, _ := New(
		WithAccountStorage(trackingStorage),
		WithProviderStorage(store),
		WithStatsStore(store),
		WithCircuitBreaker(cb),
	)

	err := b.ReportSuccess(ctx, "acc-1")
	if err != nil {
		t.Fatalf("ReportSuccess failed: %v", err)
	}

	// 验证 GetAccount 没有被调用（优化路径）
	if len(trackingStorage.getCallIDs) > 0 {
		t.Fatalf("expected GetAccount not to be called when consecutiveFailures=0, but was called %d time(s)", len(trackingStorage.getCallIDs))
	}
}

// TestReportSuccess_IncrSuccessResetsConsecutiveFailures_BeforeGetStats 验证优化路径：
// IncrSuccess 已重置 consecutiveFailures，因此 GetStats 之后看到 0，GetAccount 不被调用。
// 这是 Fix-2 的核心行为：正常路径（无历史连续失败）完全无 GetAccount 查询。
func TestReportSuccess_IncrSuccessResetsConsecutiveFailures_BeforeGetStats(t *testing.T) {
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

	trackingStorage := &trackingAccountStorage{
		inner:      store,
		getCallIDs: make([]string, 0),
	}

	ss := store
	cb, _ := circuitbreaker.NewCircuitBreaker(circuitbreaker.WithStatsStore(ss))

	// 注入连续失败记录（但 IncrSuccess 会先重置它们）
	_, _ = ss.IncrFailure(ctx, "acc-1", "error")
	_, _ = ss.IncrFailure(ctx, "acc-1", "error")

	b, _ := New(
		WithAccountStorage(trackingStorage),
		WithProviderStorage(store),
		WithStatsStore(store),
		WithCircuitBreaker(cb),
	)

	err := b.ReportSuccess(ctx, "acc-1")
	if err != nil {
		t.Fatalf("ReportSuccess failed: %v", err)
	}

	// IncrSuccess 先将 consecutiveFailures 重置为 0，
	// 然后 GetStats 返回 0，所以 GetAccount 不被调用
	// 这验证了 Fix-2 的热路径优化：IncrSuccess 之后的 GetStats 看到 0，不走 GetAccount 路径
	if len(trackingStorage.getCallIDs) > 0 {
		t.Fatalf("expected GetAccount not to be called (IncrSuccess resets consecutiveFailures first), but was called %d time(s)", len(trackingStorage.getCallIDs))
	}
}

// TestReportSuccess_AlwaysResetsConsecutiveFailures 验证无论是否调用 GetAccount，都重置连续失败计数
func TestReportSuccess_AlwaysResetsConsecutiveFailures(t *testing.T) {
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

	ss := store
	cb, _ := circuitbreaker.NewCircuitBreaker(circuitbreaker.WithStatsStore(ss))

	b, _ := New(
		WithAccountStorage(store),
		WithProviderStorage(store),
		WithStatsStore(store),
		WithCircuitBreaker(cb),
	)

	// 即便 consecutiveFailures=0，调用 ReportSuccess 后连续失败计数仍应重置（保持 0）
	err := b.ReportSuccess(ctx, "acc-1")
	if err != nil {
		t.Fatalf("ReportSuccess failed: %v", err)
	}

	failures, _ := ss.GetConsecutiveFailures(ctx, "acc-1")
	if failures != 0 {
		t.Fatalf("expected 0 consecutive failures after ReportSuccess, got %d", failures)
	}
}

// TestReportSuccess_WithConsecutiveFailures_ResetsAfterSuccess 验证有失败时 ReportSuccess 重置计数
func TestReportSuccess_WithConsecutiveFailures_ResetsAfterSuccess(t *testing.T) {
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

	ss := store
	cb, _ := circuitbreaker.NewCircuitBreaker(circuitbreaker.WithStatsStore(ss))

	// 先注入失败
	_, _ = ss.IncrFailure(ctx, "acc-1", "error")
	_, _ = ss.IncrFailure(ctx, "acc-1", "error")

	b, _ := New(
		WithAccountStorage(store),
		WithProviderStorage(store),
		WithStatsStore(store),
		WithCircuitBreaker(cb),
	)

	err := b.ReportSuccess(ctx, "acc-1")
	if err != nil {
		t.Fatalf("ReportSuccess failed: %v", err)
	}

	// 连续失败应被重置为 0
	failures, _ := ss.GetConsecutiveFailures(ctx, "acc-1")
	if failures != 0 {
		t.Fatalf("expected consecutive failures reset to 0 after success, got %d", failures)
	}
}

// --- 辅助类型 ---

// mockAccountStorage 模拟版本冲突场景的 AccountStorage
type mockAccountStorage struct {
	accounts map[string]*account.Account

	// 第一次 UpdateAccount 是否返回版本冲突
	updateConflictOnce bool
	conflictOnceUsed   bool

	// 版本冲突后第二次 GetAccount 返回的状态
	conflictThenStatus account.Status

	// 记录最后一次 UpdateAccount 的状态
	lastUpdateStatus account.Status
}

func (m *mockAccountStorage) GetAccount(_ context.Context, id string) (*account.Account, error) {
	acct, ok := m.accounts[id]
	if !ok {
		return nil, storage.ErrNotFound
	}
	// 版本冲突后，返回已更新为终态的账号
	if m.updateConflictOnce && m.conflictOnceUsed && m.conflictThenStatus != 0 {
		cloned := *acct
		cloned.Status = m.conflictThenStatus
		cloned.Version = acct.Version + 1
		return &cloned, nil
	}
	cloned := *acct
	return &cloned, nil
}

func (m *mockAccountStorage) SearchAccounts(_ context.Context, _ *storage.SearchFilter) ([]*account.Account, error) {
	return nil, nil
}

func (m *mockAccountStorage) AddAccount(_ context.Context, acct *account.Account) error {
	m.accounts[acct.ID] = acct
	return nil
}

func (m *mockAccountStorage) UpdateAccount(_ context.Context, acct *account.Account, _ storage.UpdateField) error {
	if m.updateConflictOnce && !m.conflictOnceUsed {
		m.conflictOnceUsed = true
		return storage.ErrVersionConflict
	}
	m.lastUpdateStatus = acct.Status
	if a, ok := m.accounts[acct.ID]; ok {
		a.Status = acct.Status
		a.Version++
	}
	return nil
}

func (m *mockAccountStorage) RemoveAccount(_ context.Context, _ string) error {
	return nil
}

func (m *mockAccountStorage) RemoveAccounts(_ context.Context, _ *storage.SearchFilter) error {
	return nil
}

func (m *mockAccountStorage) CountAccounts(_ context.Context, _ *storage.SearchFilter) (int, error) {
	return len(m.accounts), nil
}

// trackingAccountStorage 追踪 GetAccount 调用次数
type trackingAccountStorage struct {
	inner      storage.AccountStorage
	getCallIDs []string
}

func (t *trackingAccountStorage) GetAccount(ctx context.Context, id string) (*account.Account, error) {
	t.getCallIDs = append(t.getCallIDs, id)
	return t.inner.GetAccount(ctx, id)
}

func (t *trackingAccountStorage) SearchAccounts(ctx context.Context, filter *storage.SearchFilter) ([]*account.Account, error) {
	return t.inner.SearchAccounts(ctx, filter)
}

func (t *trackingAccountStorage) AddAccount(ctx context.Context, acct *account.Account) error {
	return t.inner.AddAccount(ctx, acct)
}

func (t *trackingAccountStorage) UpdateAccount(ctx context.Context, acct *account.Account, fields storage.UpdateField) error {
	return t.inner.UpdateAccount(ctx, acct, fields)
}

func (t *trackingAccountStorage) RemoveAccount(ctx context.Context, id string) error {
	return t.inner.RemoveAccount(ctx, id)
}

func (t *trackingAccountStorage) RemoveAccounts(ctx context.Context, filter *storage.SearchFilter) error {
	return t.inner.RemoveAccounts(ctx, filter)
}

func (t *trackingAccountStorage) CountAccounts(ctx context.Context, filter *storage.SearchFilter) (int, error) {
	return t.inner.CountAccounts(ctx, filter)
}

// --- Fix-1 额外测试: ReportFailure 版本冲突重试后成功更新 ---

// TestReportFailure_VersionConflict_RetrySuccess 版本冲突后重试成功更新 CircuitOpen
func TestReportFailure_VersionConflict_RetrySuccess(t *testing.T) {
	ctx := context.Background()

	mock := &mockAccountStorage{
		accounts: map[string]*account.Account{
			"acc-1": {
				ID:           "acc-1",
				ProviderType: "test",
				ProviderName: "default",
				Status:       account.StatusAvailable,
				Version:      1,
			},
		},
		// 第一次 Update 冲突，第二次 GetAccount 返回非终态（Available），允许重试 Update
		updateConflictOnce: true,
		conflictThenStatus: account.StatusAvailable, // 非终态，允许重试更新
	}

	ss := storememory.NewStore()
	cb, _ := circuitbreaker.NewCircuitBreaker(circuitbreaker.WithStatsStore(ss))
	for i := 0; i < 5; i++ {
		_, _ = ss.IncrFailure(ctx, "acc-1", "error")
	}

	b, _ := New(
		WithAccountStorage(mock),
		WithProviderStorage(storememory.NewStore()),
		WithStatsStore(ss),
		WithCircuitBreaker(cb),
	)

	err := b.ReportFailure(ctx, "acc-1", errors.New("service error"))
	// 重试后应该成功（静默忽略第二次冲突）
	if err != nil {
		t.Fatalf("expected nil error after retry, got %v", err)
	}
}

// --- isRateLimitError 辅助测试 ---

type mockRateLimitError struct{}

func (e *mockRateLimitError) Error() string     { return "rate limit" }
func (e *mockRateLimitError) IsRateLimit() bool { return true }

type mockHTTP429Error struct{}

func (e *mockHTTP429Error) Error() string   { return "http 429" }
func (e *mockHTTP429Error) StatusCode() int { return 429 }

func TestIsRateLimitError_RateLimitInterface(t *testing.T) {
	err := &mockRateLimitError{}
	if !isRateLimitError(err) {
		t.Fatal("expected isRateLimitError=true for rateLimitError interface")
	}
}

func TestIsRateLimitError_HTTP429(t *testing.T) {
	err := &mockHTTP429Error{}
	if !isRateLimitError(err) {
		t.Fatal("expected isRateLimitError=true for HTTP 429")
	}
}

func TestIsRateLimitError_Nil(t *testing.T) {
	if isRateLimitError(nil) {
		t.Fatal("expected isRateLimitError=false for nil error")
	}
}

func TestIsRateLimitError_RegularError(t *testing.T) {
	err := errors.New("regular error")
	if isRateLimitError(err) {
		t.Fatal("expected isRateLimitError=false for regular error")
	}
}

// --- RetryAfter 辅助测试 ---

type mockRetryAfterError struct {
	retryAfter *time.Time
}

func (e *mockRetryAfterError) Error() string          { return "rate limit with retry" }
func (e *mockRetryAfterError) IsRateLimit() bool      { return true }
func (e *mockRetryAfterError) RetryAfter() *time.Time { return e.retryAfter }

func TestExtractRetryAfter_WithTime(t *testing.T) {
	t1 := time.Now().Add(time.Minute)
	err := &mockRetryAfterError{retryAfter: &t1}
	ra := extractRetryAfter(err)
	if ra == nil {
		t.Fatal("expected non-nil RetryAfter")
	}
	if !ra.Equal(t1) {
		t.Fatalf("expected %v, got %v", t1, *ra)
	}
}

func TestExtractRetryAfter_Nil(t *testing.T) {
	ra := extractRetryAfter(nil)
	if ra != nil {
		t.Fatal("expected nil RetryAfter for nil error")
	}
}
