package memory

import (
	"context"
	"errors"
	"testing"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
)

func TestAddProvider_Success(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	err := store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType: "openai",
		ProviderName: "default",
		Status:       account.ProviderStatusActive,
	})
	if err != nil {
		t.Fatalf("AddProvider failed: %v", err)
	}

	prov, err := store.GetProvider(ctx, account.BuildProviderKey("openai", "default"))
	if err != nil {
		t.Fatalf("GetProvider failed: %v", err)
	}
	if prov.ProviderType != "openai" || prov.ProviderName != "default" {
		t.Fatalf("unexpected provider: %+v", prov)
	}
}

func TestAddProvider_Duplicate(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	info := &account.ProviderInfo{
		ProviderType: "openai",
		ProviderName: "default",
		Status:       account.ProviderStatusActive,
	}
	_ = store.AddProvider(ctx, info)

	err := store.AddProvider(ctx, info)
	if !errors.Is(err, storage.ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}
}

func TestGetProvider_NotFound(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_, err := store.GetProvider(ctx, account.BuildProviderKey("nonexistent", "ghost"))
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGetProvider_FillsAccountCounts(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType: "openai",
		ProviderName: "default",
		Status:       account.ProviderStatusActive,
	})

	// 添加 2 个 Available，1 个 CoolingDown
	for i, status := range []account.Status{
		account.StatusAvailable,
		account.StatusAvailable,
		account.StatusCoolingDown,
	} {
		_ = store.AddAccount(ctx, &account.Account{
			ID:           string(rune('a' + i)),
			ProviderType: "openai",
			ProviderName: "default",
			Status:       status,
		})
	}

	prov, err := store.GetProvider(ctx, account.BuildProviderKey("openai", "default"))
	if err != nil {
		t.Fatalf("GetProvider failed: %v", err)
	}
	if prov.AccountCount != 3 {
		t.Errorf("expected AccountCount=3, got %d", prov.AccountCount)
	}
	if prov.AvailableAccountCount != 2 {
		t.Errorf("expected AvailableAccountCount=2, got %d", prov.AvailableAccountCount)
	}
}

func TestSearchProviders_ByType(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "typeA", ProviderName: "p1", Status: account.ProviderStatusActive})
	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "typeA", ProviderName: "p2", Status: account.ProviderStatusActive})
	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "typeB", ProviderName: "p3", Status: account.ProviderStatusActive})

	result, err := store.SearchProviders(ctx, &storage.SearchFilter{ProviderType: "typeA"})
	if err != nil {
		t.Fatalf("SearchProviders failed: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 providers of typeA, got %d", len(result))
	}
	for _, p := range result {
		if p.ProviderType != "typeA" {
			t.Errorf("expected typeA, got %s", p.ProviderType)
		}
	}
}

func TestSearchProviders_ByModel(t *testing.T) {
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
		SupportedModels: []string{"gpt-3.5"},
	})

	result, err := store.SearchProviders(ctx, &storage.SearchFilter{SupportedModel: "gpt-4"})
	if err != nil {
		t.Fatalf("SearchProviders failed: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 provider supporting gpt-4, got %d", len(result))
	}
	if result[0].ProviderName != "p1" {
		t.Errorf("expected p1, got %s", result[0].ProviderName)
	}
}

func TestSearchProviders_Empty(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "typeA", ProviderName: "p1", Status: account.ProviderStatusActive})

	result, err := store.SearchProviders(ctx, &storage.SearchFilter{ProviderType: "nonexistent"})
	if err != nil {
		t.Fatalf("SearchProviders failed: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected empty result, got %d providers", len(result))
	}
}

func TestUpdateProvider_Success(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType: "openai",
		ProviderName: "default",
		Status:       account.ProviderStatusActive,
	})

	err := store.UpdateProvider(ctx, &account.ProviderInfo{
		ProviderType:    "openai",
		ProviderName:    "default",
		Status:          account.ProviderStatusDisabled,
		SupportedModels: []string{"gpt-4"},
	})
	if err != nil {
		t.Fatalf("UpdateProvider failed: %v", err)
	}

	prov, _ := store.GetProvider(ctx, account.BuildProviderKey("openai", "default"))
	if prov.Status != account.ProviderStatusDisabled {
		t.Errorf("expected ProviderStatusDisabled, got %v", prov.Status)
	}
	if len(prov.SupportedModels) != 1 || prov.SupportedModels[0] != "gpt-4" {
		t.Errorf("expected SupportedModels=[gpt-4], got %v", prov.SupportedModels)
	}
}

func TestUpdateProvider_VersionConflict(t *testing.T) {
	// provider.go 里 UpdateProvider 没有版本检查（与 Account 不同），
	// 但更新不存在的 provider 应该返回 ErrNotFound。
	ctx := context.Background()
	store := NewStore()

	err := store.UpdateProvider(ctx, &account.ProviderInfo{
		ProviderType: "openai",
		ProviderName: "ghost",
	})
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing provider, got %v", err)
	}
}

func TestRemoveProviders_ByType(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "typeA", ProviderName: "p1", Status: account.ProviderStatusActive})
	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "typeB", ProviderName: "p2", Status: account.ProviderStatusActive})

	err := store.RemoveProvider(ctx, account.BuildProviderKey("typeA", "p1"))
	if err != nil {
		t.Fatalf("RemoveProvider failed: %v", err)
	}

	result, _ := store.SearchProviders(ctx, &storage.SearchFilter{ProviderType: "typeA"})
	if len(result) != 0 {
		t.Fatalf("expected 0 providers of typeA after removal, got %d", len(result))
	}

	// typeB 不受影响
	result, _ = store.SearchProviders(ctx, &storage.SearchFilter{ProviderType: "typeB"})
	if len(result) != 1 {
		t.Fatalf("expected typeB provider to remain, got %d", len(result))
	}
}

func TestRemoveProvider_NotFound(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	err := store.RemoveProvider(ctx, account.BuildProviderKey("nonexistent", "ghost"))
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestRemoveProvider_CascadeDeletesAccounts(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType: "openai",
		ProviderName: "default",
		Status:       account.ProviderStatusActive,
	})
	_ = store.AddAccount(ctx, &account.Account{
		ID:           "acc-cascade",
		ProviderType: "openai",
		ProviderName: "default",
		Status:       account.StatusAvailable,
	})

	_ = store.RemoveProvider(ctx, account.BuildProviderKey("openai", "default"))

	_, err := store.GetAccount(ctx, "acc-cascade")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected account to be cascade-deleted, got %v", err)
	}
}
