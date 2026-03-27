package memory

import (
	"context"
	"errors"
	"testing"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
)

func TestGetAccount_Success(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "test", ProviderName: "p", Status: account.ProviderStatusActive})
	_ = store.AddAccount(ctx, &account.Account{
		ID:           "acct-1",
		ProviderType: "test",
		ProviderName: "p",
		Status:       account.StatusAvailable,
	})

	acct, err := store.GetAccount(ctx, "acct-1")
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if acct.ID != "acct-1" {
		t.Errorf("expected ID=acct-1, got %s", acct.ID)
	}
}

func TestGetAccount_NotFound(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_, err := store.GetAccount(ctx, "no-such-id")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestAddAccount_Success(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "test", ProviderName: "p", Status: account.ProviderStatusActive})

	err := store.AddAccount(ctx, &account.Account{
		ID:           "new-acct",
		ProviderType: "test",
		ProviderName: "p",
		Status:       account.StatusAvailable,
	})
	if err != nil {
		t.Fatalf("AddAccount failed: %v", err)
	}

	acct, err := store.GetAccount(ctx, "new-acct")
	if err != nil {
		t.Fatalf("GetAccount after add failed: %v", err)
	}
	if acct.Version != 1 {
		t.Errorf("expected Version=1, got %d", acct.Version)
	}
}

func TestAddAccount_Duplicate(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "test", ProviderName: "p", Status: account.ProviderStatusActive})

	acct := &account.Account{
		ID:           "dup-acct",
		ProviderType: "test",
		ProviderName: "p",
		Status:       account.StatusAvailable,
	}
	_ = store.AddAccount(ctx, acct)

	err := store.AddAccount(ctx, acct)
	if !errors.Is(err, storage.ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}
}

func TestSearchAccounts_ByProviderType(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "typeA", ProviderName: "p1", Status: account.ProviderStatusActive})
	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "typeB", ProviderName: "p2", Status: account.ProviderStatusActive})

	_ = store.AddAccount(ctx, &account.Account{ID: "a1", ProviderType: "typeA", ProviderName: "p1", Status: account.StatusAvailable})
	_ = store.AddAccount(ctx, &account.Account{ID: "a2", ProviderType: "typeA", ProviderName: "p1", Status: account.StatusAvailable})
	_ = store.AddAccount(ctx, &account.Account{ID: "b1", ProviderType: "typeB", ProviderName: "p2", Status: account.StatusAvailable})

	result, err := store.SearchAccounts(ctx, &storage.SearchFilter{ProviderType: "typeA"})
	if err != nil {
		t.Fatalf("SearchAccounts failed: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 accounts of typeA, got %d", len(result))
	}
	for _, a := range result {
		if a.ProviderType != "typeA" {
			t.Errorf("expected typeA, got %s", a.ProviderType)
		}
	}
}

func TestSearchAccounts_ByStatus(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "test", ProviderName: "p", Status: account.ProviderStatusActive})

	_ = store.AddAccount(ctx, &account.Account{ID: "avail-1", ProviderType: "test", ProviderName: "p", Status: account.StatusAvailable})
	_ = store.AddAccount(ctx, &account.Account{ID: "avail-2", ProviderType: "test", ProviderName: "p", Status: account.StatusAvailable})
	_ = store.AddAccount(ctx, &account.Account{ID: "cool-1", ProviderType: "test", ProviderName: "p", Status: account.StatusCoolingDown})

	result, err := store.SearchAccounts(ctx, &storage.SearchFilter{
		ProviderType: "test",
		ProviderName: "p",
		Status:       int(account.StatusAvailable),
	})
	if err != nil {
		t.Fatalf("SearchAccounts failed: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 Available accounts, got %d", len(result))
	}
}

func TestSearchAccounts_ByProviderName(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "test", ProviderName: "team-a", Status: account.ProviderStatusActive})
	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "test", ProviderName: "team-b", Status: account.ProviderStatusActive})

	_ = store.AddAccount(ctx, &account.Account{ID: "ta-1", ProviderType: "test", ProviderName: "team-a", Status: account.StatusAvailable})
	_ = store.AddAccount(ctx, &account.Account{ID: "tb-1", ProviderType: "test", ProviderName: "team-b", Status: account.StatusAvailable})

	result, err := store.SearchAccounts(ctx, &storage.SearchFilter{
		ProviderType: "test",
		ProviderName: "team-a",
	})
	if err != nil {
		t.Fatalf("SearchAccounts failed: %v", err)
	}
	if len(result) != 1 || result[0].ID != "ta-1" {
		t.Fatalf("expected only ta-1, got %d results", len(result))
	}
}

func TestUpdateAccount_Status(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "test", ProviderName: "p", Status: account.ProviderStatusActive})
	_ = store.AddAccount(ctx, &account.Account{
		ID:           "upd-acct",
		ProviderType: "test",
		ProviderName: "p",
		Status:       account.StatusAvailable,
	})

	acct, _ := store.GetAccount(ctx, "upd-acct")
	acct.Status = account.StatusCoolingDown

	err := store.UpdateAccount(ctx, acct, storage.UpdateFieldStatus)
	if err != nil {
		t.Fatalf("UpdateAccount failed: %v", err)
	}

	updated, _ := store.GetAccount(ctx, "upd-acct")
	if updated.Status != account.StatusCoolingDown {
		t.Errorf("expected StatusCoolingDown, got %v", updated.Status)
	}
	// AvailableAccountCount 应减少
	prov, _ := store.GetProvider(ctx, account.BuildProviderKey("test", "p"))
	if prov.AvailableAccountCount != 0 {
		t.Errorf("expected AvailableAccountCount=0 after status change, got %d", prov.AvailableAccountCount)
	}
}

func TestUpdateAccount_VersionConflict(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "test", ProviderName: "p", Status: account.ProviderStatusActive})
	_ = store.AddAccount(ctx, &account.Account{
		ID:           "ver-acct",
		ProviderType: "test",
		ProviderName: "p",
		Status:       account.StatusAvailable,
	})

	// 使用错误版本号
	acct := &account.Account{
		ID:           "ver-acct",
		ProviderType: "test",
		ProviderName: "p",
		Status:       account.StatusCoolingDown,
		Version:      999, // 错误版本号
	}

	err := store.UpdateAccount(ctx, acct, storage.UpdateFieldStatus)
	if !errors.Is(err, storage.ErrVersionConflict) {
		t.Fatalf("expected ErrVersionConflict, got %v", err)
	}
}

func TestRemoveAccount_Success(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "test", ProviderName: "p", Status: account.ProviderStatusActive})
	_ = store.AddAccount(ctx, &account.Account{
		ID:           "rm-acct",
		ProviderType: "test",
		ProviderName: "p",
		Status:       account.StatusAvailable,
	})

	err := store.RemoveAccount(ctx, "rm-acct")
	if err != nil {
		t.Fatalf("RemoveAccount failed: %v", err)
	}

	_, err = store.GetAccount(ctx, "rm-acct")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after removal, got %v", err)
	}
}

func TestRemoveAccount_NotFound(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	err := store.RemoveAccount(ctx, "no-such-id")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestCountAccounts_Basic(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{ProviderType: "test", ProviderName: "p", Status: account.ProviderStatusActive})

	for i, id := range []string{"c1", "c2", "c3"} {
		status := account.StatusAvailable
		if i == 2 {
			status = account.StatusCoolingDown
		}
		_ = store.AddAccount(ctx, &account.Account{
			ID: id, ProviderType: "test", ProviderName: "p", Status: status,
		})
	}

	count, err := store.CountAccounts(ctx, &storage.SearchFilter{ProviderType: "test", ProviderName: "p"})
	if err != nil {
		t.Fatalf("CountAccounts failed: %v", err)
	}
	if count != 3 {
		t.Errorf("expected count=3, got %d", count)
	}

	count, err = store.CountAccounts(ctx, &storage.SearchFilter{Status: int(account.StatusAvailable)})
	if err != nil {
		t.Fatalf("CountAccounts failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected count=2 (Available), got %d", count)
	}
}
