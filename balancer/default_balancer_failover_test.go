package balancer

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/balancer/occupancy"
	"github.com/nomand-zc/lumin-acpool/resolver"
	storememory "github.com/nomand-zc/lumin-acpool/storage/memory"
)

// countingResolver 包装真实的 Resolver，统计 ResolveAccounts 的调用次数（按 ProviderKey 分类）。
type countingResolver struct {
	inner  resolver.Resolver
	counts map[account.ProviderKey]*atomic.Int64
}

func newCountingResolver(inner resolver.Resolver, keys []account.ProviderKey) *countingResolver {
	counts := make(map[account.ProviderKey]*atomic.Int64, len(keys))
	for _, k := range keys {
		var c atomic.Int64
		counts[k] = &c
	}
	return &countingResolver{inner: inner, counts: counts}
}

func (r *countingResolver) ResolveProvider(ctx context.Context, key account.ProviderKey, model string) (*account.ProviderInfo, error) {
	return r.inner.ResolveProvider(ctx, key, model)
}

func (r *countingResolver) ResolveProviders(ctx context.Context, model string, providerType string) ([]*account.ProviderInfo, error) {
	return r.inner.ResolveProviders(ctx, model, providerType)
}

func (r *countingResolver) ResolveAccounts(ctx context.Context, req resolver.ResolveAccountsRequest) ([]*account.Account, error) {
	if c, ok := r.counts[req.Key]; ok {
		c.Add(1)
	}
	return r.inner.ResolveAccounts(ctx, req)
}

func (r *countingResolver) callCount(key account.ProviderKey) int64 {
	if c, ok := r.counts[key]; ok {
		return c.Load()
	}
	return 0
}

// blockingOccupancyController 可按 ProviderKey 阻止 FilterAvailable（返回空），
// 用于模拟某个 Provider 下所有账号占用满的场景。
type blockingOccupancyController struct {
	blockedProviderTypes map[string]bool // providerType → true 表示该类型下的账号全部过滤掉
}

func (c *blockingOccupancyController) FilterAvailable(_ context.Context, accounts []*account.Account) []*account.Account {
	var result []*account.Account
	for _, acct := range accounts {
		if !c.blockedProviderTypes[acct.ProviderType] {
			result = append(result, acct)
		}
	}
	return result
}

func (c *blockingOccupancyController) Acquire(_ context.Context, _ *account.Account) bool {
	return true
}

func (c *blockingOccupancyController) Release(_ context.Context, _ string) {}

// --- 测试用例 ---

// TestPickAuto_FailoverCachesAccounts 验证 failover 时同一 Provider 的账号列表只查询一次（缓存命中）。
// 场景：Provider-A 占用满（FilterAvailable 返回空），failover 到 Provider-B 成功。
// 期望：Provider-A 的 ResolveAccounts 只被调用 1 次（缓存）。
func TestPickAuto_FailoverCachesAccounts(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()

	keyA := account.ProviderKey{Type: "provider-a", Name: "instance-1"}
	keyB := account.ProviderKey{Type: "provider-b", Name: "instance-1"}

	// 注册 Provider-A 和 Provider-B
	_ = store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType:          keyA.Type,
		ProviderName:          keyA.Name,
		Status:                account.ProviderStatusActive,
		SupportedModels:       []string{"gpt-4"},
		AccountCount:          2,
		AvailableAccountCount: 2,
	})
	_ = store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType:          keyB.Type,
		ProviderName:          keyB.Name,
		Status:                account.ProviderStatusActive,
		SupportedModels:       []string{"gpt-4"},
		AccountCount:          2,
		AvailableAccountCount: 2,
	})

	// Provider-A 下的账号
	_ = store.AddAccount(ctx, &account.Account{
		ID:           "acct-a-1",
		ProviderType: keyA.Type,
		ProviderName: keyA.Name,
		Status:       account.StatusAvailable,
	})
	_ = store.AddAccount(ctx, &account.Account{
		ID:           "acct-a-2",
		ProviderType: keyA.Type,
		ProviderName: keyA.Name,
		Status:       account.StatusAvailable,
	})

	// Provider-B 下的账号
	_ = store.AddAccount(ctx, &account.Account{
		ID:           "acct-b-1",
		ProviderType: keyB.Type,
		ProviderName: keyB.Name,
		Status:       account.StatusAvailable,
	})
	_ = store.AddAccount(ctx, &account.Account{
		ID:           "acct-b-2",
		ProviderType: keyB.Type,
		ProviderName: keyB.Name,
		Status:       account.StatusAvailable,
	})

	// 包装 resolver 以统计调用次数
	innerResolver := resolver.NewStorageResolver(store, store)
	cResolver := newCountingResolver(innerResolver, []account.ProviderKey{keyA, keyB})

	// Provider-A 的账号全部占用满（模拟 OccupancyFull）
	occCtrl := &blockingOccupancyController{
		blockedProviderTypes: map[string]bool{keyA.Type: true},
	}

	// 使用固定顺序的 GroupSelector：总是返回第一个（Provider-A 优先）
	// 这里使用默认 GroupPriority（同 Priority=0，Shuffle 后顺序不确定），
	// 因此我们通过设置 Priority 来确保 Provider-A 先被选中。
	_ = store.UpdateProvider(ctx, &account.ProviderInfo{
		ProviderType:          keyA.Type,
		ProviderName:          keyA.Name,
		Status:                account.ProviderStatusActive,
		SupportedModels:       []string{"gpt-4"},
		Priority:              10, // 更高优先级，确保先被选中
		AccountCount:          2,
		AvailableAccountCount: 2,
	})

	b, err := New(
		WithAccountStorage(store),
		WithResolver(cResolver),
		WithOccupancyController(occCtrl),
		WithDefaultFailover(true),
		WithDefaultMaxRetries(0),
	)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	result, err := b.Pick(ctx, &PickRequest{
		Model:          "gpt-4",
		EnableFailover: true,
	})
	if err != nil {
		t.Fatalf("Pick() should succeed via failover to Provider-B, got: %v", err)
	}

	// 验证结果来自 Provider-B
	if result.Account.ProviderType != keyB.Type {
		t.Fatalf("expected account from provider-b, got provider type: %s", result.Account.ProviderType)
	}

	// 关键验证：Provider-A 的 ResolveAccounts 应只被调用 1 次（缓存命中）
	countA := cResolver.callCount(keyA)
	if countA != 1 {
		t.Fatalf("expected Provider-A ResolveAccounts called exactly 1 time (cache), got %d", countA)
	}

	// Provider-B 也只被调用 1 次
	countB := cResolver.callCount(keyB)
	if countB != 1 {
		t.Fatalf("expected Provider-B ResolveAccounts called exactly 1 time, got %d", countB)
	}
}

// TestPickAuto_FailoverNoRetryStaleAccounts 验证缓存中的账号列表每次仍经过 FilterAvailable 实时过滤。
// 场景：Provider-A 有 2 个账号，初次可用但第二次通过 FilterAvailable 动态过滤掉。
// 该测试确保 FilterAvailable 不被缓存跳过。
func TestPickAuto_FailoverNoRetryStaleAccounts(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()

	keyA := account.ProviderKey{Type: "typeA", Name: "nameA"}

	_ = store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType:          keyA.Type,
		ProviderName:          keyA.Name,
		Status:                account.ProviderStatusActive,
		SupportedModels:       []string{"gpt-4"},
		AccountCount:          2,
		AvailableAccountCount: 2,
	})

	_ = store.AddAccount(ctx, &account.Account{
		ID:           "acct-1",
		ProviderType: keyA.Type,
		ProviderName: keyA.Name,
		Status:       account.StatusAvailable,
	})
	_ = store.AddAccount(ctx, &account.Account{
		ID:           "acct-2",
		ProviderType: keyA.Type,
		ProviderName: keyA.Name,
		Status:       account.StatusAvailable,
	})

	// 使用计数型占用控制器：第一次 FilterAvailable 返回账号，第二次返回空
	filterCallCount := 0
	_ = filterCallCount

	// 实现一个在第二次 FilterAvailable 时返回空的控制器
	filterCtrl := &filterCountingController{
		callCount:     0,
		blockAfterN:   1, // 第 1 次调用后，后续全部返回空
		inner:         occupancy.NewUnlimited(),
	}
	_ = filterCallCount // suppress unused warning

	innerResolver := resolver.NewStorageResolver(store, store)
	cResolver := newCountingResolver(innerResolver, []account.ProviderKey{keyA})

	b, err := New(
		WithAccountStorage(store),
		WithResolver(cResolver),
		WithOccupancyController(filterCtrl),
		WithDefaultMaxRetries(0),
	)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// 第一次 Pick：FilterAvailable 第 1 次调用，返回账号，应该成功
	result, err := b.Pick(ctx, &PickRequest{
		Model: "gpt-4",
	})
	if err != nil {
		t.Fatalf("first Pick() should succeed, got: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// 验证 ResolveAccounts 只被调用了 1 次（非缓存，这是第一次 Pick 的新请求）
	if cResolver.callCount(keyA) != 1 {
		t.Fatalf("expected 1 ResolveAccounts call, got %d", cResolver.callCount(keyA))
	}

	// 第二次 Pick（新请求，新缓存）：FilterAvailable 第 2 次调用，返回空
	_, err = b.Pick(ctx, &PickRequest{
		Model: "gpt-4",
	})
	// 应该返回 ErrOccupancyFull 或 ErrNoAvailableProvider（取决于 failover 是否开启）
	if err == nil {
		t.Fatal("second Pick() should fail when FilterAvailable returns empty")
	}

	// 验证 FilterAvailable 确实被调用了（不被缓存跳过）
	if filterCtrl.callCount < 2 {
		t.Fatalf("expected FilterAvailable called at least 2 times (once per Pick), got %d", filterCtrl.callCount)
	}
}

// filterCountingController 统计 FilterAvailable 调用次数，超过 blockAfterN 次后返回空。
type filterCountingController struct {
	inner       occupancy.Controller
	callCount   int
	blockAfterN int
}

func (c *filterCountingController) FilterAvailable(ctx context.Context, accounts []*account.Account) []*account.Account {
	c.callCount++
	if c.callCount > c.blockAfterN {
		return nil
	}
	return c.inner.FilterAvailable(ctx, accounts)
}

func (c *filterCountingController) Acquire(ctx context.Context, acct *account.Account) bool {
	return c.inner.Acquire(ctx, acct)
}

func (c *filterCountingController) Release(ctx context.Context, accountID string) {
	c.inner.Release(ctx, accountID)
}

// TestPickAuto_SingleProvider_CacheHitOnRetry 验证单 Provider 场景下缓存正常工作。
// 无 failover，单次请求 ResolveAccounts 只被调用 1 次。
func TestPickAuto_SingleProvider_CacheHitOnRetry(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()

	keyA := account.ProviderKey{Type: "typeA", Name: "nameA"}

	_ = store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType:          keyA.Type,
		ProviderName:          keyA.Name,
		Status:                account.ProviderStatusActive,
		SupportedModels:       []string{"gpt-4"},
		AccountCount:          3,
		AvailableAccountCount: 3,
	})

	for i := 1; i <= 3; i++ {
		_ = store.AddAccount(ctx, &account.Account{
			ID:           fmt.Sprintf("acct-%d", i),
			ProviderType: keyA.Type,
			ProviderName: keyA.Name,
			Status:       account.StatusAvailable,
		})
	}

	innerResolver := resolver.NewStorageResolver(store, store)
	cResolver := newCountingResolver(innerResolver, []account.ProviderKey{keyA})

	b, err := New(
		WithAccountStorage(store),
		WithResolver(cResolver),
		WithDefaultMaxRetries(2), // 允许重试，但 ResolveAccounts 应只调用 1 次
	)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	result, err := b.Pick(ctx, &PickRequest{
		Model: "gpt-4",
	})
	if err != nil {
		t.Fatalf("Pick() failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// 单次请求，ResolveAccounts 只调用 1 次
	if cResolver.callCount(keyA) != 1 {
		t.Fatalf("expected 1 ResolveAccounts call, got %d", cResolver.callCount(keyA))
	}
}
