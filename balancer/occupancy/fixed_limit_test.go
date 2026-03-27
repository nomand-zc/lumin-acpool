package occupancy

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	storememory "github.com/nomand-zc/lumin-acpool/storage/memory"
	"github.com/nomand-zc/lumin-acpool/usagetracker"
	"github.com/nomand-zc/lumin-client/usagerule"
)

// ==================== FixedLimit: getLimit 五层优先级 ====================

// TestFixedLimit_GetLimit_MetadataOverride: Account.Metadata["occupancy_limit"] 最高优先级
func TestFixedLimit_GetLimit_MetadataOverride(t *testing.T) {
	fl := NewFixedLimit(1,
		WithAccountLimit("acc-1", 3),
		WithProviderTypeLimit("type-a", 2),
	)
	acct := &account.Account{
		ID:           "acc-1",
		ProviderType: "type-a",
		ProviderName: "name-a",
		Metadata:     map[string]any{MetaKeyOccupancyLimit: int64(5)},
	}
	limit := fl.getLimit(acct)
	if limit != 5 {
		t.Fatalf("expected limit=5 from Metadata, got %d", limit)
	}
}

// TestFixedLimit_GetLimit_AccountID: accountLimits 优先于 ProviderKey/ProviderType
func TestFixedLimit_GetLimit_AccountID(t *testing.T) {
	fl := NewFixedLimit(1,
		WithAccountLimit("acc-1", 3),
		WithProviderTypeLimit("type-a", 2),
	)
	acct := &account.Account{
		ID:           "acc-1",
		ProviderType: "type-a",
		ProviderName: "name-a",
	}
	limit := fl.getLimit(acct)
	if limit != 3 {
		t.Fatalf("expected limit=3 from accountLimits, got %d", limit)
	}
}

// TestFixedLimit_GetLimit_ProviderKey: providerKeyLimits 优先于 ProviderType
func TestFixedLimit_GetLimit_ProviderKey(t *testing.T) {
	key := account.BuildProviderKey("type-a", "name-a")
	fl := NewFixedLimit(1,
		WithProviderKeyLimit(key, 4),
		WithProviderTypeLimit("type-a", 2),
	)
	acct := &account.Account{
		ID:           "acc-other",
		ProviderType: "type-a",
		ProviderName: "name-a",
	}
	limit := fl.getLimit(acct)
	if limit != 4 {
		t.Fatalf("expected limit=4 from providerKeyLimits, got %d", limit)
	}
}

// TestFixedLimit_GetLimit_ProviderType: 无上层配置时使用 providerTypeLimits
func TestFixedLimit_GetLimit_ProviderType(t *testing.T) {
	fl := NewFixedLimit(1,
		WithProviderTypeLimit("type-a", 2),
	)
	acct := &account.Account{
		ID:           "acc-other",
		ProviderType: "type-a",
		ProviderName: "name-b",
	}
	limit := fl.getLimit(acct)
	if limit != 2 {
		t.Fatalf("expected limit=2 from providerTypeLimits, got %d", limit)
	}
}

// TestFixedLimit_GetLimit_Default: 所有层都无配置时返回 defaultLimit
func TestFixedLimit_GetLimit_Default(t *testing.T) {
	fl := NewFixedLimit(7)
	acct := &account.Account{
		ID:           "acc-other",
		ProviderType: "type-x",
		ProviderName: "name-x",
	}
	limit := fl.getLimit(acct)
	if limit != 7 {
		t.Fatalf("expected limit=7 (default), got %d", limit)
	}
}

// TestFixedLimit_GetLimit_PriorityOrder: 同时设置多层，高优先级覆盖低优先级
func TestFixedLimit_GetLimit_PriorityOrder(t *testing.T) {
	key := account.BuildProviderKey("type-a", "name-a")
	fl := NewFixedLimit(10,
		WithProviderTypeLimit("type-a", 4),
		WithProviderKeyLimit(key, 3),
		WithAccountLimit("acc-1", 2),
	)

	// 有 Metadata: 返回 Metadata 值
	acct := &account.Account{
		ID:           "acc-1",
		ProviderType: "type-a",
		ProviderName: "name-a",
		Metadata:     map[string]any{MetaKeyOccupancyLimit: int64(1)},
	}
	if got := fl.getLimit(acct); got != 1 {
		t.Fatalf("priority: expected 1 (metadata), got %d", got)
	}

	// 无 Metadata: 返回 accountLimits
	acct.Metadata = nil
	if got := fl.getLimit(acct); got != 2 {
		t.Fatalf("priority: expected 2 (account), got %d", got)
	}

	// 无 accountLimits: 返回 ProviderKey
	acct.ID = "acc-x"
	if got := fl.getLimit(acct); got != 3 {
		t.Fatalf("priority: expected 3 (providerKey), got %d", got)
	}

	// 无 ProviderKey 匹配: 返回 ProviderType
	acct.ProviderName = "name-other"
	if got := fl.getLimit(acct); got != 4 {
		t.Fatalf("priority: expected 4 (providerType), got %d", got)
	}

	// 无 ProviderType 匹配: 返回 default
	acct.ProviderType = "type-z"
	if got := fl.getLimit(acct); got != 10 {
		t.Fatalf("priority: expected 10 (default), got %d", got)
	}
}

// ==================== FixedLimit: FilterAvailable ====================

// TestFixedLimit_FilterAvailable_Empty: 空账号列表直接返回空
func TestFixedLimit_FilterAvailable_Empty(t *testing.T) {
	fl := NewFixedLimit(3)
	ctx := context.Background()
	result := fl.FilterAvailable(ctx, nil)
	if len(result) != 0 {
		t.Fatalf("expected empty, got %d", len(result))
	}

	result = fl.FilterAvailable(ctx, []*account.Account{})
	if len(result) != 0 {
		t.Fatalf("expected empty slice, got %d", len(result))
	}
}

// TestFixedLimit_FilterAvailable_AllUnderLimit: 所有账号占用 < 上限，全部通过
func TestFixedLimit_FilterAvailable_AllUnderLimit(t *testing.T) {
	store := storememory.NewStore()
	fl := NewFixedLimit(3, WithStore(store))
	ctx := context.Background()

	accounts := []*account.Account{
		{ID: "acc-1"},
		{ID: "acc-2"},
		{ID: "acc-3"},
	}
	// acc-1 占用 2，acc-2 占用 0，acc-3 占用 1，都在 limit=3 内
	_, _ = store.IncrOccupancy(ctx, "acc-1")
	_, _ = store.IncrOccupancy(ctx, "acc-1")
	_, _ = store.IncrOccupancy(ctx, "acc-3")

	result := fl.FilterAvailable(ctx, accounts)
	if len(result) != 3 {
		t.Fatalf("expected 3, got %d", len(result))
	}
}

// TestFixedLimit_FilterAvailable_SomeAtLimit: 部分账号已满，只返回可用的
func TestFixedLimit_FilterAvailable_SomeAtLimit(t *testing.T) {
	store := storememory.NewStore()
	fl := NewFixedLimit(2, WithStore(store))
	ctx := context.Background()

	accounts := []*account.Account{
		{ID: "acc-1"},
		{ID: "acc-2"},
	}
	// acc-1 占用 2（达到上限），acc-2 占用 0
	_, _ = store.IncrOccupancy(ctx, "acc-1")
	_, _ = store.IncrOccupancy(ctx, "acc-1")

	result := fl.FilterAvailable(ctx, accounts)
	if len(result) != 1 {
		t.Fatalf("expected 1 available, got %d", len(result))
	}
	if result[0].ID != "acc-2" {
		t.Fatalf("expected acc-2 to be available, got %s", result[0].ID)
	}
}

// TestFixedLimit_FilterAvailable_AllFull: 所有账号已满，返回空
func TestFixedLimit_FilterAvailable_AllFull(t *testing.T) {
	store := storememory.NewStore()
	fl := NewFixedLimit(1, WithStore(store))
	ctx := context.Background()

	accounts := []*account.Account{
		{ID: "acc-1"},
		{ID: "acc-2"},
	}
	// 两个账号都达到上限 1
	_, _ = store.IncrOccupancy(ctx, "acc-1")
	_, _ = store.IncrOccupancy(ctx, "acc-2")

	result := fl.FilterAvailable(ctx, accounts)
	if len(result) != 0 {
		t.Fatalf("expected 0 available, got %d", len(result))
	}
}

// ==================== FixedLimit: Acquire/Release ====================

// TestFixedLimit_Acquire_Success: 未超限时 Acquire 返回 true，占用计数+1
func TestFixedLimit_Acquire_Success(t *testing.T) {
	store := storememory.NewStore()
	fl := NewFixedLimit(3, WithStore(store))
	ctx := context.Background()

	acct := &account.Account{ID: "acc-1"}
	ok := fl.Acquire(ctx, acct)
	if !ok {
		t.Fatal("expected Acquire to return true")
	}
	// 验证占用计数为 1
	occ, _ := store.GetOccupancies(ctx, []string{"acc-1"})
	if occ["acc-1"] != 1 {
		t.Fatalf("expected occupancy=1, got %d", occ["acc-1"])
	}
}

// TestFixedLimit_Acquire_AtLimit: 当前=上限时 Acquire 返回 false，计数回退
func TestFixedLimit_Acquire_AtLimit(t *testing.T) {
	store := storememory.NewStore()
	fl := NewFixedLimit(2, WithStore(store))
	ctx := context.Background()

	acct := &account.Account{ID: "acc-1"}
	// 先占满
	_, _ = store.IncrOccupancy(ctx, "acc-1")
	_, _ = store.IncrOccupancy(ctx, "acc-1")

	ok := fl.Acquire(ctx, acct)
	if ok {
		t.Fatal("expected Acquire to return false when at limit")
	}
	// 验证计数回退，仍为 2
	occ, _ := store.GetOccupancies(ctx, []string{"acc-1"})
	if occ["acc-1"] != 2 {
		t.Fatalf("expected occupancy still 2 after failed Acquire, got %d", occ["acc-1"])
	}
}

// TestFixedLimit_Acquire_Release_Pair: Acquire 后 Release，占用计数恢复为 0
func TestFixedLimit_Acquire_Release_Pair(t *testing.T) {
	store := storememory.NewStore()
	fl := NewFixedLimit(3, WithStore(store))
	ctx := context.Background()

	acct := &account.Account{ID: "acc-1"}
	ok := fl.Acquire(ctx, acct)
	if !ok {
		t.Fatal("Acquire should succeed")
	}

	fl.Release(ctx, "acc-1")

	occ, _ := store.GetOccupancies(ctx, []string{"acc-1"})
	if occ["acc-1"] != 0 {
		t.Fatalf("expected occupancy=0 after Release, got %d", occ["acc-1"])
	}
}

// TestFixedLimit_Acquire_ConcurrentSafe: 并发 Acquire，最多 limit 个成功
func TestFixedLimit_Acquire_ConcurrentSafe(t *testing.T) {
	const limit = 5
	const goroutines = 20

	store := storememory.NewStore()
	fl := NewFixedLimit(limit, WithStore(store))
	ctx := context.Background()
	acct := &account.Account{ID: "acc-concurrent"}

	var successCount atomic.Int64
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for range goroutines {
		go func() {
			defer wg.Done()
			if fl.Acquire(ctx, acct) {
				successCount.Add(1)
			}
		}()
	}
	wg.Wait()

	got := successCount.Load()
	if got > limit {
		t.Fatalf("expected at most %d successful Acquire, got %d", limit, got)
	}
	if got == 0 {
		t.Fatal("expected at least 1 successful Acquire")
	}
}

// ==================== Unlimited ====================

// TestUnlimited_FilterAvailable_ReturnsAll: 返回所有账号
func TestUnlimited_FilterAvailable_ReturnsAll(t *testing.T) {
	u := NewUnlimited()
	ctx := context.Background()

	accounts := []*account.Account{
		{ID: "acc-1"},
		{ID: "acc-2"},
		{ID: "acc-3"},
	}
	result := u.FilterAvailable(ctx, accounts)
	if len(result) != 3 {
		t.Fatalf("expected 3, got %d", len(result))
	}
}

// TestUnlimited_Acquire_AlwaysTrue: Acquire 始终返回 true
func TestUnlimited_Acquire_AlwaysTrue(t *testing.T) {
	u := NewUnlimited()
	ctx := context.Background()
	for i := range 100 {
		acct := &account.Account{ID: "acc-1"}
		if !u.Acquire(ctx, acct) {
			t.Fatalf("Unlimited.Acquire should always return true, failed at iteration %d", i)
		}
	}
}

// TestUnlimited_Release_NoOp: Release 不报错
func TestUnlimited_Release_NoOp(t *testing.T) {
	u := NewUnlimited()
	ctx := context.Background()
	// 不应 panic 或报错
	u.Release(ctx, "acc-1")
	u.Release(ctx, "")
}

// ==================== AdaptiveLimit 补充测试 ====================

// TestAdaptiveLimit_DefaultValues: 默认值正确
func TestAdaptiveLimit_DefaultValues(t *testing.T) {
	innerTracker := &mockUsageTracker{}
	al := NewAdaptiveLimit(innerTracker)

	if al.factor != 1.0 {
		t.Fatalf("expected factor=1.0, got %f", al.factor)
	}
	if al.minLimit != 1 {
		t.Fatalf("expected minLimit=1, got %d", al.minLimit)
	}
	if al.maxLimit != 0 {
		t.Fatalf("expected maxLimit=0, got %d", al.maxLimit)
	}
	if al.fallbackLimit != 1 {
		t.Fatalf("expected fallbackLimit=1, got %d", al.fallbackLimit)
	}
	if al.store == nil {
		t.Fatal("expected non-nil store")
	}
}

// TestAdaptiveLimit_FilterAvailable_Empty: 空账号列表直接返回空
func TestAdaptiveLimit_FilterAvailable_Empty(t *testing.T) {
	al := NewAdaptiveLimit(&mockUsageTracker{})
	ctx := context.Background()

	result := al.FilterAvailable(ctx, nil)
	if len(result) != 0 {
		t.Fatalf("expected empty, got %d", len(result))
	}

	result = al.FilterAvailable(ctx, []*account.Account{})
	if len(result) != 0 {
		t.Fatalf("expected empty slice, got %d", len(result))
	}
}

// TestAdaptiveLimit_FilterAvailable_NoUsageData_UsesFallback: 无数据时使用 fallbackLimit 过滤
func TestAdaptiveLimit_FilterAvailable_NoUsageData_UsesFallback(t *testing.T) {
	store := storememory.NewStore()
	// mockUsageTracker 返回空 usages
	al := NewAdaptiveLimit(&mockUsageTracker{}, WithAdaptiveStore(store), WithFallbackLimit(2))
	ctx := context.Background()

	accounts := []*account.Account{
		{ID: "acc-1"},
		{ID: "acc-2"},
	}

	// 未超 fallbackLimit=2，所有账号通过
	result := al.FilterAvailable(ctx, accounts)
	if len(result) != 2 {
		t.Fatalf("expected 2 (fallback allows all), got %d", len(result))
	}

	// 让 acc-1 占用达到 fallbackLimit
	_, _ = store.IncrOccupancy(ctx, "acc-1")
	_, _ = store.IncrOccupancy(ctx, "acc-1")

	result = al.FilterAvailable(ctx, accounts)
	if len(result) != 1 {
		t.Fatalf("expected 1 (acc-1 full), got %d", len(result))
	}
	if result[0].ID != "acc-2" {
		t.Fatalf("expected acc-2, got %s", result[0].ID)
	}
}

// TestAdaptiveLimit_Acquire_Success: 未超动态上限时 Acquire 成功
func TestAdaptiveLimit_Acquire_Success(t *testing.T) {
	al := NewAdaptiveLimit(&mockUsageTracker{}, WithFallbackLimit(3))
	ctx := context.Background()

	acct := &account.Account{ID: "acc-1"}
	ok := al.Acquire(ctx, acct)
	if !ok {
		t.Fatal("expected Acquire to succeed")
	}
}

// TestAdaptiveLimit_Acquire_AtLimit: 超动态上限返回 false
func TestAdaptiveLimit_Acquire_AtLimit(t *testing.T) {
	store := storememory.NewStore()
	al := NewAdaptiveLimit(&mockUsageTracker{}, WithAdaptiveStore(store), WithFallbackLimit(1))
	ctx := context.Background()

	acct := &account.Account{ID: "acc-1"}
	// 先占满 fallbackLimit=1
	_, _ = store.IncrOccupancy(ctx, "acc-1")

	ok := al.Acquire(ctx, acct)
	if ok {
		t.Fatal("expected Acquire to fail when at limit")
	}
}

// TestAdaptiveLimit_Release: Release 后占用计数递减
func TestAdaptiveLimit_Release(t *testing.T) {
	store := storememory.NewStore()
	al := NewAdaptiveLimit(&mockUsageTracker{}, WithAdaptiveStore(store), WithFallbackLimit(3))
	ctx := context.Background()

	acct := &account.Account{ID: "acc-1"}
	ok := al.Acquire(ctx, acct)
	if !ok {
		t.Fatal("Acquire should succeed")
	}

	al.Release(ctx, "acc-1")

	occ, _ := store.GetOccupancies(ctx, []string{"acc-1"})
	if occ["acc-1"] != 0 {
		t.Fatalf("expected occupancy=0 after Release, got %d", occ["acc-1"])
	}
}

// TestAdaptiveLimit_MetadataOverridesOptions: Account.Metadata 参数覆盖本地配置
func TestAdaptiveLimit_MetadataOverridesOptions(t *testing.T) {
	store := storememory.NewStore()
	// 本地配置 fallbackLimit=1，但 metadata 设置 fallbackLimit=5
	al := NewAdaptiveLimit(&mockUsageTracker{}, WithAdaptiveStore(store), WithFallbackLimit(1))
	ctx := context.Background()

	acct := &account.Account{
		ID: "acc-1",
		Metadata: map[string]any{
			MetaKeyOccupancyFallbackLimit: int64(5),
		},
	}

	// metadata fallbackLimit=5，占用 3 时仍可 Acquire
	_, _ = store.IncrOccupancy(ctx, "acc-1")
	_, _ = store.IncrOccupancy(ctx, "acc-1")
	_, _ = store.IncrOccupancy(ctx, "acc-1")

	ok := al.Acquire(ctx, acct)
	if !ok {
		t.Fatal("expected Acquire to succeed with metadata fallbackLimit=5")
	}
}

// mockUsageTracker 是最简实现，GetTrackedUsages 返回空（触发 fallbackLimit 路径）
type mockUsageTracker struct{}

func (m *mockUsageTracker) RecordUsage(_ context.Context, _ string, _ usagerule.SourceType, _ float64) error {
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

// ==================== metadata helpers 覆盖 ====================

// TestMetadataInt64_Float64Value: JSON 反序列化的 float64 值也能正确解析
func TestMetadataInt64_Float64Value(t *testing.T) {
	acct := &account.Account{
		Metadata: map[string]any{MetaKeyOccupancyLimit: float64(8)},
	}
	v, ok := metadataInt64(acct, MetaKeyOccupancyLimit)
	if !ok || v != 8 {
		t.Fatalf("expected 8 from float64 metadata, got ok=%v v=%d", ok, v)
	}
}

// TestMetadataInt64_IntValue: int 类型也能正确转换
func TestMetadataInt64_IntValue(t *testing.T) {
	acct := &account.Account{
		Metadata: map[string]any{MetaKeyOccupancyLimit: int(6)},
	}
	v, ok := metadataInt64(acct, MetaKeyOccupancyLimit)
	if !ok || v != 6 {
		t.Fatalf("expected 6 from int metadata, got ok=%v v=%d", ok, v)
	}
}

// TestMetadataFloat64_Float64: metadataFloat64 正确读取 float64
func TestMetadataFloat64_Float64(t *testing.T) {
	acct := &account.Account{
		Metadata: map[string]any{MetaKeyOccupancyFactor: float64(0.5)},
	}
	v, ok := metadataFloat64(acct, MetaKeyOccupancyFactor)
	if !ok || v != 0.5 {
		t.Fatalf("expected 0.5, got ok=%v v=%f", ok, v)
	}
}

// TestMetadataFloat64_Int64: metadataFloat64 正确读取 int64
func TestMetadataFloat64_Int64(t *testing.T) {
	acct := &account.Account{
		Metadata: map[string]any{MetaKeyOccupancyFactor: int64(2)},
	}
	v, ok := metadataFloat64(acct, MetaKeyOccupancyFactor)
	if !ok || v != 2.0 {
		t.Fatalf("expected 2.0 from int64, got ok=%v v=%f", ok, v)
	}
}

// TestMetadataFloat64_Int: metadataFloat64 正确读取 int
func TestMetadataFloat64_Int(t *testing.T) {
	acct := &account.Account{
		Metadata: map[string]any{MetaKeyOccupancyFactor: int(3)},
	}
	v, ok := metadataFloat64(acct, MetaKeyOccupancyFactor)
	if !ok || v != 3.0 {
		t.Fatalf("expected 3.0 from int, got ok=%v v=%f", ok, v)
	}
}

// TestMetadataFloat64_MissingKey: 缺失 key 返回 false
func TestMetadataFloat64_MissingKey(t *testing.T) {
	acct := &account.Account{
		Metadata: map[string]any{},
	}
	_, ok := metadataFloat64(acct, MetaKeyOccupancyFactor)
	if ok {
		t.Fatal("expected false for missing key")
	}
}

// TestMetadataFloat64_NilMetadata: Metadata 为 nil 返回 false
func TestMetadataFloat64_NilMetadata(t *testing.T) {
	acct := &account.Account{}
	_, ok := metadataFloat64(acct, MetaKeyOccupancyFactor)
	if ok {
		t.Fatal("expected false for nil metadata")
	}
}

// TestMetadataFloat64_WrongType: 类型不匹配返回 false
func TestMetadataFloat64_WrongType(t *testing.T) {
	acct := &account.Account{
		Metadata: map[string]any{MetaKeyOccupancyFactor: "not-a-number"},
	}
	_, ok := metadataFloat64(acct, MetaKeyOccupancyFactor)
	if ok {
		t.Fatal("expected false for wrong type")
	}
}

// ==================== AdaptiveLimit: Metadata 覆盖 factor/minLimit/maxLimit ====================

// TestAdaptiveLimit_GetFactor_FromMetadata: Metadata 覆盖 factor
func TestAdaptiveLimit_GetFactor_FromMetadata(t *testing.T) {
	al := NewAdaptiveLimit(&mockUsageTracker{}, WithFactor(1.0))
	acct := &account.Account{
		Metadata: map[string]any{MetaKeyOccupancyFactor: float64(2.0)},
	}
	if v := al.getFactor(acct); v != 2.0 {
		t.Fatalf("expected factor=2.0 from metadata, got %f", v)
	}
}

// TestAdaptiveLimit_GetMinLimit_FromMetadata: Metadata 覆盖 minLimit
func TestAdaptiveLimit_GetMinLimit_FromMetadata(t *testing.T) {
	al := NewAdaptiveLimit(&mockUsageTracker{}, WithMinLimit(1))
	acct := &account.Account{
		Metadata: map[string]any{MetaKeyOccupancyMinLimit: int64(3)},
	}
	if v := al.getMinLimit(acct); v != 3 {
		t.Fatalf("expected minLimit=3 from metadata, got %d", v)
	}
}

// TestAdaptiveLimit_GetMaxLimit_FromMetadata: Metadata 覆盖 maxLimit
func TestAdaptiveLimit_GetMaxLimit_FromMetadata(t *testing.T) {
	al := NewAdaptiveLimit(&mockUsageTracker{}, WithMaxLimit(0))
	acct := &account.Account{
		Metadata: map[string]any{MetaKeyOccupancyMaxLimit: int64(10)},
	}
	if v := al.getMaxLimit(acct); v != 10 {
		t.Fatalf("expected maxLimit=10 from metadata, got %d", v)
	}
}

// TestAdaptiveLimit_CalculateLimit_LowMinRatio: minRatio < 0.1 时压缩 limit
func TestAdaptiveLimit_CalculateLimit_LowMinRatio(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	innerTracker := usagetracker.NewUsageTracker(usagetracker.WithUsageStore(store))

	// 配额几乎耗尽：RemoteRemain=5, Total=100 → ratio=0.05 < 0.1
	windowEnd := time.Now().Add(time.Hour)
	rules := []*usagerule.UsageRule{
		{
			SourceType:      usagerule.SourceTypeRequest,
			Total:           100,
			TimeGranularity: usagerule.GranularityHour,
			WindowSize:      1,
		},
	}
	_ = innerTracker.InitRules(ctx, "acc-low", rules)
	_ = store.CalibrateRule(ctx, "acc-low", 0, &account.TrackedUsage{
		RemoteUsed:   95,
		RemoteRemain: 5,
		WindowEnd:    &windowEnd,
	})

	al := NewAdaptiveLimit(innerTracker, WithFactor(1.0), WithMinLimit(1))
	acct := &account.Account{ID: "acc-low"}
	limit := al.calculateLimit(ctx, acct)
	// 应该 >= 1（minLimit 保障）
	if limit < 1 {
		t.Fatalf("expected limit >= 1, got %d", limit)
	}
}
