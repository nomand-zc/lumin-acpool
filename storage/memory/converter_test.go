package memory

import (
	"context"
	"testing"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-acpool/storage/filtercond"
)

// --- AccountConverter ExtraCond 路径测试 ---

func TestSearchAccounts_ExtraCond_Equal(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "test", ProviderName: "p", Status: account.ProviderStatusActive})
	for _, id := range []string{"acc-1", "acc-2", "acc-3"} {
		_ = store.AddAccount(ctx, &account.Account{
			ID:           id,
			ProviderType: "test",
			ProviderName: "p",
			Status:       account.StatusAvailable,
		})
	}

	result, err := store.SearchAccounts(ctx, &storage.SearchFilter{
		ExtraCond: filtercond.Equal(storage.AccountFieldID, "acc-2"),
	})
	if err != nil {
		t.Fatalf("SearchAccounts with ExtraCond failed: %v", err)
	}
	if len(result) != 1 || result[0].ID != "acc-2" {
		t.Fatalf("expected only acc-2, got %d results", len(result))
	}
}

func TestSearchAccounts_ExtraCond_NotEqual(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "test", ProviderName: "p", Status: account.ProviderStatusActive})
	for _, id := range []string{"acc-1", "acc-2", "acc-3"} {
		_ = store.AddAccount(ctx, &account.Account{
			ID:           id,
			ProviderType: "test",
			ProviderName: "p",
			Status:       account.StatusAvailable,
		})
	}

	result, err := store.SearchAccounts(ctx, &storage.SearchFilter{
		ExtraCond: filtercond.NotEqual(storage.AccountFieldID, "acc-1"),
	})
	if err != nil {
		t.Fatalf("SearchAccounts with NotEqual failed: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 accounts (excluding acc-1), got %d", len(result))
	}
	for _, a := range result {
		if a.ID == "acc-1" {
			t.Error("acc-1 should be excluded")
		}
	}
}

func TestSearchAccounts_ExtraCond_In(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "test", ProviderName: "p", Status: account.ProviderStatusActive})
	for _, id := range []string{"acc-1", "acc-2", "acc-3"} {
		_ = store.AddAccount(ctx, &account.Account{
			ID:           id,
			ProviderType: "test",
			ProviderName: "p",
			Status:       account.StatusAvailable,
		})
	}

	result, err := store.SearchAccounts(ctx, &storage.SearchFilter{
		ExtraCond: filtercond.In(storage.AccountFieldID, "acc-1", "acc-3"),
	})
	if err != nil {
		t.Fatalf("SearchAccounts with In failed: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 accounts (acc-1 and acc-3), got %d", len(result))
	}
}

func TestSearchAccounts_ExtraCond_NotIn(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "test", ProviderName: "p", Status: account.ProviderStatusActive})
	for _, id := range []string{"acc-1", "acc-2", "acc-3"} {
		_ = store.AddAccount(ctx, &account.Account{
			ID:           id,
			ProviderType: "test",
			ProviderName: "p",
			Status:       account.StatusAvailable,
		})
	}

	result, err := store.SearchAccounts(ctx, &storage.SearchFilter{
		ExtraCond: filtercond.NotIn(storage.AccountFieldID, "acc-1", "acc-2"),
	})
	if err != nil {
		t.Fatalf("SearchAccounts with NotIn failed: %v", err)
	}
	if len(result) != 1 || result[0].ID != "acc-3" {
		t.Fatalf("expected only acc-3, got %d results", len(result))
	}
}

func TestSearchAccounts_ExtraCond_Like(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "test", ProviderName: "p", Status: account.ProviderStatusActive})
	_ = store.AddAccount(ctx, &account.Account{ID: "user-alice", ProviderType: "test", ProviderName: "p", Status: account.StatusAvailable})
	_ = store.AddAccount(ctx, &account.Account{ID: "user-bob", ProviderType: "test", ProviderName: "p", Status: account.StatusAvailable})
	_ = store.AddAccount(ctx, &account.Account{ID: "service-x", ProviderType: "test", ProviderName: "p", Status: account.StatusAvailable})

	result, err := store.SearchAccounts(ctx, &storage.SearchFilter{
		ExtraCond: filtercond.Like(storage.AccountFieldID, "user-"),
	})
	if err != nil {
		t.Fatalf("SearchAccounts with Like failed: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 user- accounts, got %d", len(result))
	}
}

func TestSearchAccounts_ExtraCond_NotLike(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "test", ProviderName: "p", Status: account.ProviderStatusActive})
	_ = store.AddAccount(ctx, &account.Account{ID: "user-alice", ProviderType: "test", ProviderName: "p", Status: account.StatusAvailable})
	_ = store.AddAccount(ctx, &account.Account{ID: "service-x", ProviderType: "test", ProviderName: "p", Status: account.StatusAvailable})

	result, err := store.SearchAccounts(ctx, &storage.SearchFilter{
		ExtraCond: filtercond.NotLike(storage.AccountFieldID, "user-"),
	})
	if err != nil {
		t.Fatalf("SearchAccounts with NotLike failed: %v", err)
	}
	if len(result) != 1 || result[0].ID != "service-x" {
		t.Fatalf("expected only service-x, got %d results", len(result))
	}
}

func TestSearchAccounts_ExtraCond_Between(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "test", ProviderName: "p", Status: account.ProviderStatusActive})
	for _, id := range []string{"p1", "p2", "p3", "p4", "p5"} {
		priority := 0
		switch id {
		case "p1":
			priority = 1
		case "p2":
			priority = 2
		case "p3":
			priority = 3
		case "p4":
			priority = 4
		case "p5":
			priority = 5
		}
		_ = store.AddAccount(ctx, &account.Account{
			ID:           id,
			ProviderType: "test",
			ProviderName: "p",
			Status:       account.StatusAvailable,
			Priority:     priority,
		})
	}

	result, err := store.SearchAccounts(ctx, &storage.SearchFilter{
		ExtraCond: filtercond.Between(storage.AccountFieldPriority, 2, 4),
	})
	if err != nil {
		t.Fatalf("SearchAccounts with Between failed: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 accounts with priority 2-4, got %d", len(result))
	}
}

func TestSearchAccounts_ExtraCond_And(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "test", ProviderName: "p", Status: account.ProviderStatusActive})
	_ = store.AddAccount(ctx, &account.Account{ID: "high-avail", ProviderType: "test", ProviderName: "p", Status: account.StatusAvailable, Priority: 10})
	_ = store.AddAccount(ctx, &account.Account{ID: "low-avail", ProviderType: "test", ProviderName: "p", Status: account.StatusAvailable, Priority: 1})
	_ = store.AddAccount(ctx, &account.Account{ID: "high-cool", ProviderType: "test", ProviderName: "p", Status: account.StatusCoolingDown, Priority: 10})

	result, err := store.SearchAccounts(ctx, &storage.SearchFilter{
		ExtraCond: filtercond.And(
			filtercond.Equal(storage.AccountFieldStatus, int(account.StatusAvailable)),
			filtercond.Equal(storage.AccountFieldPriority, 10),
		),
	})
	if err != nil {
		t.Fatalf("SearchAccounts with And failed: %v", err)
	}
	if len(result) != 1 || result[0].ID != "high-avail" {
		t.Fatalf("expected only high-avail, got %d results", len(result))
	}
}

func TestSearchAccounts_ExtraCond_Or(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "test", ProviderName: "p", Status: account.ProviderStatusActive})
	_ = store.AddAccount(ctx, &account.Account{ID: "acc-1", ProviderType: "test", ProviderName: "p", Status: account.StatusAvailable, Priority: 5})
	_ = store.AddAccount(ctx, &account.Account{ID: "acc-2", ProviderType: "test", ProviderName: "p", Status: account.StatusCoolingDown, Priority: 1})
	_ = store.AddAccount(ctx, &account.Account{ID: "acc-3", ProviderType: "test", ProviderName: "p", Status: account.StatusCircuitOpen, Priority: 1})

	result, err := store.SearchAccounts(ctx, &storage.SearchFilter{
		ExtraCond: filtercond.Or(
			filtercond.Equal(storage.AccountFieldID, "acc-1"),
			filtercond.Equal(storage.AccountFieldID, "acc-3"),
		),
	})
	if err != nil {
		t.Fatalf("SearchAccounts with Or failed: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 accounts (acc-1 and acc-3), got %d", len(result))
	}
}

func TestSearchAccounts_ExtraCond_GreaterThan(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "test", ProviderName: "p", Status: account.ProviderStatusActive})
	_ = store.AddAccount(ctx, &account.Account{ID: "a1", ProviderType: "test", ProviderName: "p", Status: account.StatusAvailable, Priority: 1})
	_ = store.AddAccount(ctx, &account.Account{ID: "a5", ProviderType: "test", ProviderName: "p", Status: account.StatusAvailable, Priority: 5})
	_ = store.AddAccount(ctx, &account.Account{ID: "a10", ProviderType: "test", ProviderName: "p", Status: account.StatusAvailable, Priority: 10})

	result, err := store.SearchAccounts(ctx, &storage.SearchFilter{
		ExtraCond: filtercond.GreaterThan(storage.AccountFieldPriority, 4),
	})
	if err != nil {
		t.Fatalf("SearchAccounts with GreaterThan failed: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 accounts with priority > 4, got %d", len(result))
	}
}

func TestSearchAccounts_ExtraCond_LessThan(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "test", ProviderName: "p", Status: account.ProviderStatusActive})
	_ = store.AddAccount(ctx, &account.Account{ID: "a1", ProviderType: "test", ProviderName: "p", Status: account.StatusAvailable, Priority: 1})
	_ = store.AddAccount(ctx, &account.Account{ID: "a5", ProviderType: "test", ProviderName: "p", Status: account.StatusAvailable, Priority: 5})

	result, err := store.SearchAccounts(ctx, &storage.SearchFilter{
		ExtraCond: filtercond.LessThan(storage.AccountFieldPriority, 5),
	})
	if err != nil {
		t.Fatalf("SearchAccounts with LessThan failed: %v", err)
	}
	if len(result) != 1 || result[0].ID != "a1" {
		t.Fatalf("expected only a1 (priority < 5), got %d results", len(result))
	}
}

func TestSearchAccounts_ExtraCond_UnsupportedOperator_Error(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_, err := store.SearchAccounts(ctx, &storage.SearchFilter{
		ExtraCond: &filtercond.Filter{
			Field:    storage.AccountFieldID,
			Operator: "unknown_op",
			Value:    "x",
		},
	})
	if err == nil {
		t.Error("expected error for unsupported operator")
	}
}

func TestSearchAccounts_ExtraCond_UnsupportedField_Error(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_, err := store.SearchAccounts(ctx, &storage.SearchFilter{
		ExtraCond: filtercond.Equal("unsupported_field", "value"),
	})
	if err == nil {
		t.Error("expected error for unsupported field")
	}
}

// --- ProviderConverter ExtraCond 路径测试 ---

func TestSearchProviders_ExtraCond_Equal(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "openai", ProviderName: "p1", Status: account.ProviderStatusActive})
	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "openai", ProviderName: "p2", Status: account.ProviderStatusActive})

	result, err := store.SearchProviders(ctx, &storage.SearchFilter{
		ExtraCond: filtercond.Equal(storage.ProviderFieldName, "p1"),
	})
	if err != nil {
		t.Fatalf("SearchProviders with ExtraCond failed: %v", err)
	}
	if len(result) != 1 || result[0].ProviderName != "p1" {
		t.Fatalf("expected only p1, got %d results", len(result))
	}
}

func TestSearchProviders_ExtraCond_In(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "typeA", ProviderName: "p1", Status: account.ProviderStatusActive})
	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "typeB", ProviderName: "p2", Status: account.ProviderStatusActive})
	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "typeC", ProviderName: "p3", Status: account.ProviderStatusActive})

	result, err := store.SearchProviders(ctx, &storage.SearchFilter{
		ExtraCond: filtercond.In(storage.ProviderFieldType, "typeA", "typeB"),
	})
	if err != nil {
		t.Fatalf("SearchProviders with In failed: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 providers (typeA, typeB), got %d", len(result))
	}
}

func TestSearchProviders_ExtraCond_Like(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "openai", ProviderName: "team-alpha", Status: account.ProviderStatusActive})
	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "openai", ProviderName: "team-beta", Status: account.ProviderStatusActive})
	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "openai", ProviderName: "service-x", Status: account.ProviderStatusActive})

	result, err := store.SearchProviders(ctx, &storage.SearchFilter{
		ExtraCond: filtercond.Like(storage.ProviderFieldName, "team-"),
	})
	if err != nil {
		t.Fatalf("SearchProviders with Like failed: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 team- providers, got %d", len(result))
	}
}

func TestSearchProviders_ExtraCond_NotLike(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "openai", ProviderName: "team-alpha", Status: account.ProviderStatusActive})
	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "openai", ProviderName: "service-x", Status: account.ProviderStatusActive})

	result, err := store.SearchProviders(ctx, &storage.SearchFilter{
		ExtraCond: filtercond.NotLike(storage.ProviderFieldName, "team-"),
	})
	if err != nil {
		t.Fatalf("SearchProviders with NotLike failed: %v", err)
	}
	if len(result) != 1 || result[0].ProviderName != "service-x" {
		t.Fatalf("expected only service-x, got %d results", len(result))
	}
}

func TestSearchProviders_ExtraCond_And(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "openai", ProviderName: "p1", Status: account.ProviderStatusActive, Priority: 10})
	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "openai", ProviderName: "p2", Status: account.ProviderStatusActive, Priority: 1})
	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "openai", ProviderName: "p3", Status: account.ProviderStatusDisabled, Priority: 10})

	result, err := store.SearchProviders(ctx, &storage.SearchFilter{
		ExtraCond: filtercond.And(
			filtercond.Equal(storage.ProviderFieldStatus, int(account.ProviderStatusActive)),
			filtercond.Equal(storage.ProviderFieldPriority, 10),
		),
	})
	if err != nil {
		t.Fatalf("SearchProviders with And failed: %v", err)
	}
	if len(result) != 1 || result[0].ProviderName != "p1" {
		t.Fatalf("expected only p1, got %d results", len(result))
	}
}

func TestSearchProviders_ExtraCond_Or(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "openai", ProviderName: "p1", Status: account.ProviderStatusActive})
	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "openai", ProviderName: "p2", Status: account.ProviderStatusActive})
	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "openai", ProviderName: "p3", Status: account.ProviderStatusActive})

	result, err := store.SearchProviders(ctx, &storage.SearchFilter{
		ExtraCond: filtercond.Or(
			filtercond.Equal(storage.ProviderFieldName, "p1"),
			filtercond.Equal(storage.ProviderFieldName, "p3"),
		),
	})
	if err != nil {
		t.Fatalf("SearchProviders with Or failed: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 providers (p1, p3), got %d", len(result))
	}
}

func TestSearchProviders_ExtraCond_Between(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	for _, p := range []struct {
		name     string
		priority int
	}{
		{"p1", 1}, {"p3", 3}, {"p5", 5}, {"p7", 7}, {"p10", 10},
	} {
		_ = store.AddProvider(ctx, &account.ProviderInfo{
			ProviderType: "test",
			ProviderName: p.name,
			Status:       account.ProviderStatusActive,
			Priority:     p.priority,
		})
	}

	result, err := store.SearchProviders(ctx, &storage.SearchFilter{
		ExtraCond: filtercond.Between(storage.ProviderFieldPriority, 3, 7),
	})
	if err != nil {
		t.Fatalf("SearchProviders with Between failed: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 providers with priority 3-7, got %d", len(result))
	}
}

func TestSearchProviders_ExtraCond_SupportedModel_Equal(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType:    "openai",
		ProviderName:    "p1",
		Status:          account.ProviderStatusActive,
		SupportedModels: []string{"gpt-4", "gpt-3.5"},
	})
	_ = store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType:    "openai",
		ProviderName:    "p2",
		Status:          account.ProviderStatusActive,
		SupportedModels: []string{"claude-3"},
	})

	result, err := store.SearchProviders(ctx, &storage.SearchFilter{
		ExtraCond: filtercond.Equal(storage.ProviderFieldSupportedModel, "gpt-4"),
	})
	if err != nil {
		t.Fatalf("SearchProviders with SupportedModel Equal failed: %v", err)
	}
	if len(result) != 1 || result[0].ProviderName != "p1" {
		t.Fatalf("expected only p1, got %d results", len(result))
	}
}

func TestSearchProviders_ExtraCond_SupportedModel_In(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType:    "openai",
		ProviderName:    "p1",
		Status:          account.ProviderStatusActive,
		SupportedModels: []string{"gpt-4"},
	})
	_ = store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType:    "openai",
		ProviderName:    "p2",
		Status:          account.ProviderStatusActive,
		SupportedModels: []string{"claude-3"},
	})
	_ = store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType:    "openai",
		ProviderName:    "p3",
		Status:          account.ProviderStatusActive,
		SupportedModels: []string{"gemini-pro"},
	})

	result, err := store.SearchProviders(ctx, &storage.SearchFilter{
		ExtraCond: filtercond.In(storage.ProviderFieldSupportedModel, "gpt-4", "claude-3"),
	})
	if err != nil {
		t.Fatalf("SearchProviders with SupportedModel In failed: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 providers (p1, p2), got %d", len(result))
	}
}

func TestSearchProviders_ExtraCond_UnsupportedOperator_Error(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_, err := store.SearchProviders(ctx, &storage.SearchFilter{
		ExtraCond: &filtercond.Filter{
			Field:    storage.ProviderFieldName,
			Operator: "bad_op",
			Value:    "x",
		},
	})
	if err == nil {
		t.Error("expected error for unsupported operator")
	}
}

func TestSearchProviders_ExtraCond_UnsupportedField_Error(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_, err := store.SearchProviders(ctx, &storage.SearchFilter{
		ExtraCond: filtercond.Equal("unsupported_field_xyz", "value"),
	})
	if err == nil {
		t.Error("expected error for unsupported provider field")
	}
}

func TestSearchProviders_ExtraCond_NotIn(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "typeA", ProviderName: "p1", Status: account.ProviderStatusActive})
	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "typeB", ProviderName: "p2", Status: account.ProviderStatusActive})
	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "typeC", ProviderName: "p3", Status: account.ProviderStatusActive})

	result, err := store.SearchProviders(ctx, &storage.SearchFilter{
		ExtraCond: filtercond.NotIn(storage.ProviderFieldName, "p1", "p2"),
	})
	if err != nil {
		t.Fatalf("SearchProviders with NotIn failed: %v", err)
	}
	if len(result) != 1 || result[0].ProviderName != "p3" {
		t.Fatalf("expected only p3, got %d results", len(result))
	}
}
