package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-acpool/storage/filtercond"
	"github.com/nomand-zc/lumin-client/credentials"
	"github.com/nomand-zc/lumin-client/usagerule"
)

// ========================
// 测试辅助
// ========================

const testProviderType = "test-sqlite"

// mockCredential 实现 credentials.Credential 接口，用于测试。
type mockCredential struct {
	token string
}

func (m *mockCredential) Clone() credentials.Credential { return &mockCredential{token: m.token} }
func (m *mockCredential) Validate() error               { return nil }
func (m *mockCredential) GetAccessToken() string        { return m.token }
func (m *mockCredential) GetRefreshToken() string       { return "" }
func (m *mockCredential) GetExpiresAt() *time.Time      { return nil }
func (m *mockCredential) IsExpired() bool               { return false }
func (m *mockCredential) GetUserInfo() (credentials.UserInfo, error) {
	return credentials.UserInfo{}, nil
}
func (m *mockCredential) ToMap() map[string]any { return map[string]any{"token": m.token} }

func init() {
	// 注册测试用 credential 工厂
	credentials.Register(testProviderType, func(data []byte) credentials.Credential {
		return &mockCredential{token: "test-token"}
	})
}

// newTestStore 创建使用内存 SQLite 数据库的 Store。
func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		t.Fatalf("failed to enable foreign keys: %v", err)
	}

	providerJSONFields := map[string]bool{
		storage.ProviderFieldSupportedModel: true,
	}
	store := &Store{
		client:            WrapSQLDB(db),
		accountConverter:  NewConditionConverter(accountFieldMapping, nil),
		providerConverter: NewConditionConverter(providerFieldMapping, providerJSONFields),
	}
	if err := store.initDB(); err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return store
}

func newTestProvider(providerName string) *account.ProviderInfo {
	return &account.ProviderInfo{
		ProviderType: testProviderType,
		ProviderName: providerName,
		Status:       account.ProviderStatusActive,
		Priority:     1,
		Weight:       100,
	}
}

func newTestAccount(id, providerName string, status account.Status) *account.Account {
	return &account.Account{
		ID:           id,
		ProviderType: testProviderType,
		ProviderName: providerName,
		Credential:   &mockCredential{token: "tok-" + id},
		Status:       status,
		Priority:     1,
	}
}

// mustAddProvider 添加 Provider，失败时 Fatal。
func mustAddProvider(t *testing.T, s *Store, name string) {
	t.Helper()
	if err := s.AddProvider(context.Background(), newTestProvider(name)); err != nil {
		t.Fatalf("AddProvider(%s): %v", name, err)
	}
}

// mustAddAccount 添加 Account，失败时 Fatal。
func mustAddAccount(t *testing.T, s *Store, id, provName string, status account.Status) {
	t.Helper()
	if err := s.AddAccount(context.Background(), newTestAccount(id, provName, status)); err != nil {
		t.Fatalf("AddAccount(%s): %v", id, err)
	}
}

// ========================
// NewStore 测试
// ========================

func TestNewStore_WithDSN(t *testing.T) {
	store, err := NewStore(WithDSN(":memory:"))
	if err != nil {
		t.Fatalf("NewStore with :memory: failed: %v", err)
	}
	defer store.Close()
}

func TestNewStore_WithInstanceName(t *testing.T) {
	RegisterInstance("test-inst", WithClientBuilderDSN(":memory:"))
	store, err := NewStore(WithInstanceName("test-inst"))
	if err != nil {
		t.Fatalf("NewStore with instance name failed: %v", err)
	}
	defer store.Close()
}

func TestNewStore_MissingDSN(t *testing.T) {
	_, err := NewStore()
	if err == nil {
		t.Error("expected error for missing DSN, got nil")
	}
}

func TestNewStore_UnknownInstance(t *testing.T) {
	_, err := NewStore(WithInstanceName("no-such-instance-xyz"))
	if err == nil {
		t.Error("expected error for unknown instance, got nil")
	}
}

func TestNewStore_SkipInitDB(t *testing.T) {
	store, err := NewStore(WithDSN(":memory:"), WithSkipInitDB(true))
	if err != nil {
		t.Fatalf("NewStore with SkipInitDB failed: %v", err)
	}
	defer store.Close()
	// 跳过建表后，查询应报错
	_, err = store.GetAccount(context.Background(), "any")
	if err == nil {
		t.Fatal("expected error when querying without table initialization")
	}
}

// ========================
// Provider 测试
// ========================

func TestProvider_AddGetRemove(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	info := newTestProvider("prov-a")
	if err := s.AddProvider(ctx, info); err != nil {
		t.Fatalf("AddProvider: %v", err)
	}

	got, err := s.GetProvider(ctx, account.ProviderKey{Type: testProviderType, Name: "prov-a"})
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	if got.ProviderName != "prov-a" {
		t.Errorf("expected prov-a, got %s", got.ProviderName)
	}

	// 重复添加 -> ErrAlreadyExists
	if err := s.AddProvider(ctx, info); !errors.Is(err, storage.ErrAlreadyExists) {
		t.Errorf("expected ErrAlreadyExists, got %v", err)
	}

	if err := s.RemoveProvider(ctx, account.ProviderKey{Type: testProviderType, Name: "prov-a"}); err != nil {
		t.Fatalf("RemoveProvider: %v", err)
	}

	// 删除不存在 -> ErrNotFound
	if err := s.RemoveProvider(ctx, account.ProviderKey{Type: testProviderType, Name: "prov-a"}); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	// Get 不存在 -> ErrNotFound
	_, err = s.GetProvider(ctx, account.ProviderKey{Type: testProviderType, Name: "prov-a"})
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestProvider_UpdateAndSearch(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	mustAddProvider(t, s, "prov-b")

	got, _ := s.GetProvider(ctx, account.ProviderKey{Type: testProviderType, Name: "prov-b"})
	got.Priority = 99
	got.Weight = 50
	got.Status = account.ProviderStatusDisabled

	if err := s.UpdateProvider(ctx, got); err != nil {
		t.Fatalf("UpdateProvider: %v", err)
	}

	updated, _ := s.GetProvider(ctx, account.ProviderKey{Type: testProviderType, Name: "prov-b"})
	if updated.Priority != 99 || updated.Weight != 50 {
		t.Errorf("update not persisted: priority=%d weight=%d", updated.Priority, updated.Weight)
	}
	if updated.Status != account.ProviderStatusDisabled {
		t.Errorf("expected disabled, got %v", updated.Status)
	}

	// UpdateProvider 不存在 -> ErrNotFound
	if err := s.UpdateProvider(ctx, newTestProvider("no-such")); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("expected ErrNotFound for missing provider, got %v", err)
	}

	// SearchProviders by type
	list, err := s.SearchProviders(ctx, &storage.SearchFilter{ProviderType: testProviderType})
	if err != nil {
		t.Fatalf("SearchProviders: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 provider, got %d", len(list))
	}

	// Search with nil filter
	list2, err := s.SearchProviders(ctx, nil)
	if err != nil {
		t.Fatalf("SearchProviders(nil): %v", err)
	}
	if len(list2) != 1 {
		t.Errorf("expected 1 provider (nil filter), got %d", len(list2))
	}
}

func TestProvider_SearchByModel(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p1 := newTestProvider("prov-model-a")
	p1.SupportedModels = []string{"model-x", "model-y"}
	_ = s.AddProvider(ctx, p1)

	p2 := newTestProvider("prov-model-b")
	p2.SupportedModels = []string{"model-z"}
	_ = s.AddProvider(ctx, p2)

	results, err := s.SearchProviders(ctx, &storage.SearchFilter{SupportedModel: "model-x"})
	if err != nil {
		t.Fatalf("SearchProviders by model: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ProviderName != "prov-model-a" {
		t.Errorf("expected prov-model-a, got %s", results[0].ProviderName)
	}
}

// ========================
// Account 测试
// ========================

func TestAccount_AddGetRemove(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	mustAddProvider(t, s, "prov-c")

	acct := newTestAccount("acc-1", "prov-c", account.StatusAvailable)
	if err := s.AddAccount(ctx, acct); err != nil {
		t.Fatalf("AddAccount: %v", err)
	}

	got, err := s.GetAccount(ctx, "acc-1")
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if got.ID != "acc-1" || got.Status != account.StatusAvailable {
		t.Errorf("unexpected account: %+v", got)
	}

	// 重复添加 -> ErrAlreadyExists
	if err := s.AddAccount(ctx, acct); !errors.Is(err, storage.ErrAlreadyExists) {
		t.Errorf("expected ErrAlreadyExists, got %v", err)
	}

	if err := s.RemoveAccount(ctx, "acc-1"); err != nil {
		t.Fatalf("RemoveAccount: %v", err)
	}

	// 删除不存在 -> ErrNotFound
	if err := s.RemoveAccount(ctx, "acc-1"); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	// Get 不存在 -> ErrNotFound
	if _, err := s.GetAccount(ctx, "acc-1"); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestAccount_UpdateStatus(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	mustAddProvider(t, s, "prov-d")
	mustAddAccount(t, s, "acc-upd", "prov-d", account.StatusAvailable)

	got, _ := s.GetAccount(ctx, "acc-upd")
	got.Status = account.StatusDisabled
	if err := s.UpdateAccount(ctx, got, storage.UpdateFieldStatus); err != nil {
		t.Fatalf("UpdateAccount(status): %v", err)
	}

	updated, _ := s.GetAccount(ctx, "acc-upd")
	if updated.Status != account.StatusDisabled {
		t.Errorf("expected disabled, got %v", updated.Status)
	}
}

func TestAccount_UpdatePriority(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	mustAddProvider(t, s, "prov-prio")
	mustAddAccount(t, s, "acc-prio", "prov-prio", account.StatusAvailable)

	got, _ := s.GetAccount(ctx, "acc-prio")
	got.Priority = 10
	if err := s.UpdateAccount(ctx, got, storage.UpdateFieldPriority); err != nil {
		t.Fatalf("UpdateAccount(priority): %v", err)
	}

	updated, _ := s.GetAccount(ctx, "acc-prio")
	if updated.Priority != 10 {
		t.Errorf("expected priority=10, got %d", updated.Priority)
	}
}

func TestAccount_UpdateVersionConflict(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	mustAddProvider(t, s, "prov-vc")
	mustAddAccount(t, s, "acc-vc", "prov-vc", account.StatusAvailable)

	got, _ := s.GetAccount(ctx, "acc-vc")
	// 使用过期的版本号触发冲突
	got.Version = 999
	got.Status = account.StatusDisabled
	if err := s.UpdateAccount(ctx, got, storage.UpdateFieldStatus); !errors.Is(err, storage.ErrVersionConflict) {
		t.Errorf("expected ErrVersionConflict, got %v", err)
	}
}

func TestAccount_UpdateNoFields(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	mustAddProvider(t, s, "prov-nf")
	mustAddAccount(t, s, "acc-nf", "prov-nf", account.StatusAvailable)

	got, _ := s.GetAccount(ctx, "acc-nf")
	if err := s.UpdateAccount(ctx, got, 0); err == nil {
		t.Error("expected error for no update fields, got nil")
	}
}

func TestAccount_UpdateTagsAndMetadata(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	mustAddProvider(t, s, "prov-tags")
	mustAddAccount(t, s, "acc-tags", "prov-tags", account.StatusAvailable)

	got, _ := s.GetAccount(ctx, "acc-tags")
	got.Tags = map[string]string{"env": "prod"}
	got.Metadata = map[string]any{"region": "us-east"}

	if err := s.UpdateAccount(ctx, got, storage.UpdateFieldTags|storage.UpdateFieldMetadata); err != nil {
		t.Fatalf("UpdateAccount(tags+metadata): %v", err)
	}

	updated, _ := s.GetAccount(ctx, "acc-tags")
	if updated.Tags["env"] != "prod" {
		t.Errorf("tags not persisted: %v", updated.Tags)
	}
	if updated.Metadata["region"] != "us-east" {
		t.Errorf("metadata not persisted: %v", updated.Metadata)
	}
}

func TestAccount_SearchByStatus(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	mustAddProvider(t, s, "prov-srch")
	mustAddAccount(t, s, "acc-avail", "prov-srch", account.StatusAvailable)
	mustAddAccount(t, s, "acc-cool", "prov-srch", account.StatusCoolingDown)

	results, err := s.SearchAccounts(ctx, &storage.SearchFilter{
		ProviderType: testProviderType,
		ProviderName: "prov-srch",
		Status:       int(account.StatusAvailable),
	})
	if err != nil {
		t.Fatalf("SearchAccounts: %v", err)
	}
	if len(results) != 1 || results[0].ID != "acc-avail" {
		t.Errorf("expected 1 available account, got %d", len(results))
	}
}

func TestAccount_SearchByProviderType(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	mustAddProvider(t, s, "prov-type-a")
	mustAddProvider(t, s, "prov-type-b")
	mustAddAccount(t, s, "acc-a1", "prov-type-a", account.StatusAvailable)
	mustAddAccount(t, s, "acc-a2", "prov-type-a", account.StatusAvailable)
	mustAddAccount(t, s, "acc-b1", "prov-type-b", account.StatusAvailable)

	results, err := s.SearchAccounts(ctx, &storage.SearchFilter{
		ProviderType: testProviderType,
		ProviderName: "prov-type-a",
	})
	if err != nil {
		t.Fatalf("SearchAccounts: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 accounts, got %d", len(results))
	}
}

func TestAccount_RemoveAccounts(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	mustAddProvider(t, s, "prov-e")
	mustAddAccount(t, s, "acc-e1", "prov-e", account.StatusAvailable)
	mustAddAccount(t, s, "acc-e2", "prov-e", account.StatusAvailable)

	if err := s.RemoveAccounts(ctx, &storage.SearchFilter{
		ProviderType: testProviderType,
		ProviderName: "prov-e",
	}); err != nil {
		t.Fatalf("RemoveAccounts: %v", err)
	}

	count, _ := s.CountAccounts(ctx, &storage.SearchFilter{ProviderType: testProviderType})
	if count != 0 {
		t.Errorf("expected 0 accounts after RemoveAccounts, got %d", count)
	}
}

func TestAccount_RemoveAccounts_NilFilter(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	mustAddProvider(t, s, "prov-nil")
	mustAddAccount(t, s, "acc-nil1", "prov-nil", account.StatusAvailable)

	if err := s.RemoveAccounts(ctx, nil); err != nil {
		t.Fatalf("RemoveAccounts(nil): %v", err)
	}

	count, _ := s.CountAccounts(ctx, nil)
	if count != 0 {
		t.Errorf("expected 0 accounts, got %d", count)
	}
}

func TestAccount_CountAccounts(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	mustAddProvider(t, s, "prov-cnt")
	mustAddAccount(t, s, "cnt-1", "prov-cnt", account.StatusAvailable)
	mustAddAccount(t, s, "cnt-2", "prov-cnt", account.StatusAvailable)
	mustAddAccount(t, s, "cnt-3", "prov-cnt", account.StatusDisabled)

	count, err := s.CountAccounts(ctx, &storage.SearchFilter{
		ProviderType: testProviderType,
		ProviderName: "prov-cnt",
	})
	if err != nil {
		t.Fatalf("CountAccounts: %v", err)
	}
	if count != 3 {
		t.Errorf("expected count=3, got %d", count)
	}
}

func TestAccount_AddAccount_UpdatesProviderCount(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	mustAddProvider(t, s, "prov-count")
	mustAddAccount(t, s, "c1", "prov-count", account.StatusAvailable)
	mustAddAccount(t, s, "c2", "prov-count", account.StatusAvailable)
	mustAddAccount(t, s, "c3", "prov-count", account.StatusCoolingDown)

	info, err := s.GetProvider(ctx, account.BuildProviderKey(testProviderType, "prov-count"))
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	if info.AccountCount != 3 {
		t.Errorf("expected AccountCount=3, got %d", info.AccountCount)
	}
	if info.AvailableAccountCount != 2 {
		t.Errorf("expected AvailableAccountCount=2, got %d", info.AvailableAccountCount)
	}
}

func TestAccount_UpdateStatus_UpdatesProviderCount(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	mustAddProvider(t, s, "prov-sc")
	mustAddAccount(t, s, "sc1", "prov-sc", account.StatusAvailable)

	acct, _ := s.GetAccount(ctx, "sc1")
	acct.Status = account.StatusDisabled
	_ = s.UpdateAccount(ctx, acct, storage.UpdateFieldStatus)

	info, _ := s.GetProvider(ctx, account.BuildProviderKey(testProviderType, "prov-sc"))
	if info.AvailableAccountCount != 0 {
		t.Errorf("expected AvailableAccountCount=0 after disable, got %d", info.AvailableAccountCount)
	}
}

func TestAccount_RemoveSingle_UpdatesProviderCount(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	mustAddProvider(t, s, "prov-del")
	mustAddAccount(t, s, "del1", "prov-del", account.StatusAvailable)

	_ = s.RemoveAccount(ctx, "del1")

	info, _ := s.GetProvider(ctx, account.BuildProviderKey(testProviderType, "prov-del"))
	if info.AccountCount != 0 {
		t.Errorf("expected AccountCount=0 after remove, got %d", info.AccountCount)
	}
}

// ========================
// Stats 测试
// ========================

func TestStats_GetEmpty(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	st, err := s.GetStats(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("GetStats(nonexistent): %v", err)
	}
	if st.TotalCalls != 0 {
		t.Error("expected zero stats for nonexistent account")
	}
}

func TestStats_IncrSuccessAndFailure(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	mustAddProvider(t, s, "prov-stats")
	mustAddAccount(t, s, "acc-s1", "prov-stats", account.StatusAvailable)

	_ = s.IncrSuccess(ctx, "acc-s1")
	_ = s.IncrSuccess(ctx, "acc-s1")

	n, err := s.IncrFailure(ctx, "acc-s1", "timeout")
	if err != nil {
		t.Fatalf("IncrFailure: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 consecutive failure, got %d", n)
	}

	st, err := s.GetStats(ctx, "acc-s1")
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if st.TotalCalls != 3 || st.SuccessCalls != 2 || st.FailedCalls != 1 {
		t.Errorf("unexpected stats: %+v", st)
	}
}

func TestStats_IncrSuccess_ResetsConsecutiveFailures(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	mustAddProvider(t, s, "prov-reset")
	mustAddAccount(t, s, "acc-reset", "prov-reset", account.StatusAvailable)

	_, _ = s.IncrFailure(ctx, "acc-reset", "err")
	_, _ = s.IncrFailure(ctx, "acc-reset", "err")
	_ = s.IncrSuccess(ctx, "acc-reset")

	st, _ := s.GetStats(ctx, "acc-reset")
	if st.ConsecutiveFailures != 0 {
		t.Errorf("expected ConsecutiveFailures=0 after success, got %d", st.ConsecutiveFailures)
	}
}

func TestStats_GetConsecutiveFailures(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	// 无记录时返回 0
	n, err := s.GetConsecutiveFailures(ctx, "no-such")
	if err != nil {
		t.Fatalf("GetConsecutiveFailures: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0, got %d", n)
	}

	mustAddProvider(t, s, "prov-cf")
	mustAddAccount(t, s, "acc-f1", "prov-cf", account.StatusAvailable)

	for i := 0; i < 3; i++ {
		if _, err := s.IncrFailure(ctx, "acc-f1", "err"); err != nil {
			t.Fatalf("IncrFailure: %v", err)
		}
	}

	n, _ = s.GetConsecutiveFailures(ctx, "acc-f1")
	if n != 3 {
		t.Errorf("expected 3, got %d", n)
	}
}

func TestStats_ResetConsecutiveFailures(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	mustAddProvider(t, s, "prov-rf")
	mustAddAccount(t, s, "acc-rf", "prov-rf", account.StatusAvailable)

	_, _ = s.IncrFailure(ctx, "acc-rf", "err")
	_, _ = s.IncrFailure(ctx, "acc-rf", "err")

	if err := s.ResetConsecutiveFailures(ctx, "acc-rf"); err != nil {
		t.Fatalf("ResetConsecutiveFailures: %v", err)
	}

	n, _ := s.GetConsecutiveFailures(ctx, "acc-rf")
	if n != 0 {
		t.Errorf("expected 0 after reset, got %d", n)
	}
}

func TestStats_UpdateLastUsed(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	mustAddProvider(t, s, "prov-lu")
	mustAddAccount(t, s, "acc-lu", "prov-lu", account.StatusAvailable)

	now := time.Now().Truncate(time.Millisecond)
	if err := s.UpdateLastUsed(ctx, "acc-lu", now); err != nil {
		t.Fatalf("UpdateLastUsed: %v", err)
	}

	st, err := s.GetStats(ctx, "acc-lu")
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if st.LastUsedAt == nil {
		t.Fatal("expected LastUsedAt to be set")
	}
}

func TestStats_RemoveStats(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	mustAddProvider(t, s, "prov-rm")
	mustAddAccount(t, s, "acc-rm", "prov-rm", account.StatusAvailable)

	_ = s.IncrSuccess(ctx, "acc-rm")
	if err := s.RemoveStats(ctx, "acc-rm"); err != nil {
		t.Fatalf("RemoveStats: %v", err)
	}

	st, _ := s.GetStats(ctx, "acc-rm")
	if st.TotalCalls != 0 {
		t.Errorf("expected stats cleared, got TotalCalls=%d", st.TotalCalls)
	}
}

// ========================
// Occupancy 测试
// ========================

func TestOccupancy_IncrDecrGet(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	mustAddProvider(t, s, "prov-occ")
	mustAddAccount(t, s, "acc-occ", "prov-occ", account.StatusAvailable)

	// 未记录时返回 0
	n, err := s.GetOccupancy(ctx, "acc-occ")
	if err != nil {
		t.Fatalf("GetOccupancy: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0, got %d", n)
	}

	cnt, err := s.IncrOccupancy(ctx, "acc-occ")
	if err != nil {
		t.Fatalf("IncrOccupancy: %v", err)
	}
	if cnt != 1 {
		t.Errorf("expected 1, got %d", cnt)
	}

	cnt, _ = s.IncrOccupancy(ctx, "acc-occ")
	if cnt != 2 {
		t.Errorf("expected 2, got %d", cnt)
	}

	if err := s.DecrOccupancy(ctx, "acc-occ"); err != nil {
		t.Fatalf("DecrOccupancy: %v", err)
	}

	n, _ = s.GetOccupancy(ctx, "acc-occ")
	if n != 1 {
		t.Errorf("expected 1 after decr, got %d", n)
	}
}

func TestOccupancy_GetOccupancy_NotFound(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	n, err := s.GetOccupancy(ctx, "nonexistent-occ")
	if err != nil {
		t.Fatalf("GetOccupancy(nonexistent): %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0, got %d", n)
	}
}

func TestOccupancy_GetOccupancies(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	// 空 IDs
	m, err := s.GetOccupancies(ctx, nil)
	if err != nil {
		t.Fatalf("GetOccupancies(nil): %v", err)
	}
	if len(m) != 0 {
		t.Errorf("expected empty map, got %v", m)
	}

	mustAddProvider(t, s, "prov-occ2")
	mustAddAccount(t, s, "occ-a", "prov-occ2", account.StatusAvailable)
	mustAddAccount(t, s, "occ-b", "prov-occ2", account.StatusAvailable)

	_, _ = s.IncrOccupancy(ctx, "occ-a")
	_, _ = s.IncrOccupancy(ctx, "occ-a")
	_, _ = s.IncrOccupancy(ctx, "occ-b")

	m, err = s.GetOccupancies(ctx, []string{"occ-a", "occ-b", "occ-c"})
	if err != nil {
		t.Fatalf("GetOccupancies: %v", err)
	}
	if m["occ-a"] != 2 {
		t.Errorf("expected occ-a=2, got %d", m["occ-a"])
	}
	if m["occ-b"] != 1 {
		t.Errorf("expected occ-b=1, got %d", m["occ-b"])
	}
	// occ-c 不存在，不应该出现在结果中
	if _, ok := m["occ-c"]; ok {
		t.Errorf("occ-c should not be in result")
	}
}

// ========================
// Usage 测试
// ========================

func TestUsage_GetEmpty(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	usages, err := s.GetCurrentUsages(ctx, "acc-u-none")
	if err != nil {
		t.Fatalf("GetCurrentUsages: %v", err)
	}
	if len(usages) != 0 {
		t.Errorf("expected empty usages, got %d", len(usages))
	}
}

func TestUsage_SaveAndGet(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	mustAddProvider(t, s, "prov-usg")
	mustAddAccount(t, s, "acc-u1", "prov-usg", account.StatusAvailable)

	now := time.Now()
	windowEnd := now.Add(time.Hour)
	testUsage := &account.TrackedUsage{
		Rule: &usagerule.UsageRule{
			SourceType:      usagerule.SourceTypeRequest,
			TimeGranularity: usagerule.GranularityHour,
			WindowSize:      1,
			Total:           1000,
		},
		LocalUsed:    10,
		RemoteUsed:   50,
		RemoteRemain: 940,
		WindowEnd:    &windowEnd,
		LastSyncAt:   now,
	}

	if err := s.SaveUsages(ctx, "acc-u1", []*account.TrackedUsage{testUsage}); err != nil {
		t.Fatalf("SaveUsages: %v", err)
	}

	usages, err := s.GetCurrentUsages(ctx, "acc-u1")
	if err != nil {
		t.Fatalf("GetCurrentUsages after save: %v", err)
	}
	if len(usages) != 1 {
		t.Fatalf("expected 1 usage, got %d", len(usages))
	}
	if usages[0].RemoteUsed != 50 {
		t.Errorf("expected RemoteUsed=50, got %v", usages[0].RemoteUsed)
	}
	if usages[0].RemoteRemain != 940 {
		t.Errorf("expected RemoteRemain=940, got %v", usages[0].RemoteRemain)
	}
}

func TestUsage_SaveEmpty(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	mustAddProvider(t, s, "prov-empty")
	mustAddAccount(t, s, "acc-empty", "prov-empty", account.StatusAvailable)

	if err := s.SaveUsages(ctx, "acc-empty", []*account.TrackedUsage{}); err != nil {
		t.Fatalf("SaveUsages(empty): %v", err)
	}
}

func TestUsage_SaveOverwrites(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	mustAddProvider(t, s, "prov-ow")
	mustAddAccount(t, s, "acc-ow", "prov-ow", account.StatusAvailable)

	now := time.Now()
	windowEnd := now.Add(time.Hour)
	_ = s.SaveUsages(ctx, "acc-ow", []*account.TrackedUsage{
		{
			Rule:       &usagerule.UsageRule{SourceType: usagerule.SourceTypeRequest, TimeGranularity: usagerule.GranularityHour, WindowSize: 1, Total: 100},
			WindowEnd:  &windowEnd,
			LastSyncAt: now,
		},
	})

	// 覆盖：2条新记录
	_ = s.SaveUsages(ctx, "acc-ow", []*account.TrackedUsage{
		{
			Rule:       &usagerule.UsageRule{SourceType: usagerule.SourceTypeRequest, TimeGranularity: usagerule.GranularityHour, WindowSize: 1, Total: 100},
			WindowEnd:  &windowEnd,
			LastSyncAt: now,
		},
		{
			Rule:       &usagerule.UsageRule{SourceType: usagerule.SourceTypeToken, TimeGranularity: usagerule.GranularityDay, WindowSize: 1, Total: 5000},
			WindowEnd:  &windowEnd,
			LastSyncAt: now,
		},
	})

	result, _ := s.GetCurrentUsages(ctx, "acc-ow")
	if len(result) != 2 {
		t.Fatalf("expected 2 usages after overwrite, got %d", len(result))
	}
}

func TestUsage_IncrLocalUsed(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	mustAddProvider(t, s, "prov-incr")
	mustAddAccount(t, s, "acc-incr", "prov-incr", account.StatusAvailable)

	now := time.Now()
	windowEnd := now.Add(time.Hour)
	_ = s.SaveUsages(ctx, "acc-incr", []*account.TrackedUsage{
		{
			Rule:       &usagerule.UsageRule{SourceType: usagerule.SourceTypeRequest, TimeGranularity: usagerule.GranularityHour, WindowSize: 1, Total: 100},
			LocalUsed:  0,
			WindowEnd:  &windowEnd,
			LastSyncAt: now,
		},
	})

	if err := s.IncrLocalUsed(ctx, "acc-incr", 0, 5.0); err != nil {
		t.Fatalf("IncrLocalUsed: %v", err)
	}

	result, _ := s.GetCurrentUsages(ctx, "acc-incr")
	if len(result) != 1 {
		t.Fatalf("expected 1 usage, got %d", len(result))
	}
	if result[0].LocalUsed != 5.0 {
		t.Errorf("expected LocalUsed=5.0, got %v", result[0].LocalUsed)
	}
}

func TestUsage_RemoveUsages(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	mustAddProvider(t, s, "prov-rem")
	mustAddAccount(t, s, "acc-rem", "prov-rem", account.StatusAvailable)

	now := time.Now()
	windowEnd := now.Add(time.Hour)
	_ = s.SaveUsages(ctx, "acc-rem", []*account.TrackedUsage{
		{
			Rule:       &usagerule.UsageRule{SourceType: usagerule.SourceTypeRequest, TimeGranularity: usagerule.GranularityHour, WindowSize: 1, Total: 100},
			WindowEnd:  &windowEnd,
			LastSyncAt: now,
		},
	})

	if err := s.RemoveUsages(ctx, "acc-rem"); err != nil {
		t.Fatalf("RemoveUsages: %v", err)
	}

	result, _ := s.GetCurrentUsages(ctx, "acc-rem")
	if len(result) != 0 {
		t.Fatalf("expected 0 usages after remove, got %d", len(result))
	}
}

func TestUsage_CalibrateRule(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	mustAddProvider(t, s, "prov-cal")
	mustAddAccount(t, s, "acc-cal", "prov-cal", account.StatusAvailable)

	now := time.Now()
	windowEnd := now.Add(time.Hour)
	_ = s.SaveUsages(ctx, "acc-cal", []*account.TrackedUsage{
		{
			Rule:         &usagerule.UsageRule{SourceType: usagerule.SourceTypeRequest, TimeGranularity: usagerule.GranularityHour, WindowSize: 1, Total: 1000},
			LocalUsed:    20,
			RemoteUsed:   100,
			RemoteRemain: 880,
			WindowEnd:    &windowEnd,
			LastSyncAt:   now,
		},
	})

	newWindowEnd := now.Add(2 * time.Hour)
	if err := s.CalibrateRule(ctx, "acc-cal", 0, &account.TrackedUsage{
		RemoteUsed:   200,
		RemoteRemain: 800,
		WindowEnd:    &newWindowEnd,
	}); err != nil {
		t.Fatalf("CalibrateRule: %v", err)
	}

	result, _ := s.GetCurrentUsages(ctx, "acc-cal")
	if len(result) != 1 {
		t.Fatalf("expected 1 usage after calibrate, got %d", len(result))
	}
	if result[0].RemoteUsed != 200 {
		t.Errorf("expected RemoteUsed=200, got %v", result[0].RemoteUsed)
	}
	if result[0].LocalUsed != 0 {
		t.Errorf("expected LocalUsed=0 after calibrate, got %v", result[0].LocalUsed)
	}
}

// ========================
// Affinity 测试
// ========================

func TestAffinity_SetGet(t *testing.T) {
	s := newTestStore(t)

	// 不存在时返回 false
	_, ok := s.GetAffinity("key-1")
	if ok {
		t.Error("expected false for nonexistent affinity")
	}

	s.SetAffinity("key-1", "target-a")

	val, ok := s.GetAffinity("key-1")
	if !ok {
		t.Error("expected true after SetAffinity")
	}
	if val != "target-a" {
		t.Errorf("expected target-a, got %s", val)
	}
}

func TestAffinity_Overwrite(t *testing.T) {
	s := newTestStore(t)

	s.SetAffinity("key-ow", "target-old")
	s.SetAffinity("key-ow", "target-new")

	val, ok := s.GetAffinity("key-ow")
	if !ok {
		t.Error("expected affinity to exist after overwrite")
	}
	if val != "target-new" {
		t.Errorf("expected target-new, got %s", val)
	}
}

func TestAffinity_GetNotFound(t *testing.T) {
	s := newTestStore(t)

	_, ok := s.GetAffinity("nonexistent")
	if ok {
		t.Error("expected false for nonexistent affinity")
	}
}

// ========================
// MarshalJSON / IsDuplicateEntry 单元测试
// ========================

func TestMarshalJSON_Nil(t *testing.T) {
	result, err := MarshalJSON(nil)
	if err != nil {
		t.Fatalf("MarshalJSON(nil) failed: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil for nil input, got %v", result)
	}
}

func TestMarshalJSON_Map(t *testing.T) {
	m := map[string]string{"key": "value"}
	result, err := MarshalJSON(m)
	if err != nil {
		t.Fatalf("MarshalJSON(map) failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if *result != `{"key":"value"}` {
		t.Errorf("expected {\"key\":\"value\"}, got %s", *result)
	}
}

func TestIsDuplicateEntry_Nil(t *testing.T) {
	if IsDuplicateEntry(nil) {
		t.Error("IsDuplicateEntry(nil) should be false")
	}
}

func TestIsDuplicateEntry_UniqueConstraint(t *testing.T) {
	err := errors.New("UNIQUE constraint failed: accounts.id")
	if !IsDuplicateEntry(err) {
		t.Error("expected true for UNIQUE constraint error")
	}
}

func TestIsDuplicateEntry_OtherError(t *testing.T) {
	if IsDuplicateEntry(errors.New("some other error")) {
		t.Error("expected false for unrelated error")
	}
}

// ========================
// parseTime 测试
// ========================

func TestParseTime_Formats(t *testing.T) {
	cases := []string{
		"2026-01-15 10:30:45.000",
		"2026-01-15 10:30:45",
		"2026-01-15T10:30:45.000Z",
		"2026-01-15T10:30:45Z",
	}
	for _, tc := range cases {
		t.Run(tc, func(t *testing.T) {
			_, err := parseTime(tc)
			if err != nil {
				t.Errorf("parseTime(%q) failed: %v", tc, err)
			}
		})
	}
}

func TestParseTime_Invalid(t *testing.T) {
	_, err := parseTime("not-a-time")
	if err == nil {
		t.Error("expected error for invalid time string")
	}
}

// ========================
// ConditionConverter 直接测试
// ========================

func TestConditionConverter_NilFilter(t *testing.T) {
	c := NewConditionConverter(nil, nil)
	result, err := c.Convert(nil)
	if err != nil {
		t.Fatalf("Convert(nil): %v", err)
	}
	if result.Cond != "1=1" {
		t.Errorf("expected '1=1', got %q", result.Cond)
	}
}

func TestConditionConverter_Equal(t *testing.T) {
	c := NewConditionConverter(nil, nil)
	f := &filtercond.Filter{Field: "status", Operator: filtercond.OperatorEqual, Value: 1}
	result, err := c.Convert(f)
	if err != nil {
		t.Fatalf("Convert(eq): %v", err)
	}
	if result.Cond == "" {
		t.Error("expected non-empty cond for eq operator")
	}
	if len(result.Args) != 1 {
		t.Errorf("expected 1 arg, got %d", len(result.Args))
	}
}

func TestConditionConverter_NotEqual(t *testing.T) {
	c := NewConditionConverter(nil, nil)
	f := &filtercond.Filter{Field: "status", Operator: filtercond.OperatorNotEqual, Value: 2}
	result, err := c.Convert(f)
	if err != nil {
		t.Fatalf("Convert(ne): %v", err)
	}
	if result.Cond == "" {
		t.Error("expected non-empty cond")
	}
}

func TestConditionConverter_ComparisonOps(t *testing.T) {
	c := NewConditionConverter(nil, nil)
	ops := []string{
		filtercond.OperatorGreaterThan,
		filtercond.OperatorGreaterThanOrEqual,
		filtercond.OperatorLessThan,
		filtercond.OperatorLessThanOrEqual,
	}
	for _, op := range ops {
		t.Run(op, func(t *testing.T) {
			f := &filtercond.Filter{Field: "priority", Operator: op, Value: 5}
			result, err := c.Convert(f)
			if err != nil {
				t.Fatalf("Convert(%s): %v", op, err)
			}
			if result.Cond == "" {
				t.Errorf("empty cond for op %s", op)
			}
		})
	}
}

func TestConditionConverter_In(t *testing.T) {
	c := NewConditionConverter(nil, nil)
	f := filtercond.In("status", 1, 2, 3)
	result, err := c.Convert(f)
	if err != nil {
		t.Fatalf("Convert(in): %v", err)
	}
	if len(result.Args) != 3 {
		t.Errorf("expected 3 args, got %d", len(result.Args))
	}
}

func TestConditionConverter_NotIn(t *testing.T) {
	c := NewConditionConverter(nil, nil)
	f := filtercond.NotIn("status", 3, 4)
	result, err := c.Convert(f)
	if err != nil {
		t.Fatalf("Convert(not in): %v", err)
	}
	if len(result.Args) != 2 {
		t.Errorf("expected 2 args, got %d", len(result.Args))
	}
}

func TestConditionConverter_Like(t *testing.T) {
	c := NewConditionConverter(nil, nil)
	f := filtercond.Like("name", "%foo%")
	result, err := c.Convert(f)
	if err != nil {
		t.Fatalf("Convert(like): %v", err)
	}
	if result.Cond == "" {
		t.Error("expected non-empty cond for like")
	}
}

func TestConditionConverter_NotLike(t *testing.T) {
	c := NewConditionConverter(nil, nil)
	f := filtercond.NotLike("name", "%bar%")
	result, err := c.Convert(f)
	if err != nil {
		t.Fatalf("Convert(not like): %v", err)
	}
	if result.Cond == "" {
		t.Error("expected non-empty cond for not like")
	}
}

func TestConditionConverter_Between(t *testing.T) {
	c := NewConditionConverter(nil, nil)
	f := filtercond.Between("priority", 1, 10)
	result, err := c.Convert(f)
	if err != nil {
		t.Fatalf("Convert(between): %v", err)
	}
	if len(result.Args) != 2 {
		t.Errorf("expected 2 args, got %d", len(result.Args))
	}
}

func TestConditionConverter_JSONContains(t *testing.T) {
	c := NewConditionConverter(nil, nil)
	f := filtercond.JSONContains("tags", "prod")
	result, err := c.Convert(f)
	if err != nil {
		t.Fatalf("Convert(json_contains): %v", err)
	}
	if result.Cond == "" {
		t.Error("expected non-empty cond for json_contains")
	}
}

func TestConditionConverter_JSONNotContains(t *testing.T) {
	c := NewConditionConverter(nil, nil)
	f := filtercond.JSONNotContains("tags", "staging")
	result, err := c.Convert(f)
	if err != nil {
		t.Fatalf("Convert(json_not_contains): %v", err)
	}
	if result.Cond == "" {
		t.Error("expected non-empty cond for json_not_contains")
	}
}

func TestConditionConverter_And(t *testing.T) {
	c := NewConditionConverter(nil, nil)
	f := filtercond.And(
		filtercond.Equal("status", 1),
		filtercond.GreaterThan("priority", 0),
	)
	result, err := c.Convert(f)
	if err != nil {
		t.Fatalf("Convert(and): %v", err)
	}
	if result.Cond == "" {
		t.Error("expected non-empty cond for and")
	}
	if len(result.Args) != 2 {
		t.Errorf("expected 2 args, got %d", len(result.Args))
	}
}

func TestConditionConverter_Or(t *testing.T) {
	c := NewConditionConverter(nil, nil)
	f := filtercond.Or(
		filtercond.Equal("status", 1),
		filtercond.Equal("status", 2),
	)
	result, err := c.Convert(f)
	if err != nil {
		t.Fatalf("Convert(or): %v", err)
	}
	if result.Cond == "" {
		t.Error("expected non-empty cond for or")
	}
}

func TestConditionConverter_FieldMapping(t *testing.T) {
	fieldMapping := map[string]string{
		"myField": "actual_column",
	}
	c := NewConditionConverter(fieldMapping, nil)
	f := filtercond.Equal("myField", "value")
	result, err := c.Convert(f)
	if err != nil {
		t.Fatalf("Convert with field mapping: %v", err)
	}
	// result should contain "actual_column"
	if result.Cond == "" {
		t.Error("expected non-empty cond")
	}
}

func TestConditionConverter_UnsupportedOperator(t *testing.T) {
	c := NewConditionConverter(nil, nil)
	f := &filtercond.Filter{Field: "status", Operator: "unsupported_op", Value: 1}
	_, err := c.Convert(f)
	if err == nil {
		t.Error("expected error for unsupported operator")
	}
}

// ========================
// SearchAccounts/SearchProviders with ExtraCond 集成测试
// ========================

func TestSearchAccounts_WithExtraCond_Equal(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	mustAddProvider(t, s, "prov-extra")
	mustAddAccount(t, s, "extra-prio5", "prov-extra", account.StatusAvailable)
	mustAddAccount(t, s, "extra-prio10", "prov-extra", account.StatusAvailable)

	// 更新第二个账号的 priority
	acct, _ := s.GetAccount(ctx, "extra-prio10")
	acct.Priority = 10
	_ = s.UpdateAccount(ctx, acct, storage.UpdateFieldPriority)

	// 用 ExtraCond 过滤 priority=10
	results, err := s.SearchAccounts(ctx, &storage.SearchFilter{
		ExtraCond: filtercond.Equal(storage.AccountFieldPriority, 10),
	})
	if err != nil {
		t.Fatalf("SearchAccounts with ExtraCond: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != "extra-prio10" {
		t.Errorf("expected extra-prio10, got %s", results[0].ID)
	}
}

func TestSearchAccounts_WithExtraCond_And(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	mustAddProvider(t, s, "prov-and")
	mustAddAccount(t, s, "and-1", "prov-and", account.StatusAvailable)
	mustAddAccount(t, s, "and-2", "prov-and", account.StatusDisabled)

	// ExtraCond: priority >= 0 AND status = available
	results, err := s.SearchAccounts(ctx, &storage.SearchFilter{
		ExtraCond: filtercond.And(
			filtercond.GreaterThanOrEqual(storage.AccountFieldPriority, 0),
			filtercond.Equal(storage.AccountFieldStatus, int(account.StatusAvailable)),
		),
	})
	if err != nil {
		t.Fatalf("SearchAccounts with And ExtraCond: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 available result, got %d", len(results))
	}
}

func TestSearchProviders_WithExtraCond(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	mustAddProvider(t, s, "prov-ex-a")
	mustAddProvider(t, s, "prov-ex-b")

	// 更新 prov-ex-b 的 priority
	pb := newTestProvider("prov-ex-b")
	pb.Priority = 50
	_ = s.UpdateProvider(ctx, pb)

	results, err := s.SearchProviders(ctx, &storage.SearchFilter{
		ExtraCond: filtercond.Equal(storage.ProviderFieldPriority, 50),
	})
	if err != nil {
		t.Fatalf("SearchProviders with ExtraCond: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(results))
	}
	if results[0].ProviderName != "prov-ex-b" {
		t.Errorf("expected prov-ex-b, got %s", results[0].ProviderName)
	}
}

// ========================
// ClientBuilder 选项测试
// ========================

func TestClientBuilderOpts_MaxConns(t *testing.T) {
	store, err := NewStore(
		WithDSN(":memory:"),
		WithStoreExtraOptions(),
	)
	if err != nil {
		t.Fatalf("NewStore with extra options: %v", err)
	}
	defer store.Close()
}

// ========================
// ClientBuilder 选项测试
// ========================

func TestClientBuilder_WithMaxOpenConns(t *testing.T) {
	// 通过 defaultClientBuilder 直接测试连接池选项
	client, err := defaultClientBuilder(
		WithClientBuilderDSN(":memory:"),
		WithMaxOpenConns(5),
		WithMaxIdleConns(2),
		WithConnMaxLifetime(time.Minute),
		WithConnMaxIdleTime(30*time.Second),
	)
	if err != nil {
		t.Fatalf("defaultClientBuilder with pool options: %v", err)
	}
	defer client.Close()
}

func TestClientBuilder_WithExtraOptions(t *testing.T) {
	client, err := defaultClientBuilder(
		WithClientBuilderDSN(":memory:"),
		WithExtraOptions("extra1", "extra2"),
	)
	if err != nil {
		t.Fatalf("defaultClientBuilder with ExtraOptions: %v", err)
	}
	defer client.Close()
}

func TestSetClientBuilder(t *testing.T) {
	orig := GetClientBuilder()
	defer SetClientBuilder(orig) // 恢复原 builder

	called := false
	SetClientBuilder(func(opts ...ClientBuilderOpt) (Client, error) {
		called = true
		return defaultClientBuilder(opts...)
	})

	store, err := NewStore(WithDSN(":memory:"))
	if err != nil {
		t.Fatalf("NewStore with custom builder: %v", err)
	}
	defer store.Close()
	if !called {
		t.Error("expected custom client builder to be called")
	}
}

// ========================
// Account with CooldownUntil/CircuitOpenUntil
// ========================

func TestAccount_WithCooldownAndCircuit(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	mustAddProvider(t, s, "prov-cd")

	now := time.Now().Add(time.Hour)
	circuitOpen := time.Now().Add(2 * time.Hour)

	err := s.AddAccount(ctx, &account.Account{
		ID:               "acc-cd",
		ProviderType:     testProviderType,
		ProviderName:     "prov-cd",
		Credential:       &mockCredential{token: "tok"},
		Status:           account.StatusCoolingDown,
		CooldownUntil:    &now,
		CircuitOpenUntil: &circuitOpen,
	})
	if err != nil {
		t.Fatalf("AddAccount with CooldownUntil: %v", err)
	}

	got, err := s.GetAccount(ctx, "acc-cd")
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if got.CooldownUntil == nil {
		t.Error("expected CooldownUntil to be set")
	}
	if got.CircuitOpenUntil == nil {
		t.Error("expected CircuitOpenUntil to be set")
	}
}

func TestAccount_UpdateCredential(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	mustAddProvider(t, s, "prov-cred")
	mustAddAccount(t, s, "acc-cred", "prov-cred", account.StatusAvailable)

	got, _ := s.GetAccount(ctx, "acc-cred")
	got.Credential = &mockCredential{token: "new-token"}
	if err := s.UpdateAccount(ctx, got, storage.UpdateFieldCredential); err != nil {
		t.Fatalf("UpdateAccount(credential): %v", err)
	}
}

func TestAccount_UpdateUsageRules(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	mustAddProvider(t, s, "prov-ur")
	mustAddAccount(t, s, "acc-ur", "prov-ur", account.StatusAvailable)

	got, _ := s.GetAccount(ctx, "acc-ur")
	got.UsageRules = []*usagerule.UsageRule{
		{SourceType: usagerule.SourceTypeRequest, TimeGranularity: usagerule.GranularityHour, WindowSize: 1, Total: 100},
	}
	if err := s.UpdateAccount(ctx, got, storage.UpdateFieldUsageRules); err != nil {
		t.Fatalf("UpdateAccount(usage_rules): %v", err)
	}

	updated, _ := s.GetAccount(ctx, "acc-ur")
	if len(updated.UsageRules) != 1 {
		t.Errorf("expected 1 usage rule, got %d", len(updated.UsageRules))
	}
}

func TestAccount_UpdateStatus_WithCooldown(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	mustAddProvider(t, s, "prov-cd2")
	mustAddAccount(t, s, "acc-cd2", "prov-cd2", account.StatusAvailable)

	got, _ := s.GetAccount(ctx, "acc-cd2")
	cooldown := time.Now().Add(time.Hour)
	got.Status = account.StatusCoolingDown
	got.CooldownUntil = &cooldown
	if err := s.UpdateAccount(ctx, got, storage.UpdateFieldStatus); err != nil {
		t.Fatalf("UpdateAccount(status+cooldown): %v", err)
	}

	updated, _ := s.GetAccount(ctx, "acc-cd2")
	if updated.CooldownUntil == nil {
		t.Error("expected CooldownUntil to be set after update")
	}
}

// ========================
// Provider with 完整字段
// ========================

func TestProvider_WithUsageRulesAndTags(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	err := s.AddProvider(ctx, &account.ProviderInfo{
		ProviderType:    testProviderType,
		ProviderName:    "prov-full",
		Status:          account.ProviderStatusActive,
		Tags:            map[string]string{"env": "prod"},
		Metadata:        map[string]any{"region": "us-east"},
		SupportedModels: []string{"model-a", "model-b"},
	})
	if err != nil {
		t.Fatalf("AddProvider with full fields: %v", err)
	}

	got, err := s.GetProvider(ctx, account.BuildProviderKey(testProviderType, "prov-full"))
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	if len(got.SupportedModels) != 2 {
		t.Errorf("expected 2 supported models, got %d", len(got.SupportedModels))
	}
	if got.Tags["env"] != "prod" {
		t.Errorf("expected tags to be set")
	}
}

// ========================
// SearchAccounts 更多 ExtraCond 测试
// ========================

func TestSearchAccounts_ExtraCond_In(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	mustAddProvider(t, s, "prov-in")
	mustAddAccount(t, s, "in-1", "prov-in", account.StatusAvailable)
	mustAddAccount(t, s, "in-2", "prov-in", account.StatusDisabled)
	mustAddAccount(t, s, "in-3", "prov-in", account.StatusCoolingDown)

	results, err := s.SearchAccounts(ctx, &storage.SearchFilter{
		ExtraCond: filtercond.In(storage.AccountFieldID, "in-1", "in-2"),
	})
	if err != nil {
		t.Fatalf("SearchAccounts with In: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestSearchAccounts_ExtraCond_Or(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	mustAddProvider(t, s, "prov-or")
	mustAddAccount(t, s, "or-avail", "prov-or", account.StatusAvailable)
	mustAddAccount(t, s, "or-disabled", "prov-or", account.StatusDisabled)
	mustAddAccount(t, s, "or-cooling", "prov-or", account.StatusCoolingDown)

	results, err := s.SearchAccounts(ctx, &storage.SearchFilter{
		ExtraCond: filtercond.Or(
			filtercond.Equal(storage.AccountFieldStatus, int(account.StatusAvailable)),
			filtercond.Equal(storage.AccountFieldStatus, int(account.StatusDisabled)),
		),
	})
	if err != nil {
		t.Fatalf("SearchAccounts with Or: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}
