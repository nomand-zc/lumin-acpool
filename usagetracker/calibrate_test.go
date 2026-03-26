package usagetracker

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/nomand-zc/lumin-acpool/account"
	storememory "github.com/nomand-zc/lumin-acpool/storage/memory"
	"github.com/nomand-zc/lumin-client/usagerule"
)

// trackingUsageStore 包装内存 UsageStore，追踪 SaveUsages 和 CalibrateRule 的调用次数
type trackingUsageStore struct {
	inner            *storememory.Store
	saveUsagesCalls  atomic.Int64
	calibrateRuleCalls atomic.Int64
}

func newTrackingUsageStore() *trackingUsageStore {
	return &trackingUsageStore{inner: storememory.NewStore()}
}

func (s *trackingUsageStore) GetCurrentUsages(ctx context.Context, accountID string) ([]*account.TrackedUsage, error) {
	return s.inner.GetCurrentUsages(ctx, accountID)
}

func (s *trackingUsageStore) SaveUsages(ctx context.Context, accountID string, usages []*account.TrackedUsage) error {
	s.saveUsagesCalls.Add(1)
	return s.inner.SaveUsages(ctx, accountID, usages)
}

func (s *trackingUsageStore) IncrLocalUsed(ctx context.Context, accountID string, ruleIndex int, amount float64) error {
	return s.inner.IncrLocalUsed(ctx, accountID, ruleIndex, amount)
}

func (s *trackingUsageStore) CalibrateRule(ctx context.Context, accountID string, ruleIndex int, usage *account.TrackedUsage) error {
	s.calibrateRuleCalls.Add(1)
	return s.inner.CalibrateRule(ctx, accountID, ruleIndex, usage)
}

func (s *trackingUsageStore) RemoveUsages(ctx context.Context, accountID string) error {
	return s.inner.RemoveUsages(ctx, accountID)
}

// --- Fix-8: CalibrateFromResponse 使用 CalibrateRule 而非 SaveUsages ---

// TestCalibrateFromResponse_UsesCalibrateRule_NotSaveUsages 验证通过 CalibrateRule 而非 SaveUsages 更新
func TestCalibrateFromResponse_UsesCalibrateRule_NotSaveUsages(t *testing.T) {
	ctx := context.Background()
	store := newTrackingUsageStore()

	ut := NewUsageTracker(WithUsageStore(store))

	rules := []*usagerule.UsageRule{
		{
			SourceType:      usagerule.SourceTypeRequest,
			Total:           100,
			TimeGranularity: usagerule.GranularityDay,
			WindowSize:      1,
		},
	}

	// InitRules 会调用 SaveUsages（初始化），重置计数器
	_ = ut.InitRules(ctx, "acc-1", rules)
	store.saveUsagesCalls.Store(0) // 重置，只关注 CalibrateFromResponse 的行为

	// 调用 CalibrateFromResponse
	if err := ut.CalibrateFromResponse(ctx, "acc-1", usagerule.SourceTypeRequest); err != nil {
		t.Fatalf("CalibrateFromResponse failed: %v", err)
	}

	// 验证：CalibrateRule 被调用（原子更新）
	if store.calibrateRuleCalls.Load() == 0 {
		t.Fatal("expected CalibrateRule to be called, but it was not")
	}

	// 验证：SaveUsages 没有被调用（避免全量覆写并发增量）
	if store.saveUsagesCalls.Load() > 0 {
		t.Fatalf("expected SaveUsages NOT to be called, but was called %d time(s)", store.saveUsagesCalls.Load())
	}
}

// TestCalibrateFromResponse_SetsRemoteRemainToZero 验证 CalibrateFromResponse 将 RemoteRemain 设为 0
func TestCalibrateFromResponse_SetsRemoteRemainToZero(t *testing.T) {
	ctx := context.Background()
	ut := NewUsageTracker()

	rules := []*usagerule.UsageRule{
		{
			SourceType:      usagerule.SourceTypeRequest,
			Total:           100,
			TimeGranularity: usagerule.GranularityDay,
			WindowSize:      1,
		},
	}
	_ = ut.InitRules(ctx, "acc-1", rules)

	// 记录一些用量
	_ = ut.RecordUsage(ctx, "acc-1", usagerule.SourceTypeRequest, 10.0)

	// 调用 CalibrateFromResponse 标记耗尽
	if err := ut.CalibrateFromResponse(ctx, "acc-1", usagerule.SourceTypeRequest); err != nil {
		t.Fatalf("CalibrateFromResponse failed: %v", err)
	}

	usages, _ := ut.GetTrackedUsages(ctx, "acc-1")
	if len(usages) == 0 {
		t.Fatal("expected usages to be non-empty")
	}

	// 验证 RemoteRemain = 0（耗尽标记）
	if usages[0].RemoteRemain != 0 {
		t.Fatalf("expected RemoteRemain=0 (exhausted), got %f", usages[0].RemoteRemain)
	}

	// 验证 EstimatedRemain <= 0
	if usages[0].EstimatedRemain() > 0 {
		t.Fatalf("expected EstimatedRemain <= 0 after marking exhausted, got %f", usages[0].EstimatedRemain())
	}
}

// TestCalibrateFromResponse_CalibrateRuleCalledPerMatchingRule 每条匹配规则调用一次 CalibrateRule
func TestCalibrateFromResponse_CalibrateRuleCalledPerMatchingRule(t *testing.T) {
	ctx := context.Background()
	store := newTrackingUsageStore()
	ut := NewUsageTracker(WithUsageStore(store))

	// 两条 Request 类型规则（不同粒度）
	rules := []*usagerule.UsageRule{
		{
			SourceType:      usagerule.SourceTypeRequest,
			Total:           100,
			TimeGranularity: usagerule.GranularityHour,
			WindowSize:      1,
		},
		{
			SourceType:      usagerule.SourceTypeRequest,
			Total:           1000,
			TimeGranularity: usagerule.GranularityDay,
			WindowSize:      1,
		},
	}
	_ = ut.InitRules(ctx, "acc-1", rules)
	store.saveUsagesCalls.Store(0)
	store.calibrateRuleCalls.Store(0)

	if err := ut.CalibrateFromResponse(ctx, "acc-1", usagerule.SourceTypeRequest); err != nil {
		t.Fatalf("CalibrateFromResponse failed: %v", err)
	}

	// 两条规则都匹配 SourceTypeRequest，应该各调用一次 CalibrateRule
	if store.calibrateRuleCalls.Load() != 2 {
		t.Fatalf("expected CalibrateRule called 2 times (one per matching rule), got %d", store.calibrateRuleCalls.Load())
	}
	// 不调用 SaveUsages
	if store.saveUsagesCalls.Load() > 0 {
		t.Fatalf("expected SaveUsages NOT called, got %d calls", store.saveUsagesCalls.Load())
	}
}

// TestCalibrateFromResponse_OnlyMatchingSourceType 只处理匹配 sourceType 的规则
func TestCalibrateFromResponse_OnlyMatchingSourceType(t *testing.T) {
	ctx := context.Background()
	store := newTrackingUsageStore()
	ut := NewUsageTracker(WithUsageStore(store))

	// 一条 Request 规则，一条 Token 规则
	rules := []*usagerule.UsageRule{
		{
			SourceType:      usagerule.SourceTypeRequest,
			Total:           100,
			TimeGranularity: usagerule.GranularityDay,
			WindowSize:      1,
		},
		{
			SourceType:      usagerule.SourceTypeToken,
			Total:           100000,
			TimeGranularity: usagerule.GranularityDay,
			WindowSize:      1,
		},
	}
	_ = ut.InitRules(ctx, "acc-1", rules)
	store.calibrateRuleCalls.Store(0)

	// 只 CalibrateFromResponse Request 类型
	if err := ut.CalibrateFromResponse(ctx, "acc-1", usagerule.SourceTypeRequest); err != nil {
		t.Fatalf("CalibrateFromResponse failed: %v", err)
	}

	// 只有 Request 规则被处理，CalibrateRule 调用 1 次
	if store.calibrateRuleCalls.Load() != 1 {
		t.Fatalf("expected 1 CalibrateRule call (only Request rule), got %d", store.calibrateRuleCalls.Load())
	}

	// Token 规则的 RemoteRemain 应该不变
	usages, _ := ut.GetTrackedUsages(ctx, "acc-1")
	var tokenUsage *account.TrackedUsage
	for _, u := range usages {
		if u.Rule != nil && u.Rule.SourceType == usagerule.SourceTypeToken {
			tokenUsage = u
			break
		}
	}
	if tokenUsage == nil {
		t.Fatal("expected Token usage to exist")
	}
	if tokenUsage.RemoteRemain == 0 {
		t.Fatal("Token usage RemoteRemain should not be zeroed when CalibrateFromResponse was for Request type")
	}
}

// TestCalibrateFromResponse_NoRules_ReturnsNil 无规则时静默返回 nil
func TestCalibrateFromResponse_NoRules_ReturnsNil(t *testing.T) {
	ctx := context.Background()
	ut := NewUsageTracker()

	err := ut.CalibrateFromResponse(ctx, "acc-no-rules", usagerule.SourceTypeRequest)
	if err != nil {
		t.Fatalf("expected nil for account without rules, got %v", err)
	}
}

// TestCalibrateFromResponse_PreservesLocalUsedAfterCalibration 校准后重置 LocalUsed（并发安全）
func TestCalibrateFromResponse_PreservesLocalUsedAfterCalibration(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	ut := NewUsageTracker(WithUsageStore(store))

	rules := []*usagerule.UsageRule{
		{
			SourceType:      usagerule.SourceTypeRequest,
			Total:           100,
			TimeGranularity: usagerule.GranularityDay,
			WindowSize:      1,
		},
	}
	_ = ut.InitRules(ctx, "acc-1", rules)

	// 记录一些本地用量
	_ = ut.RecordUsage(ctx, "acc-1", usagerule.SourceTypeRequest, 5.0)

	// CalibrateFromResponse 后，LocalUsed 应被重置（通过 CalibrateRule）
	if err := ut.CalibrateFromResponse(ctx, "acc-1", usagerule.SourceTypeRequest); err != nil {
		t.Fatalf("CalibrateFromResponse failed: %v", err)
	}

	usages, _ := ut.GetTrackedUsages(ctx, "acc-1")
	// CalibrateRule 重置 LocalUsed=0
	if usages[0].LocalUsed != 0 {
		t.Fatalf("expected LocalUsed=0 after CalibrateFromResponse (CalibrateRule resets it), got %f", usages[0].LocalUsed)
	}
}
