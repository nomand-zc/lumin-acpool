package usagetracker

import (
	"context"
	"testing"
	"time"

	"github.com/nomand-zc/lumin-client/usagerule"
)

// TestCalibrate_UpdateExistingRules 验证 Calibrate 更新已有规则（原子校准路径）
func TestCalibrate_UpdateExistingRules(t *testing.T) {
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

	// 先记录一些本地用量
	_ = ut.RecordUsage(ctx, "acc-1", usagerule.SourceTypeRequest, 5.0)

	now := time.Now()
	start := now.Add(-12 * time.Hour)
	end := now.Add(12 * time.Hour)

	stats := []*usagerule.UsageStats{
		{
			Rule: &usagerule.UsageRule{
				SourceType:      usagerule.SourceTypeRequest,
				Total:           100,
				TimeGranularity: usagerule.GranularityDay,
				WindowSize:      1,
			},
			Used:      20,
			Remain:    80,
			StartTime: &start,
			EndTime:   &end,
		},
	}

	if err := ut.Calibrate(ctx, "acc-1", stats); err != nil {
		t.Fatalf("Calibrate (update existing rules) failed: %v", err)
	}

	usages, _ := ut.GetTrackedUsages(ctx, "acc-1")
	if len(usages) != 1 {
		t.Fatalf("expected 1 usage after calibration, got %d", len(usages))
	}
	// 校准后 RemoteUsed 更新为 20，RemoteRemain 更新为 80
	if usages[0].RemoteUsed != 20 {
		t.Errorf("expected RemoteUsed=20 after calibration, got %f", usages[0].RemoteUsed)
	}
	if usages[0].RemoteRemain != 80 {
		t.Errorf("expected RemoteRemain=80 after calibration, got %f", usages[0].RemoteRemain)
	}
	// 校准后 LocalUsed 重置为 0
	if usages[0].LocalUsed != 0 {
		t.Errorf("expected LocalUsed=0 after calibration (CalibrateRule resets it), got %f", usages[0].LocalUsed)
	}
}

// TestCalibrate_AddNewRules 验证 Calibrate 新增规则（hasNewRules 路径）
func TestCalibrate_AddNewRules(t *testing.T) {
	ctx := context.Background()
	ut := NewUsageTracker()

	// 先初始化一条规则
	rules := []*usagerule.UsageRule{
		{
			SourceType:      usagerule.SourceTypeRequest,
			Total:           100,
			TimeGranularity: usagerule.GranularityDay,
			WindowSize:      1,
		},
	}
	_ = ut.InitRules(ctx, "acc-1", rules)

	now := time.Now()
	start := now.Add(-6 * time.Hour)
	end := now.Add(6 * time.Hour)

	// 传入两条 stats：一条匹配已有规则，一条是新规则（Token 类型）
	stats := []*usagerule.UsageStats{
		{
			Rule: &usagerule.UsageRule{
				SourceType:      usagerule.SourceTypeRequest,
				Total:           100,
				TimeGranularity: usagerule.GranularityDay,
				WindowSize:      1,
			},
			Used:      30,
			Remain:    70,
			StartTime: &start,
			EndTime:   &end,
		},
		{
			Rule: &usagerule.UsageRule{
				SourceType:      usagerule.SourceTypeToken,
				Total:           100000,
				TimeGranularity: usagerule.GranularityDay,
				WindowSize:      1,
			},
			Used:      5000,
			Remain:    95000,
			StartTime: &start,
			EndTime:   &end,
		},
	}

	if err := ut.Calibrate(ctx, "acc-1", stats); err != nil {
		t.Fatalf("Calibrate (add new rules) failed: %v", err)
	}

	usages, _ := ut.GetTrackedUsages(ctx, "acc-1")
	if len(usages) != 2 {
		t.Fatalf("expected 2 usages after adding new rule, got %d", len(usages))
	}
}

// TestCalibrate_NilStatsIgnored 验证 nil stats 条目被跳过
func TestCalibrate_NilStatsIgnored(t *testing.T) {
	ctx := context.Background()
	ut := NewUsageTracker()

	// 无初始化，传入含 nil 条目的 stats（初始化路径）
	now := time.Now()
	start := now.Add(-time.Hour)
	end := now.Add(time.Hour)

	stats := []*usagerule.UsageStats{
		nil,
		{
			Rule: &usagerule.UsageRule{
				SourceType:      usagerule.SourceTypeRequest,
				Total:           100,
				TimeGranularity: usagerule.GranularityDay,
				WindowSize:      1,
			},
			Used:      10,
			Remain:    90,
			StartTime: &start,
			EndTime:   &end,
		},
	}

	if err := ut.Calibrate(ctx, "acc-init", stats); err != nil {
		t.Fatalf("Calibrate with nil stats entry failed: %v", err)
	}

	usages, _ := ut.GetTrackedUsages(ctx, "acc-init")
	if len(usages) != 1 {
		t.Fatalf("expected 1 usage (nil skipped), got %d", len(usages))
	}
}

// TestCalibrate_ExistingRulesNilRuleInStats 有已有规则时跳过 nil Rule 的 stats 条目
func TestCalibrate_ExistingRulesNilRuleInStats(t *testing.T) {
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

	// 传入 nil Rule 条目（现有规则路径下跳过 nil）
	stats := []*usagerule.UsageStats{
		nil,
		{Rule: nil, Used: 10, Remain: 90},
	}

	if err := ut.Calibrate(ctx, "acc-1", stats); err != nil {
		t.Fatalf("Calibrate with nil rule stats failed: %v", err)
	}
}

// TestIsQuotaAvailable_RuleWithZeroTotal 规则 Total<=0 时跳过，默认可用
func TestIsQuotaAvailable_RuleWithZeroTotal(t *testing.T) {
	ctx := context.Background()
	ut := NewUsageTracker()

	rules := []*usagerule.UsageRule{
		{
			SourceType:      usagerule.SourceTypeRequest,
			Total:           0, // Total=0 跳过检查
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
		t.Fatal("expected quota to be available when Total=0 (skip check)")
	}
}

// TestMinRemainRatio_MultipleRules 多规则时返回最小剩余比例
func TestMinRemainRatio_MultipleRules(t *testing.T) {
	ctx := context.Background()
	ut := NewUsageTracker()

	rules := []*usagerule.UsageRule{
		{
			SourceType:      usagerule.SourceTypeRequest,
			Total:           100,
			TimeGranularity: usagerule.GranularityDay,
			WindowSize:      1,
		},
		{
			SourceType:      usagerule.SourceTypeToken,
			Total:           1000,
			TimeGranularity: usagerule.GranularityDay,
			WindowSize:      1,
		},
	}
	_ = ut.InitRules(ctx, "acc-1", rules)

	// 消耗 Request 50%，Token 10%
	for i := 0; i < 50; i++ {
		_ = ut.RecordUsage(ctx, "acc-1", usagerule.SourceTypeRequest, 1.0)
	}
	for i := 0; i < 100; i++ {
		_ = ut.RecordUsage(ctx, "acc-1", usagerule.SourceTypeToken, 1.0)
	}

	ratio, err := ut.MinRemainRatio(ctx, "acc-1")
	if err != nil {
		t.Fatalf("MinRemainRatio failed: %v", err)
	}
	// Request ratio=50/100=0.5, Token ratio=900/1000=0.9 => min=0.5
	if ratio < 0.49 || ratio > 0.51 {
		t.Errorf("expected MinRemainRatio ~0.5, got %f", ratio)
	}
}

// TestInitRules_InvalidRuleSkipped 无效规则被跳过
func TestInitRules_InvalidRuleSkipped(t *testing.T) {
	ctx := context.Background()
	ut := NewUsageTracker()

	rules := []*usagerule.UsageRule{
		nil, // nil 规则被跳过
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

	usages, _ := ut.GetTrackedUsages(ctx, "acc-1")
	if len(usages) != 1 {
		t.Fatalf("expected 1 usage (nil rule skipped), got %d", len(usages))
	}
}
