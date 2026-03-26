package occupancy

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	storememory "github.com/nomand-zc/lumin-acpool/storage/memory"
	"github.com/nomand-zc/lumin-acpool/usagetracker"
	"github.com/nomand-zc/lumin-client/usagerule"
)

// --- Fix-4: calculateLimit 只调一次 GetTrackedUsages ---

// trackingUsageTracker 包装真实 UsageTracker 并追踪 GetTrackedUsages 调用次数
type trackingUsageTracker struct {
	inner            usagetracker.UsageTracker
	getTrackedCalls  atomic.Int64
}

func newTrackingUsageTracker(inner usagetracker.UsageTracker) *trackingUsageTracker {
	return &trackingUsageTracker{inner: inner}
}

func (t *trackingUsageTracker) GetTrackedUsages(ctx context.Context, accountID string) ([]*account.TrackedUsage, error) {
	t.getTrackedCalls.Add(1)
	return t.inner.GetTrackedUsages(ctx, accountID)
}

func (t *trackingUsageTracker) RecordUsage(ctx context.Context, accountID string, sourceType usagerule.SourceType, amount float64) error {
	return t.inner.RecordUsage(ctx, accountID, sourceType, amount)
}

func (t *trackingUsageTracker) IsQuotaAvailable(ctx context.Context, accountID string) (bool, error) {
	return t.inner.IsQuotaAvailable(ctx, accountID)
}

func (t *trackingUsageTracker) Calibrate(ctx context.Context, accountID string, stats []*usagerule.UsageStats) error {
	return t.inner.Calibrate(ctx, accountID, stats)
}

func (t *trackingUsageTracker) CalibrateFromResponse(ctx context.Context, accountID string, sourceType usagerule.SourceType) error {
	return t.inner.CalibrateFromResponse(ctx, accountID, sourceType)
}

func (t *trackingUsageTracker) MinRemainRatio(ctx context.Context, accountID string) (float64, error) {
	return t.inner.MinRemainRatio(ctx, accountID)
}

func (t *trackingUsageTracker) InitRules(ctx context.Context, accountID string, rules []*usagerule.UsageRule) error {
	return t.inner.InitRules(ctx, accountID, rules)
}

func (t *trackingUsageTracker) Remove(ctx context.Context, accountID string) error {
	return t.inner.Remove(ctx, accountID)
}

// TestCalculateLimit_OnlyCallsGetTrackedUsagesOnce 验证 calculateLimit 只调一次 GetTrackedUsages
func TestCalculateLimit_OnlyCallsGetTrackedUsagesOnce(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	innerTracker := usagetracker.NewUsageTracker(usagetracker.WithUsageStore(store))

	// 初始化用量规则
	windowEnd := time.Now().Add(time.Hour)
	rules := []*usagerule.UsageRule{
		{
			SourceType:      usagerule.SourceTypeRequest,
			Total:           1000,
			TimeGranularity: usagerule.GranularityHour,
			WindowSize:      1,
		},
	}
	_ = innerTracker.InitRules(ctx, "acc-1", rules)
	// 手动设置 WindowEnd
	_ = store.CalibrateRule(ctx, "acc-1", 0, &account.TrackedUsage{
		RemoteUsed:   0,
		RemoteRemain: 1000,
		WindowEnd:    &windowEnd,
	})

	tracker := newTrackingUsageTracker(innerTracker)
	al := NewAdaptiveLimit(tracker)

	acct := &account.Account{ID: "acc-1"}

	// 调用 calculateLimit，验证 GetTrackedUsages 只被调用一次
	limit := al.calculateLimit(ctx, acct)
	if limit < 1 {
		t.Fatalf("expected limit >= 1, got %d", limit)
	}

	callCount := tracker.getTrackedCalls.Load()
	if callCount != 1 {
		t.Fatalf("expected GetTrackedUsages to be called exactly once, got %d times", callCount)
	}
}

// TestCalculateLimit_NoRules_FallbackLimit 无用量规则时返回 fallbackLimit
func TestCalculateLimit_NoRules_FallbackLimit(t *testing.T) {
	ctx := context.Background()
	innerTracker := usagetracker.NewUsageTracker()

	tracker := newTrackingUsageTracker(innerTracker)
	al := NewAdaptiveLimit(tracker, WithFallbackLimit(3))

	acct := &account.Account{ID: "acc-no-rules"}

	limit := al.calculateLimit(ctx, acct)
	if limit != 3 {
		t.Fatalf("expected fallbackLimit=3, got %d", limit)
	}

	// GetTrackedUsages 只调用一次
	callCount := tracker.getTrackedCalls.Load()
	if callCount != 1 {
		t.Fatalf("expected GetTrackedUsages called once, got %d", callCount)
	}
}

// TestCalculateLimit_WithMultipleRules_OnlyOneQuery 多规则时仍只调一次 GetTrackedUsages
func TestCalculateLimit_WithMultipleRules_OnlyOneQuery(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	innerTracker := usagetracker.NewUsageTracker(usagetracker.WithUsageStore(store))

	windowEnd := time.Now().Add(time.Hour)
	rules := []*usagerule.UsageRule{
		{
			SourceType:      usagerule.SourceTypeRequest,
			Total:           1000,
			TimeGranularity: usagerule.GranularityHour,
			WindowSize:      1,
		},
		{
			SourceType:      usagerule.SourceTypeToken,
			Total:           100000,
			TimeGranularity: usagerule.GranularityDay,
			WindowSize:      1,
		},
	}
	_ = innerTracker.InitRules(ctx, "acc-1", rules)
	_ = store.CalibrateRule(ctx, "acc-1", 0, &account.TrackedUsage{
		RemoteUsed:   0,
		RemoteRemain: 1000,
		WindowEnd:    &windowEnd,
	})
	_ = store.CalibrateRule(ctx, "acc-1", 1, &account.TrackedUsage{
		RemoteUsed:   0,
		RemoteRemain: 100000,
		WindowEnd:    &windowEnd,
	})

	tracker := newTrackingUsageTracker(innerTracker)
	al := NewAdaptiveLimit(tracker)

	acct := &account.Account{ID: "acc-1"}
	al.calculateLimit(ctx, acct)

	callCount := tracker.getTrackedCalls.Load()
	if callCount != 1 {
		t.Fatalf("expected exactly 1 GetTrackedUsages call for multi-rule account, got %d", callCount)
	}
}

// TestComputeMinRemainRatio_Empty 空切片返回 1.0
func TestComputeMinRemainRatio_Empty(t *testing.T) {
	ratio := computeMinRemainRatio(nil)
	if ratio != 1.0 {
		t.Fatalf("expected 1.0 for empty usages, got %f", ratio)
	}
}

// TestComputeMinRemainRatio_SingleUsage 单条用量返回正确比例
func TestComputeMinRemainRatio_SingleUsage(t *testing.T) {
	usages := []*account.TrackedUsage{
		{
			Rule:         &usagerule.UsageRule{Total: 100},
			RemoteRemain: 70,
			RemoteUsed:   30,
			LocalUsed:    0,
		},
	}
	ratio := computeMinRemainRatio(usages)
	// RemainRatio = EstimatedRemain / Total = 70 / 100 = 0.7
	if ratio < 0.69 || ratio > 0.71 {
		t.Fatalf("expected ratio ~0.7, got %f", ratio)
	}
}

// TestComputeMinRemainRatio_MultipleUsages_ReturnsMinimum 多条用量返回最小比例
func TestComputeMinRemainRatio_MultipleUsages_ReturnsMinimum(t *testing.T) {
	usages := []*account.TrackedUsage{
		{
			Rule:         &usagerule.UsageRule{Total: 100},
			RemoteRemain: 80,
			RemoteUsed:   20,
		},
		{
			Rule:         &usagerule.UsageRule{Total: 100},
			RemoteRemain: 30,
			RemoteUsed:   70,
		},
	}
	ratio := computeMinRemainRatio(usages)
	// 最小 ratio = 30/100 = 0.3
	if ratio < 0.29 || ratio > 0.31 {
		t.Fatalf("expected min ratio ~0.3, got %f", ratio)
	}
}

// TestAdaptiveLimit_FilterAvailable_OnlyOneGetTrackedUsagesPerAccount
// FilterAvailable 对每个账号调用 calculateLimit，每个 calculateLimit 只查一次 GetTrackedUsages
func TestAdaptiveLimit_FilterAvailable_OnlyOneGetTrackedUsagesPerAccount(t *testing.T) {
	ctx := context.Background()
	innerTracker := usagetracker.NewUsageTracker()
	tracker := newTrackingUsageTracker(innerTracker)

	al := NewAdaptiveLimit(tracker, WithFallbackLimit(5))

	accounts := []*account.Account{
		{ID: "acc-1"},
		{ID: "acc-2"},
		{ID: "acc-3"},
	}

	result := al.FilterAvailable(ctx, accounts)
	if len(result) != 3 {
		t.Fatalf("expected 3 available accounts, got %d", len(result))
	}

	// 3 个账号，每个 calculateLimit 调用一次 GetTrackedUsages，共 3 次
	callCount := tracker.getTrackedCalls.Load()
	if callCount != 3 {
		t.Fatalf("expected 3 GetTrackedUsages calls (one per account), got %d", callCount)
	}
}

// TestAdaptiveLimit_Acquire_ExhaustedQuota_ReturnsMinLimit 配额耗尽时返回 minLimit
func TestAdaptiveLimit_Acquire_ExhaustedQuota_ReturnsMinLimit(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	innerTracker := usagetracker.NewUsageTracker(usagetracker.WithUsageStore(store))

	// 初始化规则但配额已耗尽（RemoteRemain=0, LocalUsed=0）
	rules := []*usagerule.UsageRule{
		{
			SourceType:      usagerule.SourceTypeRequest,
			Total:           100,
			TimeGranularity: usagerule.GranularityHour,
			WindowSize:      1,
		},
	}
	_ = innerTracker.InitRules(ctx, "acc-1", rules)
	_ = store.CalibrateRule(ctx, "acc-1", 0, &account.TrackedUsage{
		RemoteUsed:   100,
		RemoteRemain: 0,
	})

	al := NewAdaptiveLimit(innerTracker, WithMinLimit(1))
	acct := &account.Account{ID: "acc-1"}

	limit := al.calculateLimit(ctx, acct)
	// 配额耗尽，返回 minLimit=1
	if limit != 1 {
		t.Fatalf("expected minLimit=1 when quota exhausted, got %d", limit)
	}
}

// TestAdaptiveLimit_MaxLimit_Respected 最大并发上限被应用
func TestAdaptiveLimit_MaxLimit_Respected(t *testing.T) {
	ctx := context.Background()
	store := storememory.NewStore()
	innerTracker := usagetracker.NewUsageTracker(usagetracker.WithUsageStore(store))

	windowEnd := time.Now().Add(time.Hour)
	rules := []*usagerule.UsageRule{
		{
			SourceType:      usagerule.SourceTypeRequest,
			Total:           10000,
			TimeGranularity: usagerule.GranularityHour,
			WindowSize:      1,
		},
	}
	_ = innerTracker.InitRules(ctx, "acc-1", rules)
	_ = store.CalibrateRule(ctx, "acc-1", 0, &account.TrackedUsage{
		RemoteUsed:   0,
		RemoteRemain: 10000,
		WindowEnd:    &windowEnd,
	})

	// 设置 MaxLimit=10
	al := NewAdaptiveLimit(innerTracker, WithMaxLimit(10), WithFactor(100.0))

	acct := &account.Account{ID: "acc-1"}
	limit := al.calculateLimit(ctx, acct)
	if limit > 10 {
		t.Fatalf("expected limit <= 10 (maxLimit), got %d", limit)
	}
}
