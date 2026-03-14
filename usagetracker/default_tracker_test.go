package usagetracker

import (
	"context"
	"testing"
	"time"

	storeMemory "github.com/nomand-zc/lumin-acpool/storage/memory"
	"github.com/nomand-zc/lumin-client/usagerule"
)

func TestNewUsageTracker_DefaultStore(t *testing.T) {
	ut := NewUsageTracker()
	if ut == nil {
		t.Fatal("expected non-nil UsageTracker")
	}
}

func TestInitRules(t *testing.T) {
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

	if err := ut.InitRules(ctx, "acc-1", rules); err != nil {
		t.Fatalf("InitRules failed: %v", err)
	}

	usages, err := ut.GetTrackedUsages(ctx, "acc-1")
	if err != nil {
		t.Fatalf("GetTrackedUsages failed: %v", err)
	}
	if len(usages) != 1 {
		t.Fatalf("expected 1 tracked usage, got %d", len(usages))
	}
	if usages[0].RemoteRemain != 100 {
		t.Fatalf("expected RemoteRemain=100, got %f", usages[0].RemoteRemain)
	}
}

func TestRecordUsage_IncrLocalUsed(t *testing.T) {
	ctx := context.Background()
	store := storeMemory.NewStore()
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

	// 记录 3 次使用
	for i := 0; i < 3; i++ {
		if err := ut.RecordUsage(ctx, "acc-1", usagerule.SourceTypeRequest, 1.0); err != nil {
			t.Fatalf("RecordUsage failed: %v", err)
		}
	}

	usages, _ := ut.GetTrackedUsages(ctx, "acc-1")
	if usages[0].LocalUsed != 3.0 {
		t.Fatalf("expected LocalUsed=3.0, got %f", usages[0].LocalUsed)
	}
}

func TestRecordUsage_NoRules_Silent(t *testing.T) {
	ctx := context.Background()
	ut := NewUsageTracker()

	// 未初始化规则，应静默忽略
	err := ut.RecordUsage(ctx, "acc-no-rules", usagerule.SourceTypeRequest, 1.0)
	if err != nil {
		t.Fatalf("expected nil error for account without rules, got: %v", err)
	}
}

func TestRecordUsage_QuotaExhaustedCallback(t *testing.T) {
	ctx := context.Background()

	callbackCalled := false
	var callbackAccountID string

	ut := NewUsageTracker(
		WithSafetyRatio(0.9),
		WithCallback(QuotaExhaustedCallback(func(ctx context.Context, accountID string, rule *usagerule.UsageRule) {
			callbackCalled = true
			callbackAccountID = accountID
		})),
	)

	rules := []*usagerule.UsageRule{
		{
			SourceType:      usagerule.SourceTypeRequest,
			Total:           10,
			TimeGranularity: usagerule.GranularityDay,
			WindowSize:      1,
		},
	}
	_ = ut.InitRules(ctx, "acc-1", rules)

	// 记录 9 次使用（RemoteRemain=10, LocalUsed=9, EstimatedUsed=9, ratio=9/10=0.9 >= 0.9）
	for i := 0; i < 9; i++ {
		_ = ut.RecordUsage(ctx, "acc-1", usagerule.SourceTypeRequest, 1.0)
	}

	if !callbackCalled {
		t.Fatal("expected quota exhausted callback to be called")
	}
	if callbackAccountID != "acc-1" {
		t.Fatalf("expected callback for acc-1, got %s", callbackAccountID)
	}
}

func TestIsQuotaAvailable_Available(t *testing.T) {
	ctx := context.Background()
	ut := NewUsageTracker(WithSafetyRatio(0.95))

	rules := []*usagerule.UsageRule{
		{
			SourceType:      usagerule.SourceTypeRequest,
			Total:           100,
			TimeGranularity: usagerule.GranularityDay,
			WindowSize:      1,
		},
	}
	_ = ut.InitRules(ctx, "acc-1", rules)

	available, err := ut.IsQuotaAvailable(ctx, "acc-1")
	if err != nil {
		t.Fatalf("IsQuotaAvailable failed: %v", err)
	}
	if !available {
		t.Fatal("expected quota to be available")
	}
}

func TestIsQuotaAvailable_Exhausted(t *testing.T) {
	ctx := context.Background()
	ut := NewUsageTracker(WithSafetyRatio(0.9))

	rules := []*usagerule.UsageRule{
		{
			SourceType:      usagerule.SourceTypeRequest,
			Total:           10,
			TimeGranularity: usagerule.GranularityDay,
			WindowSize:      1,
		},
	}
	_ = ut.InitRules(ctx, "acc-1", rules)

	// 记录 9 次使用
	for i := 0; i < 9; i++ {
		_ = ut.RecordUsage(ctx, "acc-1", usagerule.SourceTypeRequest, 1.0)
	}

	available, err := ut.IsQuotaAvailable(ctx, "acc-1")
	if err != nil {
		t.Fatalf("IsQuotaAvailable failed: %v", err)
	}
	if available {
		t.Fatal("expected quota to be unavailable")
	}
}

func TestIsQuotaAvailable_NoRules(t *testing.T) {
	ctx := context.Background()
	ut := NewUsageTracker()

	// 没有规则的账号默认可用
	available, err := ut.IsQuotaAvailable(ctx, "acc-no-rules")
	if err != nil {
		t.Fatalf("IsQuotaAvailable failed: %v", err)
	}
	if !available {
		t.Fatal("expected quota to be available when no rules exist")
	}
}

func TestCalibrate_InitFromStats(t *testing.T) {
	ctx := context.Background()
	ut := NewUsageTracker()

	now := time.Now()
	start := now.Add(-30 * time.Minute)
	end := now.Add(30 * time.Minute)

	stats := []*usagerule.UsageStats{
		{
			Rule: &usagerule.UsageRule{
				SourceType:      usagerule.SourceTypeRequest,
				Total:           100,
				TimeGranularity: usagerule.GranularityHour,
				WindowSize:      1,
			},
			Used:      50,
			Remain:    50,
			StartTime: &start,
			EndTime:   &end,
		},
	}

	if err := ut.Calibrate(ctx, "acc-1", stats); err != nil {
		t.Fatalf("Calibrate failed: %v", err)
	}

	usages, _ := ut.GetTrackedUsages(ctx, "acc-1")
	if len(usages) != 1 {
		t.Fatalf("expected 1 usage, got %d", len(usages))
	}
	if usages[0].RemoteUsed != 50 {
		t.Fatalf("expected RemoteUsed=50, got %f", usages[0].RemoteUsed)
	}
	if usages[0].RemoteRemain != 50 {
		t.Fatalf("expected RemoteRemain=50, got %f", usages[0].RemoteRemain)
	}
}

func TestCalibrateFromResponse_MarkExhausted(t *testing.T) {
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

	if err := ut.CalibrateFromResponse(ctx, "acc-1", usagerule.SourceTypeRequest); err != nil {
		t.Fatalf("CalibrateFromResponse failed: %v", err)
	}

	usages, _ := ut.GetTrackedUsages(ctx, "acc-1")
	if usages[0].EstimatedRemain() > 0 {
		t.Fatalf("expected EstimatedRemain <= 0 after CalibrateFromResponse, got %f", usages[0].EstimatedRemain())
	}
}

func TestMinRemainRatio_NoRules(t *testing.T) {
	ctx := context.Background()
	ut := NewUsageTracker()

	ratio, err := ut.MinRemainRatio(ctx, "acc-no-rules")
	if err != nil {
		t.Fatalf("MinRemainRatio failed: %v", err)
	}
	if ratio != 1.0 {
		t.Fatalf("expected 1.0 for account without rules, got %f", ratio)
	}
}

func TestMinRemainRatio_WithUsage(t *testing.T) {
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

	// 使用 30 次
	for i := 0; i < 30; i++ {
		_ = ut.RecordUsage(ctx, "acc-1", usagerule.SourceTypeRequest, 1.0)
	}

	ratio, err := ut.MinRemainRatio(ctx, "acc-1")
	if err != nil {
		t.Fatalf("MinRemainRatio failed: %v", err)
	}
	// RemoteRemain=100, LocalUsed=30, EstimatedRemain=70, ratio=70/100=0.7
	if ratio < 0.69 || ratio > 0.71 {
		t.Fatalf("expected ratio ~0.7, got %f", ratio)
	}
}

func TestRemove(t *testing.T) {
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

	if err := ut.Remove(ctx, "acc-1"); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	usages, _ := ut.GetTrackedUsages(ctx, "acc-1")
	if len(usages) != 0 {
		t.Fatalf("expected 0 usages after Remove, got %d", len(usages))
	}
}
