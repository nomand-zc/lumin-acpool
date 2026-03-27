package redis

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-acpool/storage/filtercond"
	goredis "github.com/redis/go-redis/v9"
)

// ============================
// FilterEvaluator 测试（filter_eval.go）
// ============================

func makeAccountEvaluator() *FilterEvaluator {
	return NewFilterEvaluator(accountFieldExtractor)
}

func makeTestAccountForFilter(id, provType, provName string, status account.Status, priority int) *account.Account {
	return &account.Account{
		ID:           id,
		ProviderType: provType,
		ProviderName: provName,
		Status:       status,
		Priority:     priority,
	}
}

func TestFilterEvaluator_Match_Nil(t *testing.T) {
	ev := makeAccountEvaluator()
	acct := makeTestAccountForFilter("acc-1", "kiro", "k1", account.StatusAvailable, 10)
	if !ev.Match(acct, nil) {
		t.Error("expected nil filter to match all")
	}
}

func TestFilterEvaluator_Match_Equal(t *testing.T) {
	ev := makeAccountEvaluator()
	acct := makeTestAccountForFilter("acc-1", "kiro", "k1", account.StatusAvailable, 10)

	if !ev.Match(acct, filtercond.Equal(storage.AccountFieldProviderType, "kiro")) {
		t.Error("expected kiro to match Equal 'kiro'")
	}
	if ev.Match(acct, filtercond.Equal(storage.AccountFieldProviderType, "other")) {
		t.Error("expected no match for 'other'")
	}
}

func TestFilterEvaluator_Match_NotEqual(t *testing.T) {
	ev := makeAccountEvaluator()
	acct := makeTestAccountForFilter("acc-1", "kiro", "k1", account.StatusAvailable, 10)

	if !ev.Match(acct, filtercond.NotEqual(storage.AccountFieldProviderType, "other")) {
		t.Error("expected match for NotEqual 'other'")
	}
}

func TestFilterEvaluator_Match_GreaterThan(t *testing.T) {
	ev := makeAccountEvaluator()
	acct := makeTestAccountForFilter("acc-1", "kiro", "k1", account.StatusAvailable, 10)

	if !ev.Match(acct, filtercond.GreaterThan(storage.AccountFieldPriority, 5)) {
		t.Error("expected priority 10 > 5 to match")
	}
	if ev.Match(acct, filtercond.GreaterThan(storage.AccountFieldPriority, 15)) {
		t.Error("expected priority 10 not > 15")
	}
}

func TestFilterEvaluator_Match_GreaterThanOrEqual(t *testing.T) {
	ev := makeAccountEvaluator()
	acct := makeTestAccountForFilter("acc-1", "kiro", "k1", account.StatusAvailable, 10)

	if !ev.Match(acct, filtercond.GreaterThanOrEqual(storage.AccountFieldPriority, 10)) {
		t.Error("expected priority 10 >= 10 to match")
	}
}

func TestFilterEvaluator_Match_LessThan(t *testing.T) {
	ev := makeAccountEvaluator()
	acct := makeTestAccountForFilter("acc-1", "kiro", "k1", account.StatusAvailable, 10)

	if !ev.Match(acct, filtercond.LessThan(storage.AccountFieldPriority, 20)) {
		t.Error("expected priority 10 < 20 to match")
	}
}

func TestFilterEvaluator_Match_LessThanOrEqual(t *testing.T) {
	ev := makeAccountEvaluator()
	acct := makeTestAccountForFilter("acc-1", "kiro", "k1", account.StatusAvailable, 10)

	if !ev.Match(acct, filtercond.LessThanOrEqual(storage.AccountFieldPriority, 10)) {
		t.Error("expected priority 10 <= 10 to match")
	}
}

func TestFilterEvaluator_Match_In(t *testing.T) {
	ev := makeAccountEvaluator()
	acct := makeTestAccountForFilter("acc-1", "kiro", "k1", account.StatusAvailable, 10)

	if !ev.Match(acct, filtercond.In(storage.AccountFieldProviderType, "kiro", "geminicli")) {
		t.Error("expected 'kiro' to be in [kiro, geminicli]")
	}
	if ev.Match(acct, filtercond.In(storage.AccountFieldProviderType, "other1", "other2")) {
		t.Error("expected 'kiro' not in [other1, other2]")
	}
}

func TestFilterEvaluator_Match_NotIn(t *testing.T) {
	ev := makeAccountEvaluator()
	acct := makeTestAccountForFilter("acc-1", "kiro", "k1", account.StatusAvailable, 10)

	if !ev.Match(acct, filtercond.NotIn(storage.AccountFieldProviderType, "other1", "other2")) {
		t.Error("expected 'kiro' to match NotIn [other1, other2]")
	}
}

func TestFilterEvaluator_Match_Like(t *testing.T) {
	ev := makeAccountEvaluator()
	acct := makeTestAccountForFilter("acc-1", "kiro", "k1", account.StatusAvailable, 10)

	if !ev.Match(acct, filtercond.Like(storage.AccountFieldProviderType, "ki%")) {
		t.Error("expected 'kiro' to match 'ki%'")
	}
	if !ev.Match(acct, filtercond.Like(storage.AccountFieldProviderType, "%iro")) {
		t.Error("expected 'kiro' to match '%iro'")
	}
	if !ev.Match(acct, filtercond.Like(storage.AccountFieldProviderType, "%ir%")) {
		t.Error("expected 'kiro' to match '%ir%'")
	}
	if ev.Match(acct, filtercond.Like(storage.AccountFieldProviderType, "%zzz%")) {
		t.Error("expected 'kiro' not to match '%zzz%'")
	}
	// 精确匹配
	if !ev.Match(acct, filtercond.Like(storage.AccountFieldProviderType, "kiro")) {
		t.Error("expected 'kiro' to match exact 'kiro'")
	}
}

func TestFilterEvaluator_Match_NotLike(t *testing.T) {
	ev := makeAccountEvaluator()
	acct := makeTestAccountForFilter("acc-1", "kiro", "k1", account.StatusAvailable, 10)

	if !ev.Match(acct, filtercond.NotLike(storage.AccountFieldProviderType, "%zzz%")) {
		t.Error("expected 'kiro' to match NotLike '%zzz%'")
	}
}

func TestFilterEvaluator_Match_Between(t *testing.T) {
	ev := makeAccountEvaluator()
	acct := makeTestAccountForFilter("acc-1", "kiro", "k1", account.StatusAvailable, 10)

	if !ev.Match(acct, filtercond.Between(storage.AccountFieldPriority, 5, 15)) {
		t.Error("expected priority 10 between [5,15]")
	}
	if ev.Match(acct, filtercond.Between(storage.AccountFieldPriority, 20, 30)) {
		t.Error("expected priority 10 not between [20,30]")
	}
}

func TestFilterEvaluator_Match_And(t *testing.T) {
	ev := makeAccountEvaluator()
	acct := makeTestAccountForFilter("acc-1", "kiro", "k1", account.StatusAvailable, 10)

	f := filtercond.And(
		filtercond.Equal(storage.AccountFieldProviderType, "kiro"),
		filtercond.GreaterThan(storage.AccountFieldPriority, 5),
	)
	if !ev.Match(acct, f) {
		t.Error("expected AND to match")
	}

	f2 := filtercond.And(
		filtercond.Equal(storage.AccountFieldProviderType, "kiro"),
		filtercond.GreaterThan(storage.AccountFieldPriority, 50),
	)
	if ev.Match(acct, f2) {
		t.Error("expected AND to not match (priority too low)")
	}
}

func TestFilterEvaluator_Match_Or(t *testing.T) {
	ev := makeAccountEvaluator()
	acct := makeTestAccountForFilter("acc-1", "kiro", "k1", account.StatusAvailable, 10)

	f := filtercond.Or(
		filtercond.Equal(storage.AccountFieldProviderType, "other"),
		filtercond.Equal(storage.AccountFieldPriority, 10),
	)
	if !ev.Match(acct, f) {
		t.Error("expected OR to match (priority == 10)")
	}

	f2 := filtercond.Or(
		filtercond.Equal(storage.AccountFieldProviderType, "other"),
		filtercond.Equal(storage.AccountFieldPriority, 99),
	)
	if ev.Match(acct, f2) {
		t.Error("expected OR to not match")
	}
}

func TestFilterEvaluator_Match_UnknownField(t *testing.T) {
	ev := makeAccountEvaluator()
	acct := makeTestAccountForFilter("acc-1", "kiro", "k1", account.StatusAvailable, 10)

	f := filtercond.Equal("unknown_field", "value")
	if ev.Match(acct, f) {
		t.Error("expected no match for unknown field")
	}
}

func TestFilterEvaluator_Match_UnsupportedOperator(t *testing.T) {
	ev := makeAccountEvaluator()
	acct := makeTestAccountForFilter("acc-1", "kiro", "k1", account.StatusAvailable, 10)

	f := &filtercond.Filter{
		Field:    storage.AccountFieldProviderType,
		Operator: "UNSUPPORTED",
		Value:    "kiro",
	}
	if ev.Match(acct, f) {
		t.Error("expected no match for unsupported operator")
	}
}

func TestFilterEvaluator_Match_WrongObject(t *testing.T) {
	ev := makeAccountEvaluator()
	// 传入非 account.Account 类型
	f := filtercond.Equal(storage.AccountFieldProviderType, "kiro")
	if ev.Match("not-an-account", f) {
		t.Error("expected no match for wrong object type")
	}
}

// ============================
// SearchAccounts with ExtraCond 集成测试
// ============================

func TestSearchAccounts_WithExtraCond(t *testing.T) {
	ctx := context.Background()
	s := setupStore(t)

	if err := s.AddAccount(ctx, newKiroAccount("acc-p10", "k1", account.StatusAvailable)); err != nil {
		t.Fatal(err)
	}
	// 修改第一个账号 priority
	acct, _ := s.GetAccount(ctx, "acc-p10")
	acct.Priority = 10
	_ = s.UpdateAccount(ctx, acct, storage.UpdateFieldPriority)

	if err := s.AddAccount(ctx, newKiroAccount("acc-p20", "k1", account.StatusAvailable)); err != nil {
		t.Fatal(err)
	}
	acct2, _ := s.GetAccount(ctx, "acc-p20")
	acct2.Priority = 20
	_ = s.UpdateAccount(ctx, acct2, storage.UpdateFieldPriority)

	// 用 ExtraCond 过滤 priority > 15
	results, err := s.SearchAccounts(ctx, &storage.SearchFilter{
		ExtraCond: filtercond.GreaterThan(storage.AccountFieldPriority, 15),
	})
	if err != nil {
		t.Fatalf("SearchAccounts with ExtraCond failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result (priority > 15), got %d", len(results))
	}
	if len(results) > 0 && results[0].ID != "acc-p20" {
		t.Errorf("expected acc-p20, got %s", results[0].ID)
	}
}

// ============================
// option.go 测试
// ============================

func TestWithStoreKeyPrefix(t *testing.T) {
	o := DefaultStoreOptions()
	WithStoreKeyPrefix("myprefix:")(o)
	if o.KeyPrefix != "myprefix:" {
		t.Errorf("expected myprefix:, got %s", o.KeyPrefix)
	}
}

func TestWithDSN(t *testing.T) {
	o := DefaultStoreOptions()
	WithDSN("redis://localhost:6379")(o)
	if o.DSN != "redis://localhost:6379" {
		t.Errorf("expected DSN to be set")
	}
}

func TestWithInstanceName(t *testing.T) {
	o := DefaultStoreOptions()
	WithInstanceName("my-instance")(o)
	if o.InstanceName != "my-instance" {
		t.Errorf("expected InstanceName my-instance, got %s", o.InstanceName)
	}
}

func TestWithStoreExtraOptions(t *testing.T) {
	o := DefaultStoreOptions()
	WithStoreExtraOptions("opt1", "opt2")(o)
	if len(o.ExtraOptions) != 2 {
		t.Errorf("expected 2 extra options, got %d", len(o.ExtraOptions))
	}
}

func TestDefaultStoreOptions(t *testing.T) {
	o := DefaultStoreOptions()
	if o.KeyPrefix != "acpool:" {
		t.Errorf("expected default key prefix 'acpool:', got %s", o.KeyPrefix)
	}
}

func TestBuildStoreClient_InstanceNotFound(t *testing.T) {
	o := DefaultStoreOptions()
	o.InstanceName = "nonexistent-instance"
	_, err := buildStoreClient(o)
	if err == nil {
		t.Error("expected error for nonexistent instance")
	}
}

func TestNewStore_WithMiniredis(t *testing.T) {
	mr := miniredis.RunT(t)

	origBuilder := GetClientBuilder()
	t.Cleanup(func() { SetClientBuilder(origBuilder) })

	SetClientBuilder(func(opts ...ClientBuilderOpt) (Client, error) {
		rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
		return WrapGoRedis(rdb), nil
	})

	store, err := NewStore(WithStoreKeyPrefix("newstore-test:"))
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()
}

func TestBuildStoreClient_RegisteredInstance(t *testing.T) {
	mr := miniredis.RunT(t)

	origBuilder := GetClientBuilder()
	t.Cleanup(func() { SetClientBuilder(origBuilder) })

	SetClientBuilder(func(opts ...ClientBuilderOpt) (Client, error) {
		rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
		return WrapGoRedis(rdb), nil
	})

	RegisterInstance("test-instance-2")

	o := DefaultStoreOptions()
	o.InstanceName = "test-instance-2"
	client, err := buildStoreClient(o)
	if err != nil {
		t.Fatalf("buildStoreClient with registered instance failed: %v", err)
	}
	if client == nil {
		t.Error("expected non-nil client")
	}
	defer client.Close()
}

func TestBuildStoreClient_WithExtraOptions(t *testing.T) {
	mr := miniredis.RunT(t)

	origBuilder := GetClientBuilder()
	t.Cleanup(func() { SetClientBuilder(origBuilder) })

	SetClientBuilder(func(opts ...ClientBuilderOpt) (Client, error) {
		rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
		return WrapGoRedis(rdb), nil
	})

	o := DefaultStoreOptions()
	o.ExtraOptions = []any{"extra"}
	client, err := buildStoreClient(o)
	if err != nil {
		t.Fatalf("buildStoreClient with extra options failed: %v", err)
	}
	defer client.Close()
}
