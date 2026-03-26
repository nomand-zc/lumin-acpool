package memory

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
)

// --- Fix-9: RemoveAccounts 清理 stats/usage 和 Provider 计数 ---

func setupProviderAndAccounts(t *testing.T, ctx context.Context, store *Store) {
	t.Helper()
	// 添加 Provider
	if err := store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType: "test",
		ProviderName: "default",
		Status:       account.ProviderStatusActive,
	}); err != nil {
		t.Fatalf("AddProvider failed: %v", err)
	}

	// 添加账号
	for _, id := range []string{"acc-1", "acc-2", "acc-3"} {
		if err := store.AddAccount(ctx, &account.Account{
			ID:           id,
			ProviderType: "test",
			ProviderName: "default",
			Status:       account.StatusAvailable,
		}); err != nil {
			t.Fatalf("AddAccount(%s) failed: %v", id, err)
		}
	}
}

// TestRemoveAccounts_CleansStatsStore 验证 RemoveAccounts 清理 statsStore
func TestRemoveAccounts_CleansStatsStore(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	setupProviderAndAccounts(t, ctx, store)

	// 为每个账号生成统计记录
	for _, id := range []string{"acc-1", "acc-2", "acc-3"} {
		_ = store.IncrSuccess(ctx, id)
		_, _ = store.IncrFailure(ctx, id, "test error")
	}

	// 验证统计记录存在
	stats, _ := store.GetStats(ctx, "acc-1")
	if stats.TotalCalls == 0 {
		t.Fatal("expected stats to exist before removal")
	}

	// 批量删除 test/default 下的所有账号
	if err := store.RemoveAccounts(ctx, &storage.SearchFilter{
		ProviderType: "test",
		ProviderName: "default",
	}); err != nil {
		t.Fatalf("RemoveAccounts failed: %v", err)
	}

	// 验证统计记录被清理（返回零值，不是历史数据）
	for _, id := range []string{"acc-1", "acc-2", "acc-3"} {
		stats, _ := store.GetStats(ctx, id)
		if stats.TotalCalls != 0 {
			t.Fatalf("expected stats to be cleaned for %s after RemoveAccounts, got TotalCalls=%d", id, stats.TotalCalls)
		}
	}
}

// TestRemoveAccounts_CleansUsageStore 验证 RemoveAccounts 清理 usageStore
func TestRemoveAccounts_CleansUsageStore(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	setupProviderAndAccounts(t, ctx, store)

	// 为每个账号保存用量数据
	for _, id := range []string{"acc-1", "acc-2", "acc-3"} {
		usages := []*account.TrackedUsage{
			{RemoteRemain: 100, RemoteUsed: 0},
		}
		if err := store.SaveUsages(ctx, id, usages); err != nil {
			t.Fatalf("SaveUsages(%s) failed: %v", id, err)
		}
	}

	// 验证用量记录存在
	usages, _ := store.GetCurrentUsages(ctx, "acc-1")
	if len(usages) == 0 {
		t.Fatal("expected usages to exist before removal")
	}

	// 批量删除
	if err := store.RemoveAccounts(ctx, &storage.SearchFilter{
		ProviderType: "test",
		ProviderName: "default",
	}); err != nil {
		t.Fatalf("RemoveAccounts failed: %v", err)
	}

	// 验证用量记录被清理
	for _, id := range []string{"acc-1", "acc-2", "acc-3"} {
		usages, _ := store.GetCurrentUsages(ctx, id)
		if len(usages) != 0 {
			t.Fatalf("expected usages to be cleaned for %s after RemoveAccounts, got %d", id, len(usages))
		}
	}
}

// TestRemoveAccounts_UpdatesProviderCounts 验证 RemoveAccounts 更新 Provider 计数
func TestRemoveAccounts_UpdatesProviderCounts(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	setupProviderAndAccounts(t, ctx, store)

	// 验证添加时计数
	prov, _ := store.GetProvider(ctx, account.BuildProviderKey("test", "default"))
	if prov.AccountCount != 3 {
		t.Fatalf("expected AccountCount=3 before removal, got %d", prov.AccountCount)
	}
	if prov.AvailableAccountCount != 3 {
		t.Fatalf("expected AvailableAccountCount=3 before removal, got %d", prov.AvailableAccountCount)
	}

	// 批量删除
	if err := store.RemoveAccounts(ctx, &storage.SearchFilter{
		ProviderType: "test",
		ProviderName: "default",
	}); err != nil {
		t.Fatalf("RemoveAccounts failed: %v", err)
	}

	// 验证计数被更新
	prov, _ = store.GetProvider(ctx, account.BuildProviderKey("test", "default"))
	if prov.AccountCount != 0 {
		t.Fatalf("expected AccountCount=0 after RemoveAccounts, got %d", prov.AccountCount)
	}
	if prov.AvailableAccountCount != 0 {
		t.Fatalf("expected AvailableAccountCount=0 after RemoveAccounts, got %d", prov.AvailableAccountCount)
	}
}

// TestRemoveAccounts_PartialRemoval_UpdatesCountsCorrectly 部分删除时 Provider 计数正确
func TestRemoveAccounts_PartialRemoval_UpdatesCountsCorrectly(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	// 添加 Provider
	_ = store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType: "test",
		ProviderName: "default",
		Status:       account.ProviderStatusActive,
	})

	// 添加 2 个 Available，1 个 CoolingDown
	_ = store.AddAccount(ctx, &account.Account{
		ID:           "acc-available-1",
		ProviderType: "test",
		ProviderName: "default",
		Status:       account.StatusAvailable,
	})
	_ = store.AddAccount(ctx, &account.Account{
		ID:           "acc-available-2",
		ProviderType: "test",
		ProviderName: "default",
		Status:       account.StatusAvailable,
	})
	_ = store.AddAccount(ctx, &account.Account{
		ID:           "acc-cooling",
		ProviderType: "test",
		ProviderName: "default",
		Status:       account.StatusCoolingDown,
	})

	// 只删除 Available 状态的账号
	if err := store.RemoveAccounts(ctx, &storage.SearchFilter{
		ProviderType: "test",
		ProviderName: "default",
		Status:       int(account.StatusAvailable),
	}); err != nil {
		t.Fatalf("RemoveAccounts failed: %v", err)
	}

	// 验证 Provider 计数
	prov, _ := store.GetProvider(ctx, account.BuildProviderKey("test", "default"))
	// 只有 CoolingDown 账号剩余
	if prov.AccountCount != 1 {
		t.Fatalf("expected AccountCount=1 (only CoolingDown remains), got %d", prov.AccountCount)
	}
	if prov.AvailableAccountCount != 0 {
		t.Fatalf("expected AvailableAccountCount=0 after removing Available accounts, got %d", prov.AvailableAccountCount)
	}

	// CoolingDown 账号应该还在
	remaining, _ := store.SearchAccounts(ctx, &storage.SearchFilter{
		ProviderType: "test",
		ProviderName: "default",
	})
	if len(remaining) != 1 {
		t.Fatalf("expected 1 remaining account (CoolingDown), got %d", len(remaining))
	}
	if remaining[0].ID != "acc-cooling" {
		t.Fatalf("expected acc-cooling to remain, got %s", remaining[0].ID)
	}
}

// TestRemoveAccounts_EmptyFilter_RemovesAll nil filter 删除所有账号
func TestRemoveAccounts_EmptyFilter_RemovesAll(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType: "test",
		ProviderName: "default",
		Status:       account.ProviderStatusActive,
	})

	for _, id := range []string{"acc-1", "acc-2"} {
		_ = store.AddAccount(ctx, &account.Account{
			ID:           id,
			ProviderType: "test",
			ProviderName: "default",
			Status:       account.StatusAvailable,
		})
	}

	if err := store.RemoveAccounts(ctx, nil); err != nil {
		t.Fatalf("RemoveAccounts(nil) failed: %v", err)
	}

	remaining, _ := store.SearchAccounts(ctx, nil)
	if len(remaining) != 0 {
		t.Fatalf("expected 0 accounts after RemoveAccounts(nil), got %d", len(remaining))
	}
}

// TestRemoveAccounts_NoMatchingAccounts_NoError 无匹配账号时不报错
func TestRemoveAccounts_NoMatchingAccounts_NoError(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	err := store.RemoveAccounts(ctx, &storage.SearchFilter{
		ProviderType: "nonexistent",
	})
	if err != nil {
		t.Fatalf("expected no error when no accounts match, got %v", err)
	}
}

// TestRemoveAccounts_CleansBothStatsAndUsage 同时清理 stats 和 usage
func TestRemoveAccounts_CleansBothStatsAndUsage(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	setupProviderAndAccounts(t, ctx, store)

	// 同时生成统计和用量数据
	for _, id := range []string{"acc-1", "acc-2", "acc-3"} {
		_ = store.IncrSuccess(ctx, id)
		_ = store.SaveUsages(ctx, id, []*account.TrackedUsage{{RemoteRemain: 50}})
	}

	if err := store.RemoveAccounts(ctx, &storage.SearchFilter{
		ProviderType: "test",
		ProviderName: "default",
	}); err != nil {
		t.Fatalf("RemoveAccounts failed: %v", err)
	}

	// 验证两种数据都被清理
	for _, id := range []string{"acc-1", "acc-2", "acc-3"} {
		stats, _ := store.GetStats(ctx, id)
		if stats.TotalCalls != 0 {
			t.Fatalf("stats not cleaned for %s: TotalCalls=%d", id, stats.TotalCalls)
		}

		usages, _ := store.GetCurrentUsages(ctx, id)
		if len(usages) != 0 {
			t.Fatalf("usages not cleaned for %s: %d entries remain", id, len(usages))
		}
	}
}

// --- SearchAccounts 快路径（索引）测试 ---

// TestSearchAccounts_IndexPath_ExactProvider 验证快路径只返回指定 Provider 下的账号。
func TestSearchAccounts_IndexPath_ExactProvider(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	// 添加两个 Provider
	for _, name := range []string{"team-a", "team-b"} {
		_ = store.AddProvider(ctx, &account.ProviderInfo{
			ProviderType: "openai",
			ProviderName: name,
			Status:       account.ProviderStatusActive,
		})
	}

	// team-a 10 个，team-b 90 个
	for i := range 10 {
		_ = store.AddAccount(ctx, &account.Account{
			ID:           fmt.Sprintf("team-a-%d", i),
			ProviderType: "openai",
			ProviderName: "team-a",
			Status:       account.StatusAvailable,
		})
	}
	for i := range 90 {
		_ = store.AddAccount(ctx, &account.Account{
			ID:           fmt.Sprintf("team-b-%d", i),
			ProviderType: "openai",
			ProviderName: "team-b",
			Status:       account.StatusAvailable,
		})
	}

	result, err := store.SearchAccounts(ctx, &storage.SearchFilter{
		ProviderType: "openai",
		ProviderName: "team-a",
	})
	if err != nil {
		t.Fatalf("SearchAccounts failed: %v", err)
	}
	if len(result) != 10 {
		t.Fatalf("expected 10 accounts, got %d", len(result))
	}
	for _, acct := range result {
		if acct.ProviderType != "openai" || acct.ProviderName != "team-a" {
			t.Fatalf("unexpected account: type=%s name=%s", acct.ProviderType, acct.ProviderName)
		}
	}
}

// TestSearchAccounts_IndexPath_StatusFilter 验证快路径正确过滤 Status。
func TestSearchAccounts_IndexPath_StatusFilter(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType: "openai",
		ProviderName: "team-a",
		Status:       account.ProviderStatusActive,
	})

	// 6 个 Available
	for i := range 6 {
		_ = store.AddAccount(ctx, &account.Account{
			ID:           fmt.Sprintf("avail-%d", i),
			ProviderType: "openai",
			ProviderName: "team-a",
			Status:       account.StatusAvailable,
		})
	}
	// 4 个 CoolingDown
	for i := range 4 {
		_ = store.AddAccount(ctx, &account.Account{
			ID:           fmt.Sprintf("cool-%d", i),
			ProviderType: "openai",
			ProviderName: "team-a",
			Status:       account.StatusCoolingDown,
		})
	}

	result, err := store.SearchAccounts(ctx, &storage.SearchFilter{
		ProviderType: "openai",
		ProviderName: "team-a",
		Status:       int(account.StatusAvailable),
	})
	if err != nil {
		t.Fatalf("SearchAccounts failed: %v", err)
	}
	if len(result) != 6 {
		t.Fatalf("expected 6 available accounts, got %d", len(result))
	}
	for _, acct := range result {
		if acct.Status != account.StatusAvailable {
			t.Fatalf("expected StatusAvailable, got %v", acct.Status)
		}
	}
}

// TestSearchAccounts_IndexPath_EmptyProvider 验证查询不存在的 Provider 返回空切片无错误。
func TestSearchAccounts_IndexPath_EmptyProvider(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	result, err := store.SearchAccounts(ctx, &storage.SearchFilter{
		ProviderType: "nonexistent",
		ProviderName: "ghost",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected empty result, got %d accounts", len(result))
	}
}

// TestSearchAccounts_FallbackPath_TypeOnly 验证只有 ProviderType 时走慢路径，返回所有匹配账号。
func TestSearchAccounts_FallbackPath_TypeOnly(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	for _, name := range []string{"a", "b"} {
		_ = store.AddProvider(ctx, &account.ProviderInfo{
			ProviderType: "openai",
			ProviderName: name,
			Status:       account.ProviderStatusActive,
		})
		for i := range 5 {
			_ = store.AddAccount(ctx, &account.Account{
				ID:           fmt.Sprintf("openai-%s-%d", name, i),
				ProviderType: "openai",
				ProviderName: name,
				Status:       account.StatusAvailable,
			})
		}
	}

	// 只指定 ProviderType，应走慢路径并返回全部 10 个
	result, err := store.SearchAccounts(ctx, &storage.SearchFilter{
		ProviderType: "openai",
	})
	if err != nil {
		t.Fatalf("SearchAccounts failed: %v", err)
	}
	if len(result) != 10 {
		t.Fatalf("expected 10 accounts (fallback path), got %d", len(result))
	}
}

// BenchmarkSearchAccounts_IndexPath 快路径基准测试（1000 账号，目标 Provider 10 个）。
func BenchmarkSearchAccounts_IndexPath(b *testing.B) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType: "openai",
		ProviderName: "target",
		Status:       account.ProviderStatusActive,
	})
	_ = store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType: "openai",
		ProviderName: "other",
		Status:       account.ProviderStatusActive,
	})

	for i := range 10 {
		_ = store.AddAccount(ctx, &account.Account{
			ID:           fmt.Sprintf("target-%d", i),
			ProviderType: "openai",
			ProviderName: "target",
			Status:       account.StatusAvailable,
		})
	}
	for i := range 990 {
		_ = store.AddAccount(ctx, &account.Account{
			ID:           fmt.Sprintf("other-%d", i),
			ProviderType: "openai",
			ProviderName: "other",
			Status:       account.StatusAvailable,
		})
	}

	filter := &storage.SearchFilter{
		ProviderType: "openai",
		ProviderName: "target",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, _ = store.SearchAccounts(ctx, filter)
	}
}

// BenchmarkSearchAccounts_FullScan 慢路径全表扫描基准测试（1000 账号）。
func BenchmarkSearchAccounts_FullScan(b *testing.B) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType: "openai",
		ProviderName: "target",
		Status:       account.ProviderStatusActive,
	})
	_ = store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType: "openai",
		ProviderName: "other",
		Status:       account.ProviderStatusActive,
	})

	for i := range 10 {
		_ = store.AddAccount(ctx, &account.Account{
			ID:           fmt.Sprintf("target-%d", i),
			ProviderType: "openai",
			ProviderName: "target",
			Status:       account.StatusAvailable,
		})
	}
	for i := range 990 {
		_ = store.AddAccount(ctx, &account.Account{
			ID:           fmt.Sprintf("other-%d", i),
			ProviderType: "openai",
			ProviderName: "other",
			Status:       account.StatusAvailable,
		})
	}

	// 只指定 ProviderType，走全表扫描
	filter := &storage.SearchFilter{
		ProviderType: "openai",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, _ = store.SearchAccounts(ctx, filter)
	}
}

// --- P2 性能优化：ShallowClone 数据隔离测试 ---

// TestSearchAccounts_ReturnedAccountsAreIndependent 验证 SearchAccounts 返回的账号是独立拷贝，
// 修改返回值不会影响存储中的原始数据。
func TestSearchAccounts_ReturnedAccountsAreIndependent(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType: "test",
		ProviderName: "prov",
		Status:       account.ProviderStatusActive,
	})
	_ = store.AddAccount(ctx, &account.Account{
		ID:           "iso-1",
		ProviderType: "test",
		ProviderName: "prov",
		Priority:     10,
		Status:       account.StatusAvailable,
	})

	// 第一次 SearchAccounts
	results, err := store.SearchAccounts(ctx, &storage.SearchFilter{
		ProviderType: "test",
		ProviderName: "prov",
	})
	if err != nil || len(results) != 1 {
		t.Fatalf("SearchAccounts failed: err=%v len=%d", err, len(results))
	}

	// 修改返回的账号字段
	results[0].Priority = 999
	results[0].Status = account.StatusCoolingDown

	// 再次 SearchAccounts，验证存储中的原始数据未被修改
	results2, err := store.SearchAccounts(ctx, &storage.SearchFilter{
		ProviderType: "test",
		ProviderName: "prov",
		Status:       int(account.StatusAvailable),
	})
	if err != nil {
		t.Fatalf("second SearchAccounts failed: %v", err)
	}
	if len(results2) != 1 {
		t.Fatalf("expected 1 account (original status preserved), got %d", len(results2))
	}
	if results2[0].Priority != 10 {
		t.Errorf("expected Priority=10 (original), got %d", results2[0].Priority)
	}
}

// TestSearchAccounts_CooldownUntilIsIndependent 验证时间指针独立（ShallowClone 正确处理）
func TestSearchAccounts_CooldownUntilIsIndependent(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	now := time.Now().Add(time.Hour)
	_ = store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType: "test",
		ProviderName: "prov",
		Status:       account.ProviderStatusActive,
	})
	_ = store.AddAccount(ctx, &account.Account{
		ID:            "iso-time",
		ProviderType:  "test",
		ProviderName:  "prov",
		Status:        account.StatusCoolingDown,
		CooldownUntil: &now,
	})

	results, err := store.SearchAccounts(ctx, &storage.SearchFilter{
		ProviderType: "test",
		ProviderName: "prov",
	})
	if err != nil || len(results) != 1 {
		t.Fatalf("SearchAccounts failed: err=%v len=%d", err, len(results))
	}

	// 修改时间指针指向的值
	later := now.Add(24 * time.Hour)
	*results[0].CooldownUntil = later

	// 再次获取，验证存储中时间不受影响
	results2, _ := store.SearchAccounts(ctx, &storage.SearchFilter{
		ProviderType: "test",
		ProviderName: "prov",
	})
	if len(results2) == 0 {
		t.Fatal("expected 1 account")
	}
	if !results2[0].CooldownUntil.Equal(now) {
		t.Errorf("expected original CooldownUntil %v, got %v", now, results2[0].CooldownUntil)
	}
}

// BenchmarkSearchAccounts_ShallowCloneWithTagsMetadata 带 Tags/Metadata 的 ShallowClone 性能基准。
func BenchmarkSearchAccounts_ShallowCloneWithTagsMetadata(b *testing.B) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType: "bench",
		ProviderName: "default",
		Status:       account.ProviderStatusActive,
	})

	for i := range 10 {
		_ = store.AddAccount(ctx, &account.Account{
			ID:           fmt.Sprintf("bench-%d", i),
			ProviderType: "bench",
			ProviderName: "default",
			Status:       account.StatusAvailable,
			Priority:     i,
			Tags:         map[string]string{"env": "prod", "tier": "premium"},
			Metadata:     map[string]any{"region": "us-east-1", "quota": 1000},
		})
	}

	filter := &storage.SearchFilter{
		ProviderType: "bench",
		ProviderName: "default",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, _ = store.SearchAccounts(ctx, filter)
	}
}
