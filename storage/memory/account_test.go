package memory

import (
	"context"
	"testing"

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
