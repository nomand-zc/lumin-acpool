package memory

import (
	"context"
	"testing"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/provider"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-acpool/storage/filtercond"
)

func newTestAccount(id, provType, provName string, status account.Status, priority int) *account.Account {
	return &account.Account{
		ID:           id,
		ProviderType: provType,
		ProviderName: provName,
		Status:       status,
		Priority:     priority,
		Tags:         map[string]string{"env": "test"},
	}
}

func TestAccountStore_AddAndGet(t *testing.T) {
	ctx := context.Background()
	store := NewAccountStore()

	acct := newTestAccount("acc-1", "kiro", "team-a", account.StatusAvailable, 5)

	// 正常添加
	if err := store.Add(ctx, acct); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// 重复添加应返回 ErrAlreadyExists
	if err := store.Add(ctx, acct); err != storage.ErrAlreadyExists {
		t.Fatalf("expected ErrAlreadyExists, got: %v", err)
	}

	// 获取
	got, err := store.Get(ctx, "acc-1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.ID != "acc-1" || got.ProviderType != "kiro" || got.ProviderName != "team-a" {
		t.Fatalf("Get returned wrong data: %+v", got)
	}
	if got.Priority != 5 || got.Status != account.StatusAvailable {
		t.Fatalf("Get returned wrong priority/status: %+v", got)
	}

	// 验证深拷贝：修改返回值不影响存储
	got.Priority = 999
	got2, _ := store.Get(ctx, "acc-1")
	if got2.Priority != 5 {
		t.Fatalf("expected priority 5, got %d (deep copy failed)", got2.Priority)
	}

	// 获取不存在的
	_, err = store.Get(ctx, "nonexistent")
	if err != storage.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestAccountStore_Update(t *testing.T) {
	ctx := context.Background()
	store := NewAccountStore()

	acct := newTestAccount("acc-1", "kiro", "team-a", account.StatusAvailable, 5)
	_ = store.Add(ctx, acct)

	// 更新状态和优先级
	updated := newTestAccount("acc-1", "kiro", "team-a", account.StatusCoolingDown, 10)
	if err := store.Update(ctx, updated); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	got, _ := store.Get(ctx, "acc-1")
	if got.Status != account.StatusCoolingDown || got.Priority != 10 {
		t.Fatalf("Update not persisted: %+v", got)
	}

	// 更新不存在的
	notExist := newTestAccount("nonexistent", "kiro", "team-a", account.StatusAvailable, 0)
	if err := store.Update(ctx, notExist); err != storage.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestAccountStore_UpdateProviderKey(t *testing.T) {
	ctx := context.Background()
	store := NewAccountStore()

	acct := newTestAccount("acc-1", "kiro", "team-a", account.StatusAvailable, 5)
	_ = store.Add(ctx, acct)

	// 修改 ProviderName（从 team-a 迁移到 team-b）
	moved := newTestAccount("acc-1", "kiro", "team-b", account.StatusAvailable, 5)
	_ = store.Update(ctx, moved)

	// team-a 下应该没有账号了
	list, _ := store.Search(ctx, filtercond.And(
		filtercond.Equal("provider_type", "kiro"),
		filtercond.Equal("provider_name", "team-a"),
	))
	if len(list) != 0 {
		t.Fatalf("expected 0 accounts under team-a, got %d", len(list))
	}

	// team-b 下应该有 1 个
	list, _ = store.Search(ctx, filtercond.And(
		filtercond.Equal("provider_type", "kiro"),
		filtercond.Equal("provider_name", "team-b"),
	))
	if len(list) != 1 {
		t.Fatalf("expected 1 account under team-b, got %d", len(list))
	}
}

func TestAccountStore_Remove(t *testing.T) {
	ctx := context.Background()
	store := NewAccountStore()

	acct := newTestAccount("acc-1", "kiro", "team-a", account.StatusAvailable, 5)
	_ = store.Add(ctx, acct)

	if err := store.Remove(ctx, "acc-1"); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	// 确认已删除
	_, err := store.Get(ctx, "acc-1")
	if err != storage.ErrNotFound {
		t.Fatalf("expected ErrNotFound after Remove, got: %v", err)
	}

	// 确认索引已清理
	list, _ := store.Search(ctx, filtercond.And(
		filtercond.Equal("provider_type", "kiro"),
		filtercond.Equal("provider_name", "team-a"),
	))
	if len(list) != 0 {
		t.Fatalf("expected 0 accounts in index after Remove, got %d", len(list))
	}

	// 删除不存在的
	if err := store.Remove(ctx, "nonexistent"); err != storage.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestAccountStore_List(t *testing.T) {
	ctx := context.Background()
	store := NewAccountStore()

	_ = store.Add(ctx, newTestAccount("acc-1", "kiro", "team-a", account.StatusAvailable, 5))
	_ = store.Add(ctx, newTestAccount("acc-2", "kiro", "team-a", account.StatusCoolingDown, 3))
	_ = store.Add(ctx, newTestAccount("acc-3", "kiro", "team-b", account.StatusAvailable, 8))
	_ = store.Add(ctx, newTestAccount("acc-4", "openai", "default", account.StatusBanned, 1))

	// 全量查询
	all, err := store.Search(ctx, nil)
	if err != nil {
		t.Fatalf("List(nil) failed: %v", err)
	}
	if len(all) != 4 {
		t.Fatalf("expected 4, got %d", len(all))
	}

	// 按状态过滤
	available, err := store.Search(ctx, filtercond.Equal("status", int(account.StatusAvailable)))
	if err != nil {
		t.Fatalf("List(status=available) failed: %v", err)
	}
	if len(available) != 2 {
		t.Fatalf("expected 2 available, got %d", len(available))
	}

	// 按优先级过滤
	highPrio, err := store.Search(ctx, filtercond.GreaterThanOrEqual("priority", 5))
	if err != nil {
		t.Fatalf("List(priority>=5) failed: %v", err)
	}
	if len(highPrio) != 2 {
		t.Fatalf("expected 2 with priority>=5, got %d", len(highPrio))
	}

	// 组合条件：可用 + 优先级 >= 5
	combined, err := store.Search(ctx, filtercond.And(
		filtercond.Equal("status", int(account.StatusAvailable)),
		filtercond.GreaterThanOrEqual("priority", 5),
	))
	if err != nil {
		t.Fatalf("List(and) failed: %v", err)
	}
	if len(combined) != 2 {
		t.Fatalf("expected 2, got %d", len(combined))
	}

	// Or 条件
	orResult, err := store.Search(ctx, filtercond.Or(
		filtercond.Equal("provider_name", "team-b"),
		filtercond.Equal("status", int(account.StatusBanned)),
	))
	if err != nil {
		t.Fatalf("List(or) failed: %v", err)
	}
	if len(orResult) != 2 {
		t.Fatalf("expected 2 for Or, got %d", len(orResult))
	}
}



func TestAccountStore_Count(t *testing.T) {
	ctx := context.Background()
	store := NewAccountStore()

	_ = store.Add(ctx, newTestAccount("acc-1", "kiro", "team-a", account.StatusAvailable, 5))
	_ = store.Add(ctx, newTestAccount("acc-2", "kiro", "team-a", account.StatusCoolingDown, 3))
	_ = store.Add(ctx, newTestAccount("acc-3", "kiro", "team-b", account.StatusAvailable, 8))

	// 全量计数
	total, err := store.Count(ctx, nil)
	if err != nil {
		t.Fatalf("Count(nil) failed: %v", err)
	}
	if total != 3 {
		t.Fatalf("expected 3, got %d", total)
	}

	// 按状态计数
	availableCount, err := store.Count(ctx, filtercond.Equal("status", int(account.StatusAvailable)))
	if err != nil {
		t.Fatalf("Count(status=available) failed: %v", err)
	}
	if availableCount != 2 {
		t.Fatalf("expected 2, got %d", availableCount)
	}

	// 按 ProviderKey 计数
	keyCount, err := store.CountByProvider(ctx, provider.ProviderKey{Type: "kiro", Name: "team-a"}, nil)
	if err != nil {
		t.Fatalf("CountByProvider failed: %v", err)
	}
	if keyCount != 2 {
		t.Fatalf("expected 2, got %d", keyCount)
	}

	// 按 ProviderKey + 状态计数
	keyAvail, err := store.CountByProvider(ctx,
		provider.ProviderKey{Type: "kiro", Name: "team-a"},
		filtercond.Equal("status", int(account.StatusAvailable)),
	)
	if err != nil {
		t.Fatalf("CountByProvider(status=available) failed: %v", err)
	}
	if keyAvail != 1 {
		t.Fatalf("expected 1, got %d", keyAvail)
	}
}

func TestAccountStore_TimeAutoSet(t *testing.T) {
	ctx := context.Background()
	store := NewAccountStore()

	before := time.Now()
	acct := newTestAccount("acc-1", "kiro", "team-a", account.StatusAvailable, 5)
	_ = store.Add(ctx, acct)
	after := time.Now()

	got, _ := store.Get(ctx, "acc-1")
	if got.CreatedAt.Before(before) || got.CreatedAt.After(after) {
		t.Fatalf("CreatedAt not auto-set properly: %v", got.CreatedAt)
	}
	if got.UpdatedAt.Before(before) || got.UpdatedAt.After(after) {
		t.Fatalf("UpdatedAt not auto-set properly: %v", got.UpdatedAt)
	}
}

func TestAccountStore_InOperator(t *testing.T) {
	ctx := context.Background()
	store := NewAccountStore()

	_ = store.Add(ctx, newTestAccount("acc-1", "kiro", "team-a", account.StatusAvailable, 5))
	_ = store.Add(ctx, newTestAccount("acc-2", "kiro", "team-a", account.StatusCoolingDown, 3))
	_ = store.Add(ctx, newTestAccount("acc-3", "kiro", "team-b", account.StatusBanned, 1))

	// In 操作符
	result, err := store.Search(ctx, filtercond.In("status", int(account.StatusAvailable), int(account.StatusBanned)))
	if err != nil {
		t.Fatalf("List(In) failed: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2, got %d", len(result))
	}
}

func TestAccountStore_BetweenOperator(t *testing.T) {
	ctx := context.Background()
	store := NewAccountStore()

	_ = store.Add(ctx, newTestAccount("acc-1", "kiro", "team-a", account.StatusAvailable, 1))
	_ = store.Add(ctx, newTestAccount("acc-2", "kiro", "team-a", account.StatusAvailable, 5))
	_ = store.Add(ctx, newTestAccount("acc-3", "kiro", "team-b", account.StatusAvailable, 10))

	// Between 操作符
	result, err := store.Search(ctx, filtercond.Between("priority", 3, 8))
	if err != nil {
		t.Fatalf("List(Between) failed: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 (priority=5), got %d", len(result))
	}
	if result[0].ID != "acc-2" {
		t.Fatalf("expected acc-2, got %s", result[0].ID)
	}
}

func TestAccountStore_LikeOperator(t *testing.T) {
	ctx := context.Background()
	store := NewAccountStore()

	_ = store.Add(ctx, newTestAccount("acc-1", "kiro", "team-alpha", account.StatusAvailable, 5))
	_ = store.Add(ctx, newTestAccount("acc-2", "kiro", "team-beta", account.StatusAvailable, 3))
	_ = store.Add(ctx, newTestAccount("acc-3", "openai", "default", account.StatusAvailable, 8))

	// Like 操作符
	result, err := store.Search(ctx, filtercond.Like("provider_name", "team"))
	if err != nil {
		t.Fatalf("List(Like) failed: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2, got %d", len(result))
	}
}

func TestAccountStore_InvalidField(t *testing.T) {
	ctx := context.Background()
	store := NewAccountStore()

	_, err := store.Search(ctx, filtercond.Equal("nonexistent_field", "value"))
	if err == nil {
		t.Fatal("expected error for invalid field, got nil")
	}
}
