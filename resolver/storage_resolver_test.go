package resolver

import (
	"context"
	"testing"

	"github.com/nomand-zc/lumin-acpool/account"
	storememory "github.com/nomand-zc/lumin-acpool/storage/memory"
)

func setupResolver() (Resolver, *storememory.Store, *storememory.Store) {
	store := storememory.NewStore()
	r := NewStorageResolver(store, store)
	return r, store, store
}

func addProvider(ctx context.Context, ps *storememory.Store, provType, name string, status account.ProviderStatus, priority int, models []string) {
	_ = ps.AddProvider(ctx, &account.ProviderInfo{
		ProviderType:    provType,
		ProviderName:    name,
		Status:          status,
		Priority:        priority,
		SupportedModels: models,
	})
}

func addAccount(ctx context.Context, as *storememory.Store, id, provType, provName string, status account.Status, priority int, tags map[string]string) {
	_ = as.AddAccount(ctx, &account.Account{
		ID:           id,
		ProviderType: provType,
		ProviderName: provName,
		Status:       status,
		Priority:     priority,
		Tags:         tags,
	})
}

func TestResolveProvider_Success(t *testing.T) {
	ctx := context.Background()
	r, ps, as := setupResolver()

	addProvider(ctx, ps, "kiro", "team-a", account.ProviderStatusActive, 5, []string{"gpt-4", "gpt-3.5"})
	addAccount(ctx, as, "acc-1", "kiro", "team-a", account.StatusAvailable, 5, nil)

	prov, err := r.ResolveProvider(ctx, account.BuildProviderKey("kiro", "team-a"), "gpt-4")
	if err != nil {
		t.Fatalf("ResolveProvider failed: %v", err)
	}
	if prov.ProviderName != "team-a" {
		t.Fatalf("expected team-a, got %s", prov.ProviderName)
	}
}

func TestResolveProvider_NotFound(t *testing.T) {
	ctx := context.Background()
	r, _, _ := setupResolver()

	_, err := r.ResolveProvider(ctx, account.BuildProviderKey("kiro", "nonexistent"), "gpt-4")
	if err != ErrProviderNotFound {
		t.Fatalf("expected ErrProviderNotFound, got %v", err)
	}
}

func TestResolveProvider_Inactive(t *testing.T) {
	ctx := context.Background()
	r, ps, _ := setupResolver()

	addProvider(ctx, ps, "kiro", "team-a", account.ProviderStatusDisabled, 5, []string{"gpt-4"})

	_, err := r.ResolveProvider(ctx, account.BuildProviderKey("kiro", "team-a"), "gpt-4")
	if err != ErrProviderInactive {
		t.Fatalf("expected ErrProviderInactive, got %v", err)
	}
}

func TestResolveProvider_ModelNotSupported(t *testing.T) {
	ctx := context.Background()
	r, ps, _ := setupResolver()

	addProvider(ctx, ps, "kiro", "team-a", account.ProviderStatusActive, 5, []string{"gpt-4"})

	_, err := r.ResolveProvider(ctx, account.BuildProviderKey("kiro", "team-a"), "claude-3")
	if err != ErrModelNotSupported {
		t.Fatalf("expected ErrModelNotSupported, got %v", err)
	}
}

func TestResolveProvider_FillsAccountCounts(t *testing.T) {
	ctx := context.Background()
	r, ps, as := setupResolver()

	addProvider(ctx, ps, "kiro", "team-a", account.ProviderStatusActive, 5, []string{"gpt-4"})
	addAccount(ctx, as, "acc-1", "kiro", "team-a", account.StatusAvailable, 5, nil)
	addAccount(ctx, as, "acc-2", "kiro", "team-a", account.StatusCoolingDown, 3, nil)
	addAccount(ctx, as, "acc-3", "kiro", "team-a", account.StatusAvailable, 8, nil)

	prov, _ := r.ResolveProvider(ctx, account.BuildProviderKey("kiro", "team-a"), "gpt-4")

	if prov.AccountCount != 3 {
		t.Fatalf("expected AccountCount=3, got %d", prov.AccountCount)
	}
	if prov.AvailableAccountCount != 2 {
		t.Fatalf("expected AvailableAccountCount=2, got %d", prov.AvailableAccountCount)
	}
}

func TestResolveProviders_ByModel(t *testing.T) {
	ctx := context.Background()
	r, ps, as := setupResolver()

	addProvider(ctx, ps, "kiro", "team-a", account.ProviderStatusActive, 5, []string{"gpt-4"})
	addProvider(ctx, ps, "kiro", "team-b", account.ProviderStatusActive, 3, []string{"gpt-3.5"})
	addProvider(ctx, ps, "openai", "default", account.ProviderStatusActive, 8, []string{"gpt-4", "gpt-3.5"})
	// 为支持 gpt-4 的两个 provider 添加可用账号
	addAccount(ctx, as, "acc-1", "kiro", "team-a", account.StatusAvailable, 5, nil)
	addAccount(ctx, as, "acc-2", "openai", "default", account.StatusAvailable, 5, nil)

	providers, err := r.ResolveProviders(ctx, "gpt-4", "")
	if err != nil {
		t.Fatalf("ResolveProviders failed: %v", err)
	}
	if len(providers) != 2 {
		t.Fatalf("expected 2 providers supporting gpt-4, got %d", len(providers))
	}
}

func TestResolveProviders_ByTypeAndModel(t *testing.T) {
	ctx := context.Background()
	r, ps, as := setupResolver()

	addProvider(ctx, ps, "kiro", "team-a", account.ProviderStatusActive, 5, []string{"gpt-4"})
	addProvider(ctx, ps, "kiro", "team-b", account.ProviderStatusActive, 3, []string{"gpt-4"})
	addProvider(ctx, ps, "openai", "default", account.ProviderStatusActive, 8, []string{"gpt-4"})
	// 为所有 provider 添加可用账号
	addAccount(ctx, as, "acc-1", "kiro", "team-a", account.StatusAvailable, 5, nil)
	addAccount(ctx, as, "acc-2", "kiro", "team-b", account.StatusAvailable, 5, nil)
	addAccount(ctx, as, "acc-3", "openai", "default", account.StatusAvailable, 5, nil)

	providers, err := r.ResolveProviders(ctx, "gpt-4", "kiro")
	if err != nil {
		t.Fatalf("ResolveProviders failed: %v", err)
	}
	if len(providers) != 2 {
		t.Fatalf("expected 2 kiro providers, got %d", len(providers))
	}
}

func TestResolveProviders_ExcludesDisabled(t *testing.T) {
	ctx := context.Background()
	r, ps, as := setupResolver()

	addProvider(ctx, ps, "kiro", "team-a", account.ProviderStatusActive, 5, []string{"gpt-4"})
	addProvider(ctx, ps, "kiro", "team-b", account.ProviderStatusDisabled, 3, []string{"gpt-4"})
	// 只需为 active 的 provider 添加可用账号
	addAccount(ctx, as, "acc-1", "kiro", "team-a", account.StatusAvailable, 5, nil)

	providers, _ := r.ResolveProviders(ctx, "gpt-4", "")
	if len(providers) != 1 {
		t.Fatalf("expected 1 active provider, got %d", len(providers))
	}
}

func TestResolveProviders_IncludesDegraded(t *testing.T) {
	ctx := context.Background()
	r, ps, as := setupResolver()

	addProvider(ctx, ps, "kiro", "team-a", account.ProviderStatusActive, 5, []string{"gpt-4"})
	addProvider(ctx, ps, "kiro", "team-b", account.ProviderStatusDegraded, 3, []string{"gpt-4"})
	// 为两个 provider 都添加可用账号
	addAccount(ctx, as, "acc-1", "kiro", "team-a", account.StatusAvailable, 5, nil)
	addAccount(ctx, as, "acc-2", "kiro", "team-b", account.StatusAvailable, 5, nil)

	providers, _ := r.ResolveProviders(ctx, "gpt-4", "")
	if len(providers) != 2 {
		t.Fatalf("expected 2 providers (active + degraded), got %d", len(providers))
	}
}

func TestResolveAccounts_Available(t *testing.T) {
	ctx := context.Background()
	r, _, as := setupResolver()

	addAccount(ctx, as, "acc-1", "kiro", "team-a", account.StatusAvailable, 5, nil)
	addAccount(ctx, as, "acc-2", "kiro", "team-a", account.StatusCoolingDown, 3, nil)
	addAccount(ctx, as, "acc-3", "kiro", "team-a", account.StatusAvailable, 8, nil)

	accounts, err := r.ResolveAccounts(ctx, ResolveAccountsRequest{
		Key: account.BuildProviderKey("kiro", "team-a"),
	})
	if err != nil {
		t.Fatalf("ResolveAccounts failed: %v", err)
	}
	if len(accounts) != 2 {
		t.Fatalf("expected 2 available accounts, got %d", len(accounts))
	}
}

func TestResolveAccounts_WithExcludeIDs(t *testing.T) {
	ctx := context.Background()
	r, _, as := setupResolver()

	addAccount(ctx, as, "acc-1", "kiro", "team-a", account.StatusAvailable, 5, nil)
	addAccount(ctx, as, "acc-2", "kiro", "team-a", account.StatusAvailable, 3, nil)
	addAccount(ctx, as, "acc-3", "kiro", "team-a", account.StatusAvailable, 8, nil)

	accounts, _ := r.ResolveAccounts(ctx, ResolveAccountsRequest{
		Key:        account.BuildProviderKey("kiro", "team-a"),
		ExcludeIDs: []string{"acc-1", "acc-3"},
	})
	if len(accounts) != 1 {
		t.Fatalf("expected 1 account (after excluding 2), got %d", len(accounts))
	}
	if accounts[0].ID != "acc-2" {
		t.Fatalf("expected acc-2, got %s", accounts[0].ID)
	}
}

func TestResolveAccounts_WithTags(t *testing.T) {
	ctx := context.Background()
	r, _, as := setupResolver()

	addAccount(ctx, as, "acc-1", "kiro", "team-a", account.StatusAvailable, 5, map[string]string{"env": "prod"})
	addAccount(ctx, as, "acc-2", "kiro", "team-a", account.StatusAvailable, 3, map[string]string{"env": "test"})
	addAccount(ctx, as, "acc-3", "kiro", "team-a", account.StatusAvailable, 8, map[string]string{"env": "prod", "region": "us"})

	accounts, _ := r.ResolveAccounts(ctx, ResolveAccountsRequest{
		Key:  account.BuildProviderKey("kiro", "team-a"),
		Tags: map[string]string{"env": "prod"},
	})
	if len(accounts) != 2 {
		t.Fatalf("expected 2 accounts with env=prod, got %d", len(accounts))
	}
}

func TestResolveAccounts_EmptyResult(t *testing.T) {
	ctx := context.Background()
	r, _, as := setupResolver()

	addAccount(ctx, as, "acc-1", "kiro", "team-a", account.StatusCoolingDown, 5, nil)

	accounts, err := r.ResolveAccounts(ctx, ResolveAccountsRequest{
		Key: account.BuildProviderKey("kiro", "team-a"),
	})
	if err != nil {
		t.Fatalf("ResolveAccounts failed: %v", err)
	}
	if len(accounts) != 0 {
		t.Fatalf("expected 0 accounts (all cooling down), got %d", len(accounts))
	}
}

// --- Fix-11: ResolveProviders 空结果时返回 nil, nil ---

// TestResolveProviders_EmptyResult_ReturnsNilError 无匹配供应商时返回 nil, nil
func TestResolveProviders_EmptyResult_ReturnsNilError(t *testing.T) {
	ctx := context.Background()
	r, _, _ := setupResolver()

	// 没有任何 Provider，ResolveProviders 应返回 nil, nil
	providers, err := r.ResolveProviders(ctx, "gpt-4", "")
	if err != nil {
		t.Fatalf("expected nil error when no providers found, got %v", err)
	}
	if providers != nil {
		t.Fatalf("expected nil providers slice, got %v", providers)
	}
}

// TestResolveProviders_AllInactive_ReturnsNilError 所有 Provider 不可用时返回 nil, nil
func TestResolveProviders_AllInactive_ReturnsNilError(t *testing.T) {
	ctx := context.Background()
	r, ps, _ := setupResolver()

	// 添加两个禁用 Provider
	addProvider(ctx, ps, "kiro", "team-a", account.ProviderStatusDisabled, 5, []string{"gpt-4"})
	addProvider(ctx, ps, "kiro", "team-b", account.ProviderStatusDisabled, 3, []string{"gpt-4"})

	providers, err := r.ResolveProviders(ctx, "gpt-4", "")
	if err != nil {
		t.Fatalf("expected nil error when all providers inactive, got %v", err)
	}
	if providers != nil {
		t.Fatalf("expected nil providers slice when all inactive, got %v", providers)
	}
}

// TestResolveProviders_NoAvailableAccounts_ReturnsNilError 有 Provider 但无可用账号时返回 nil, nil
func TestResolveProviders_NoAvailableAccounts_ReturnsNilError(t *testing.T) {
	ctx := context.Background()
	r, ps, as := setupResolver()

	addProvider(ctx, ps, "kiro", "team-a", account.ProviderStatusActive, 5, []string{"gpt-4"})
	// 只有 CoolingDown 账号（不算 Available）
	addAccount(ctx, as, "acc-1", "kiro", "team-a", account.StatusCoolingDown, 5, nil)

	providers, err := r.ResolveProviders(ctx, "gpt-4", "")
	if err != nil {
		t.Fatalf("expected nil error when no available accounts, got %v", err)
	}
	if providers != nil {
		t.Fatalf("expected nil providers when no available accounts, got %v", providers)
	}
}

// TestResolveProviders_NilSliceNotEmptySlice 返回的是 nil 而不是空切片
func TestResolveProviders_NilSliceNotEmptySlice(t *testing.T) {
	ctx := context.Background()
	r, _, _ := setupResolver()

	result, err := r.ResolveProviders(ctx, "nonexistent-model", "")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	// 严格验证返回 nil（不是空 slice）
	if result != nil {
		t.Fatalf("expected nil (not empty slice), got len=%d", len(result))
	}
}

// --- Fix-6: filterExcluded 使用 map 实现 ---

// TestFilterExcluded_EmptyExcludeList 空排除列表返回原切片
func TestFilterExcluded_EmptyExcludeList(t *testing.T) {
	accounts := []*account.Account{
		{ID: "acc-1"},
		{ID: "acc-2"},
	}
	result := filterExcluded(accounts, nil)
	if len(result) != 2 {
		t.Fatalf("expected 2 accounts with empty exclude list, got %d", len(result))
	}
}

// TestFilterExcluded_ExcludesSome 排除指定 ID
func TestFilterExcluded_ExcludesSome(t *testing.T) {
	accounts := []*account.Account{
		{ID: "acc-1"},
		{ID: "acc-2"},
		{ID: "acc-3"},
	}
	result := filterExcluded(accounts, []string{"acc-1", "acc-3"})
	if len(result) != 1 {
		t.Fatalf("expected 1 account after excluding acc-1 and acc-3, got %d", len(result))
	}
	if result[0].ID != "acc-2" {
		t.Fatalf("expected acc-2, got %s", result[0].ID)
	}
}

// TestFilterExcluded_ExcludesAll 排除所有返回空切片
func TestFilterExcluded_ExcludesAll(t *testing.T) {
	accounts := []*account.Account{
		{ID: "acc-1"},
		{ID: "acc-2"},
	}
	result := filterExcluded(accounts, []string{"acc-1", "acc-2"})
	if len(result) != 0 {
		t.Fatalf("expected 0 accounts after excluding all, got %d", len(result))
	}
}

// TestFilterExcluded_NonExistentIDs 排除不存在的 ID 不影响结果
func TestFilterExcluded_NonExistentIDs(t *testing.T) {
	accounts := []*account.Account{
		{ID: "acc-1"},
		{ID: "acc-2"},
	}
	result := filterExcluded(accounts, []string{"acc-nonexistent"})
	if len(result) != 2 {
		t.Fatalf("expected 2 accounts when excluding non-existent ID, got %d", len(result))
	}
}
