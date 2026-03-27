package redis

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	goredis "github.com/redis/go-redis/v9"

	_ "github.com/nomand-zc/lumin-client/credentials/geminicli"
	kirocreds "github.com/nomand-zc/lumin-client/credentials/kiro"
)

const (
	// testKiroCred 是测试用的 kiro credential JSON
	testKiroCred = `{"accessToken":"test-token","refreshToken":"test-refresh","profileArn":"arn:test","authMethod":"social","region":"us-east-1"}`
)

// setupStore 创建一个使用 miniredis 的测试用 Store
func setupStore(t *testing.T) *Store {
	t.Helper()
	mr := miniredis.RunT(t)

	rdb := goredis.NewClient(&goredis.Options{
		Addr: mr.Addr(),
	})

	client := WrapGoRedis(rdb)

	store := &Store{
		client:            client,
		keyPrefix:         "test:",
		accountEvaluator:  NewFilterEvaluator(accountFieldExtractor),
		providerEvaluator: NewFilterEvaluator(providerFieldExtractor),
	}
	t.Cleanup(func() { rdb.Close() })
	return store
}

// newTestAccount 创建一个用于测试的 Account，含最小 kiro credential
// 如果 provType 是 kiro，使用 kiro credential；否则使用 kiro credential（仅用于索引测试）
func newTestAccount(id, provType, provName string, status account.Status) *account.Account {
	cred := kirocreds.NewCredential([]byte(testKiroCred))
	return &account.Account{
		ID:           id,
		ProviderType: provType,
		ProviderName: provName,
		Status:       status,
		Credential:   cred,
	}
}

// newKiroAccount 创建 kiro 类型账号（可被完整 unmarshal）
func newKiroAccount(id, provName string, status account.Status) *account.Account {
	return newTestAccount(id, "kiro", provName, status)
}

// newTestProvider 创建一个用于测试的 ProviderInfo
func newTestProvider(provType, provName string) *account.ProviderInfo {
	return &account.ProviderInfo{
		ProviderType: provType,
		ProviderName: provName,
		Status:       account.ProviderStatusActive,
		Priority:     10,
		Weight:       100,
	}
}

// ============================
// AccountStorage 测试
// ============================

func TestAddAccount_Success(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	acct := newTestAccount("acc-1", "kiro", "kiro-main", account.StatusAvailable)
	if err := s.AddAccount(ctx, acct); err != nil {
		t.Fatalf("AddAccount failed: %v", err)
	}

	// 验证版本和时间戳被设置
	if acct.Version != 1 {
		t.Errorf("expected version 1, got %d", acct.Version)
	}
	if acct.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestAddAccount_Duplicate(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	acct := newTestAccount("acc-dup", "kiro", "kiro-main", account.StatusAvailable)
	if err := s.AddAccount(ctx, acct); err != nil {
		t.Fatalf("first AddAccount failed: %v", err)
	}

	acct2 := newTestAccount("acc-dup", "kiro", "kiro-main", account.StatusAvailable)
	err := s.AddAccount(ctx, acct2)
	if err != storage.ErrAlreadyExists {
		t.Errorf("expected ErrAlreadyExists, got %v", err)
	}
}

func TestGetAccount_Success(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	acct := newTestAccount("acc-get", "kiro", "kiro-main", account.StatusAvailable)
	acct.Priority = 5
	acct.Tags = map[string]string{"env": "prod"}
	if err := s.AddAccount(ctx, acct); err != nil {
		t.Fatalf("AddAccount failed: %v", err)
	}

	got, err := s.GetAccount(ctx, "acc-get")
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if got.ID != "acc-get" {
		t.Errorf("expected ID acc-get, got %s", got.ID)
	}
	if got.ProviderType != "kiro" {
		t.Errorf("expected ProviderType kiro, got %s", got.ProviderType)
	}
	if got.Status != account.StatusAvailable {
		t.Errorf("expected StatusAvailable, got %v", got.Status)
	}
	if got.Priority != 5 {
		t.Errorf("expected priority 5, got %d", got.Priority)
	}
	if got.Tags["env"] != "prod" {
		t.Errorf("expected tag env=prod, got %v", got.Tags)
	}
}

func TestGetAccount_NotFound(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	_, err := s.GetAccount(ctx, "nonexistent")
	if err != storage.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestSearchAccounts_All(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	for i, id := range []string{"sa-1", "sa-2", "sa-3"} {
		acct := newTestAccount(id, "kiro", "kiro-main", account.StatusAvailable)
		_ = i
		if err := s.AddAccount(ctx, acct); err != nil {
			t.Fatalf("AddAccount %s failed: %v", id, err)
		}
	}

	results, err := s.SearchAccounts(ctx, nil)
	if err != nil {
		t.Fatalf("SearchAccounts failed: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
}

func TestSearchAccounts_ByProviderType(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	if err := s.AddAccount(ctx, newKiroAccount("acc-kiro-1", "k1", account.StatusAvailable)); err != nil {
		t.Fatal(err)
	}
	if err := s.AddAccount(ctx, newKiroAccount("acc-kiro-2", "k2", account.StatusAvailable)); err != nil {
		t.Fatal(err)
	}

	// 添加另一种 provider type（使用 kiro credential 但不同的 providerType key）
	acct3 := newTestAccount("acc-other-1", "kiro-alt", "alt1", account.StatusAvailable)
	if err := s.AddAccount(ctx, acct3); err != nil {
		t.Fatal(err)
	}

	results, err := s.SearchAccounts(ctx, &storage.SearchFilter{ProviderType: "kiro"})
	if err != nil {
		t.Fatalf("SearchAccounts failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 result for kiro, got %d", len(results))
	}
}

func TestSearchAccounts_ByStatus(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	if err := s.AddAccount(ctx, newTestAccount("acc-avail", "kiro", "k1", account.StatusAvailable)); err != nil {
		t.Fatal(err)
	}
	if err := s.AddAccount(ctx, newTestAccount("acc-disabled", "kiro", "k1", account.StatusDisabled)); err != nil {
		t.Fatal(err)
	}

	results, err := s.SearchAccounts(ctx, &storage.SearchFilter{
		ProviderType: "kiro",
		ProviderName: "k1",
		Status:       int(account.StatusAvailable),
	})
	if err != nil {
		t.Fatalf("SearchAccounts by status failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 available, got %d", len(results))
	}
}

func TestSearchAccounts_ByProviderTypeAndName(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	if err := s.AddAccount(ctx, newTestAccount("acc-a1", "kiro", "kiro-a", account.StatusAvailable)); err != nil {
		t.Fatal(err)
	}
	if err := s.AddAccount(ctx, newTestAccount("acc-b1", "kiro", "kiro-b", account.StatusAvailable)); err != nil {
		t.Fatal(err)
	}

	results, err := s.SearchAccounts(ctx, &storage.SearchFilter{
		ProviderType: "kiro",
		ProviderName: "kiro-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ID != "acc-a1" {
		t.Errorf("expected acc-a1, got %v", results)
	}
}

func TestUpdateAccount_Success(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	acct := newTestAccount("acc-upd", "kiro", "kiro-main", account.StatusAvailable)
	if err := s.AddAccount(ctx, acct); err != nil {
		t.Fatalf("AddAccount failed: %v", err)
	}

	// 修改优先级
	acct.Priority = 99
	if err := s.UpdateAccount(ctx, acct, storage.UpdateFieldPriority); err != nil {
		t.Fatalf("UpdateAccount failed: %v", err)
	}

	got, err := s.GetAccount(ctx, "acc-upd")
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if got.Priority != 99 {
		t.Errorf("expected priority 99, got %d", got.Priority)
	}
}

func TestUpdateAccount_VersionConflict(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	acct := newTestAccount("acc-ver", "kiro", "kiro-main", account.StatusAvailable)
	if err := s.AddAccount(ctx, acct); err != nil {
		t.Fatalf("AddAccount failed: %v", err)
	}

	// 传错误版本
	acct.Version = 99
	err := s.UpdateAccount(ctx, acct, storage.UpdateFieldPriority)
	if err != storage.ErrVersionConflict {
		t.Errorf("expected ErrVersionConflict, got %v", err)
	}
}

func TestUpdateAccount_StatusChange(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	acct := newTestAccount("acc-status", "kiro", "kiro-main", account.StatusAvailable)
	if err := s.AddAccount(ctx, acct); err != nil {
		t.Fatalf("AddAccount failed: %v", err)
	}

	acct.Status = account.StatusDisabled
	if err := s.UpdateAccount(ctx, acct, storage.UpdateFieldStatus); err != nil {
		t.Fatalf("UpdateAccount status failed: %v", err)
	}

	got, _ := s.GetAccount(ctx, "acc-status")
	if got.Status != account.StatusDisabled {
		t.Errorf("expected disabled status, got %v", got.Status)
	}
}

func TestRemoveAccount_Basic(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	acct := newTestAccount("acc-rm", "kiro", "kiro-main", account.StatusAvailable)
	if err := s.AddAccount(ctx, acct); err != nil {
		t.Fatal(err)
	}

	if err := s.RemoveAccount(ctx, "acc-rm"); err != nil {
		t.Fatalf("RemoveAccount failed: %v", err)
	}

	_, err := s.GetAccount(ctx, "acc-rm")
	if err != storage.ErrNotFound {
		t.Errorf("expected ErrNotFound after remove, got %v", err)
	}
}

func TestRemoveAccount_NotFound(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	err := s.RemoveAccount(ctx, "no-such-account")
	if err != storage.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestRemoveAccounts_Basic(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	for _, id := range []string{"ra-1", "ra-2", "ra-3"} {
		if err := s.AddAccount(ctx, newKiroAccount(id, "kiro-main", account.StatusAvailable)); err != nil {
			t.Fatal(err)
		}
	}
	// 添加另一个 providerName 下的账号
	if err := s.AddAccount(ctx, newKiroAccount("ra-other", "kiro-other", account.StatusAvailable)); err != nil {
		t.Fatal(err)
	}

	err := s.RemoveAccounts(ctx, &storage.SearchFilter{ProviderType: "kiro", ProviderName: "kiro-main"})
	if err != nil {
		t.Fatalf("RemoveAccounts failed: %v", err)
	}

	results, _ := s.SearchAccounts(ctx, &storage.SearchFilter{ProviderType: "kiro", ProviderName: "kiro-main"})
	if len(results) != 0 {
		t.Errorf("expected 0 kiro-main accounts after remove, got %d", len(results))
	}

	// kiro-other 应仍存在
	results, _ = s.SearchAccounts(ctx, &storage.SearchFilter{ProviderType: "kiro", ProviderName: "kiro-other"})
	if len(results) != 1 {
		t.Errorf("expected 1 kiro-other account remaining, got %d", len(results))
	}
}

func TestCountAccounts_NoFilter(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	for _, id := range []string{"cnt-1", "cnt-2"} {
		if err := s.AddAccount(ctx, newTestAccount(id, "kiro", "kiro-main", account.StatusAvailable)); err != nil {
			t.Fatal(err)
		}
	}

	count, err := s.CountAccounts(ctx, nil)
	if err != nil {
		t.Fatalf("CountAccounts failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected count 2, got %d", count)
	}
}

func TestCountAccounts_WithFilter(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	if err := s.AddAccount(ctx, newTestAccount("cf-1", "kiro", "k1", account.StatusAvailable)); err != nil {
		t.Fatal(err)
	}
	if err := s.AddAccount(ctx, newTestAccount("cf-2", "kiro", "k1", account.StatusDisabled)); err != nil {
		t.Fatal(err)
	}
	if err := s.AddAccount(ctx, newTestAccount("cf-3", "gemini", "g1", account.StatusAvailable)); err != nil {
		t.Fatal(err)
	}

	count, err := s.CountAccounts(ctx, &storage.SearchFilter{ProviderType: "kiro", ProviderName: "k1", Status: int(account.StatusAvailable)})
	if err != nil {
		t.Fatalf("CountAccounts with filter failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1, got %d", count)
	}
}

// ============================
// ProviderStorage 测试
// ============================

func TestAddProvider_Success(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	prov := newTestProvider("kiro", "kiro-main")
	prov.SupportedModels = []string{"model-a", "model-b"}
	if err := s.AddProvider(ctx, prov); err != nil {
		t.Fatalf("AddProvider failed: %v", err)
	}
}

func TestAddProvider_Duplicate(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	prov := newTestProvider("kiro", "kiro-dup")
	if err := s.AddProvider(ctx, prov); err != nil {
		t.Fatal(err)
	}
	err := s.AddProvider(ctx, newTestProvider("kiro", "kiro-dup"))
	if err != storage.ErrAlreadyExists {
		t.Errorf("expected ErrAlreadyExists, got %v", err)
	}
}

func TestGetProvider_Success(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	prov := newTestProvider("kiro", "kiro-get")
	prov.Weight = 50
	if err := s.AddProvider(ctx, prov); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetProvider(ctx, account.ProviderKey{Type: "kiro", Name: "kiro-get"})
	if err != nil {
		t.Fatalf("GetProvider failed: %v", err)
	}
	if got.ProviderType != "kiro" || got.ProviderName != "kiro-get" {
		t.Errorf("unexpected provider: %+v", got)
	}
	if got.Weight != 50 {
		t.Errorf("expected weight 50, got %d", got.Weight)
	}
}

func TestGetProvider_NotFound(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	_, err := s.GetProvider(ctx, account.ProviderKey{Type: "kiro", Name: "no-such"})
	if err != storage.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestSearchProviders_All(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	for _, name := range []string{"k1", "k2"} {
		if err := s.AddProvider(ctx, newTestProvider("kiro", name)); err != nil {
			t.Fatal(err)
		}
	}

	results, err := s.SearchProviders(ctx, nil)
	if err != nil {
		t.Fatalf("SearchProviders failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 providers, got %d", len(results))
	}
}

func TestSearchProviders_ByModel(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	p1 := newTestProvider("kiro", "p1")
	p1.SupportedModels = []string{"model-x", "model-y"}
	if err := s.AddProvider(ctx, p1); err != nil {
		t.Fatal(err)
	}

	p2 := newTestProvider("kiro", "p2")
	p2.SupportedModels = []string{"model-z"}
	if err := s.AddProvider(ctx, p2); err != nil {
		t.Fatal(err)
	}

	results, err := s.SearchProviders(ctx, &storage.SearchFilter{SupportedModel: "model-x"})
	if err != nil {
		t.Fatalf("SearchProviders by model failed: %v", err)
	}
	if len(results) != 1 || results[0].ProviderName != "p1" {
		t.Errorf("expected p1 only, got %v", results)
	}
}

func TestSearchProviders_ByType(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	if err := s.AddProvider(ctx, newTestProvider("kiro", "k1")); err != nil {
		t.Fatal(err)
	}
	if err := s.AddProvider(ctx, newTestProvider("kiro-alt", "alt1")); err != nil {
		t.Fatal(err)
	}

	results, err := s.SearchProviders(ctx, &storage.SearchFilter{ProviderType: "kiro"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ProviderType != "kiro" {
		t.Errorf("expected kiro only, got %v", results)
	}
}

func TestUpdateProvider_Success(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	prov := newTestProvider("kiro", "kiro-upd")
	if err := s.AddProvider(ctx, prov); err != nil {
		t.Fatal(err)
	}

	prov.Priority = 99
	prov.Weight = 200
	if err := s.UpdateProvider(ctx, prov); err != nil {
		t.Fatalf("UpdateProvider failed: %v", err)
	}

	got, err := s.GetProvider(ctx, account.ProviderKey{Type: "kiro", Name: "kiro-upd"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Priority != 99 || got.Weight != 200 {
		t.Errorf("expected priority=99 weight=200, got %+v", got)
	}
}

func TestUpdateProvider_NotFound(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	err := s.UpdateProvider(ctx, newTestProvider("kiro", "not-exist"))
	if err != storage.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestRemoveProvider_Basic(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	prov := newTestProvider("kiro", "kiro-rm")
	if err := s.AddProvider(ctx, prov); err != nil {
		t.Fatal(err)
	}

	if err := s.RemoveProvider(ctx, account.ProviderKey{Type: "kiro", Name: "kiro-rm"}); err != nil {
		t.Fatalf("RemoveProvider failed: %v", err)
	}

	_, err := s.GetProvider(ctx, account.ProviderKey{Type: "kiro", Name: "kiro-rm"})
	if err != storage.ErrNotFound {
		t.Errorf("expected ErrNotFound after remove, got %v", err)
	}
}

func TestRemoveProvider_NotFound(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	err := s.RemoveProvider(ctx, account.ProviderKey{Type: "kiro", Name: "no-such"})
	if err != storage.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestRemoveProvider_CascadesAccounts(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	prov := newTestProvider("kiro", "kiro-cascade")
	if err := s.AddProvider(ctx, prov); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"casc-1", "casc-2"} {
		if err := s.AddAccount(ctx, newTestAccount(id, "kiro", "kiro-cascade", account.StatusAvailable)); err != nil {
			t.Fatal(err)
		}
	}

	if err := s.RemoveProvider(ctx, account.ProviderKey{Type: "kiro", Name: "kiro-cascade"}); err != nil {
		t.Fatalf("RemoveProvider cascade failed: %v", err)
	}

	results, _ := s.SearchAccounts(ctx, &storage.SearchFilter{ProviderType: "kiro", ProviderName: "kiro-cascade"})
	if len(results) != 0 {
		t.Errorf("expected 0 accounts after cascade remove, got %d", len(results))
	}
}

// ============================
// StatsStore 测试
// ============================

func TestIncrSuccess_Basic(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	if err := s.IncrSuccess(ctx, "stats-acc-1"); err != nil {
		t.Fatalf("IncrSuccess failed: %v", err)
	}
	if err := s.IncrSuccess(ctx, "stats-acc-1"); err != nil {
		t.Fatalf("IncrSuccess 2nd failed: %v", err)
	}

	stats, err := s.GetStats(ctx, "stats-acc-1")
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}
	if stats.TotalCalls != 2 {
		t.Errorf("expected TotalCalls 2, got %d", stats.TotalCalls)
	}
	if stats.SuccessCalls != 2 {
		t.Errorf("expected SuccessCalls 2, got %d", stats.SuccessCalls)
	}
	if stats.ConsecutiveFailures != 0 {
		t.Errorf("expected ConsecutiveFailures 0 after success, got %d", stats.ConsecutiveFailures)
	}
}

func TestIncrFailure_Basic(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	n, err := s.IncrFailure(ctx, "stats-acc-2", "connection error")
	if err != nil {
		t.Fatalf("IncrFailure failed: %v", err)
	}
	if n != 1 {
		t.Errorf("expected consecutive failures 1, got %d", n)
	}

	n2, _ := s.IncrFailure(ctx, "stats-acc-2", "timeout")
	if n2 != 2 {
		t.Errorf("expected consecutive failures 2, got %d", n2)
	}

	stats, _ := s.GetStats(ctx, "stats-acc-2")
	if stats.FailedCalls != 2 {
		t.Errorf("expected FailedCalls 2, got %d", stats.FailedCalls)
	}
	if stats.LastErrorMsg != "timeout" {
		t.Errorf("expected last error msg 'timeout', got %q", stats.LastErrorMsg)
	}
}

func TestGetStats_EmptyReturnsZero(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	stats, err := s.GetStats(ctx, "no-stats-acc")
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}
	if stats == nil {
		t.Fatal("expected non-nil stats for missing account")
	}
	if stats.TotalCalls != 0 {
		t.Errorf("expected 0 total calls, got %d", stats.TotalCalls)
	}
	if stats.AccountID != "no-stats-acc" {
		t.Errorf("expected AccountID 'no-stats-acc', got %q", stats.AccountID)
	}
}

func TestUpdateLastUsed_Basic(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	now := time.Now().Truncate(time.Second)
	if err := s.UpdateLastUsed(ctx, "stats-acc-3", now); err != nil {
		t.Fatalf("UpdateLastUsed failed: %v", err)
	}

	stats, err := s.GetStats(ctx, "stats-acc-3")
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}
	if stats.LastUsedAt == nil {
		t.Fatal("expected LastUsedAt to be set")
	}
	if stats.LastUsedAt.Unix() != now.Unix() {
		t.Errorf("expected LastUsedAt %v, got %v", now, stats.LastUsedAt)
	}
}

func TestResetConsecutiveFailures_Basic(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	// 先制造一些失败
	for i := 0; i < 3; i++ {
		_, _ = s.IncrFailure(ctx, "stats-acc-4", "err")
	}

	cf, _ := s.GetConsecutiveFailures(ctx, "stats-acc-4")
	if cf != 3 {
		t.Errorf("expected 3 consecutive failures, got %d", cf)
	}

	if err := s.ResetConsecutiveFailures(ctx, "stats-acc-4"); err != nil {
		t.Fatalf("ResetConsecutiveFailures failed: %v", err)
	}

	cf, _ = s.GetConsecutiveFailures(ctx, "stats-acc-4")
	if cf != 0 {
		t.Errorf("expected 0 after reset, got %d", cf)
	}
}

func TestGetConsecutiveFailures_Missing(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	cf, err := s.GetConsecutiveFailures(ctx, "absent-acc")
	if err != nil {
		t.Fatalf("GetConsecutiveFailures failed: %v", err)
	}
	if cf != 0 {
		t.Errorf("expected 0 for absent account, got %d", cf)
	}
}

func TestRemoveStats_Basic(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	_ = s.IncrSuccess(ctx, "stats-rm-acc")
	if err := s.RemoveStats(ctx, "stats-rm-acc"); err != nil {
		t.Fatalf("RemoveStats failed: %v", err)
	}

	stats, _ := s.GetStats(ctx, "stats-rm-acc")
	if stats.TotalCalls != 0 {
		t.Errorf("expected 0 after remove, got %d", stats.TotalCalls)
	}
}

// ============================
// UsageStore 测试
// ============================

func newTestTrackedUsage(localUsed float64) *account.TrackedUsage {
	t := time.Now().Add(1 * time.Hour)
	start := time.Now().Add(-1 * time.Hour)
	return &account.TrackedUsage{
		LocalUsed:    localUsed,
		RemoteUsed:   10.0,
		RemoteRemain: 90.0,
		WindowStart:  &start,
		WindowEnd:    &t,
	}
}

func TestSaveUsages_AndGet(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	usages := []*account.TrackedUsage{
		newTestTrackedUsage(5.0),
		newTestTrackedUsage(3.0),
	}

	if err := s.SaveUsages(ctx, "usage-acc-1", usages); err != nil {
		t.Fatalf("SaveUsages failed: %v", err)
	}

	got, err := s.GetCurrentUsages(ctx, "usage-acc-1")
	if err != nil {
		t.Fatalf("GetCurrentUsages failed: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 usages, got %d", len(got))
	}
}

func TestIncrLocalUsed_Basic(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	usages := []*account.TrackedUsage{newTestTrackedUsage(1.0)}
	if err := s.SaveUsages(ctx, "usage-acc-2", usages); err != nil {
		t.Fatal(err)
	}

	if err := s.IncrLocalUsed(ctx, "usage-acc-2", 0, 5.5); err != nil {
		t.Fatalf("IncrLocalUsed failed: %v", err)
	}

	got, _ := s.GetCurrentUsages(ctx, "usage-acc-2")
	if len(got) != 1 {
		t.Fatalf("expected 1 usage, got %d", len(got))
	}
	// localUsed 应为 1.0 + 5.5 = 6.5
	if got[0].LocalUsed < 6.4 || got[0].LocalUsed > 6.6 {
		t.Errorf("expected LocalUsed ~6.5, got %f", got[0].LocalUsed)
	}
}

func TestIncrLocalUsed_NonExistentKey(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	// 对不存在的 key 执行 IncrLocalUsed 应该是 no-op 不报错
	err := s.IncrLocalUsed(ctx, "nonexistent-acc", 0, 5.0)
	if err != nil {
		t.Errorf("expected no error for nonexistent key, got %v", err)
	}
}

func TestRemoveUsages_Basic(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	usages := []*account.TrackedUsage{newTestTrackedUsage(2.0), newTestTrackedUsage(3.0)}
	if err := s.SaveUsages(ctx, "usage-rm-acc", usages); err != nil {
		t.Fatal(err)
	}

	if err := s.RemoveUsages(ctx, "usage-rm-acc"); err != nil {
		t.Fatalf("RemoveUsages failed: %v", err)
	}

	got, _ := s.GetCurrentUsages(ctx, "usage-rm-acc")
	if len(got) != 0 {
		t.Errorf("expected 0 usages after remove, got %d", len(got))
	}
}

func TestGetCurrentUsages_Empty(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	got, err := s.GetCurrentUsages(ctx, "no-usages-acc")
	if err != nil {
		t.Fatalf("GetCurrentUsages failed: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty, got %d", len(got))
	}
}

func TestCalibrateRule_Basic(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	usages := []*account.TrackedUsage{newTestTrackedUsage(10.0)}
	if err := s.SaveUsages(ctx, "calib-acc", usages); err != nil {
		t.Fatal(err)
	}

	newUsage := newTestTrackedUsage(0.0)
	newUsage.RemoteUsed = 20.0
	newUsage.RemoteRemain = 80.0

	if err := s.CalibrateRule(ctx, "calib-acc", 0, newUsage); err != nil {
		t.Fatalf("CalibrateRule failed: %v", err)
	}

	got, _ := s.GetCurrentUsages(ctx, "calib-acc")
	if len(got) == 0 {
		t.Fatal("expected 1 usage after calibrate")
	}
	// local_used 应被重置为 0
	if got[0].LocalUsed != 0.0 {
		t.Errorf("expected LocalUsed 0 after calibrate, got %f", got[0].LocalUsed)
	}
}

// ============================
// OccupancyStore 测试
// ============================

func TestIncrOccupancy_Basic(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	n, err := s.IncrOccupancy(ctx, "occ-acc-1")
	if err != nil {
		t.Fatalf("IncrOccupancy failed: %v", err)
	}
	if n != 1 {
		t.Errorf("expected occupancy 1, got %d", n)
	}

	n2, _ := s.IncrOccupancy(ctx, "occ-acc-1")
	if n2 != 2 {
		t.Errorf("expected occupancy 2, got %d", n2)
	}
}

func TestDecrOccupancy_Basic(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	_, _ = s.IncrOccupancy(ctx, "occ-acc-2")
	_, _ = s.IncrOccupancy(ctx, "occ-acc-2")

	if err := s.DecrOccupancy(ctx, "occ-acc-2"); err != nil {
		t.Fatalf("DecrOccupancy failed: %v", err)
	}

	n, _ := s.GetOccupancy(ctx, "occ-acc-2")
	if n != 1 {
		t.Errorf("expected occupancy 1 after decr, got %d", n)
	}
}

func TestGetOccupancy_Zero(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	n, err := s.GetOccupancy(ctx, "occ-absent")
	if err != nil {
		t.Fatalf("GetOccupancy failed: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0, got %d", n)
	}
}

func TestGetOccupancies_Basic(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	_, _ = s.IncrOccupancy(ctx, "occ-a")
	_, _ = s.IncrOccupancy(ctx, "occ-a")
	_, _ = s.IncrOccupancy(ctx, "occ-b")

	result, err := s.GetOccupancies(ctx, []string{"occ-a", "occ-b", "occ-c"})
	if err != nil {
		t.Fatalf("GetOccupancies failed: %v", err)
	}
	if result["occ-a"] != 2 {
		t.Errorf("expected occ-a=2, got %d", result["occ-a"])
	}
	if result["occ-b"] != 1 {
		t.Errorf("expected occ-b=1, got %d", result["occ-b"])
	}
	// occ-c 不存在，不应出现在结果中或为 0
	if v, ok := result["occ-c"]; ok && v != 0 {
		t.Errorf("expected occ-c absent or 0, got %d", v)
	}
}

func TestGetOccupancies_Empty(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	result, err := s.GetOccupancies(ctx, []string{})
	if err != nil {
		t.Fatalf("GetOccupancies empty failed: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}
}

// ============================
// AffinityStore 测试
// ============================

func TestSetAffinity_AndGet(t *testing.T) {
	s := setupStore(t)

	s.SetAffinity("session-123", "acc-xyz")

	val, ok := s.GetAffinity("session-123")
	if !ok {
		t.Fatal("expected affinity to exist")
	}
	if val != "acc-xyz" {
		t.Errorf("expected acc-xyz, got %s", val)
	}
}

func TestGetAffinity_NotFound(t *testing.T) {
	s := setupStore(t)

	val, ok := s.GetAffinity("nonexistent-session")
	if ok {
		t.Errorf("expected not found, got val=%s", val)
	}
}

func TestSetAffinity_Overwrite(t *testing.T) {
	s := setupStore(t)

	s.SetAffinity("session-456", "acc-old")
	s.SetAffinity("session-456", "acc-new")

	val, ok := s.GetAffinity("session-456")
	if !ok {
		t.Fatal("expected affinity to exist")
	}
	if val != "acc-new" {
		t.Errorf("expected acc-new, got %s", val)
	}
}

// ============================
// Close 测试
// ============================

func TestStore_Close(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	client := WrapGoRedis(rdb)
	store := &Store{
		client:            client,
		keyPrefix:         "test:",
		accountEvaluator:  NewFilterEvaluator(accountFieldExtractor),
		providerEvaluator: NewFilterEvaluator(providerFieldExtractor),
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}
