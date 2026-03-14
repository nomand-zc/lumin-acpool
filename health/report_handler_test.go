package health

import (
	"context"
	"testing"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/cooldown"
	storeMemory "github.com/nomand-zc/lumin-acpool/storage/memory"
	"github.com/nomand-zc/lumin-acpool/usagetracker"
	"github.com/nomand-zc/lumin-client/usagerule"
)

func statusPtr(s account.Status) *account.Status {
	return &s
}

func addTestAccount(ctx context.Context, store *storeMemory.Store, id string, status account.Status) *account.Account {
	acct := &account.Account{
		ID:           id,
		ProviderType: "test",
		ProviderName: "default",
		Status:       status,
		Priority:     5,
	}
	_ = store.AddAccount(ctx, acct)
	return acct
}

// --- 基础场景 ---

func TestReportCallback_NilReport(t *testing.T) {
	as := storeMemory.NewStore()
	cb := NewDefaultReportCallback(ReportHandlerDeps{
		AccountStorage: as,
	})

	// 应该不 panic
	cb(context.Background(), nil)
}

func TestReportCallback_EmptyResults(t *testing.T) {
	ctx := context.Background()
	as := storeMemory.NewStore()
	addTestAccount(ctx, as, "acc-1", account.StatusAvailable)

	cb := NewDefaultReportCallback(ReportHandlerDeps{
		AccountStorage: as,
	})

	cb(ctx, &HealthReport{
		AccountID: "acc-1",
		Results:   []*CheckResult{},
	})

	// 账号状态不应变化
	acct, _ := as.GetAccount(ctx, "acc-1")
	if acct.Status != account.StatusAvailable {
		t.Fatalf("expected status Available, got %v", acct.Status)
	}
}

func TestReportCallback_AccountNotFound(t *testing.T) {
	as := storeMemory.NewStore()
	cb := NewDefaultReportCallback(ReportHandlerDeps{
		AccountStorage: as,
	})

	// 不应 panic，应静默忽略
	cb(context.Background(), &HealthReport{
		AccountID: "nonexistent",
		Results: []*CheckResult{
			{
				CheckName:       "test",
				Status:          CheckFailed,
				SuggestedStatus: statusPtr(account.StatusDisabled),
			},
		},
	})
}

// --- SuggestedStatus 处理 ---

func TestReportCallback_SuggestedStatus_RecoverToAvailable(t *testing.T) {
	ctx := context.Background()
	as := storeMemory.NewStore()

	until := time.Now().Add(time.Hour)
	acct := &account.Account{
		ID:               "acc-1",
		ProviderType:     "test",
		ProviderName:     "default",
		Status:           account.StatusCircuitOpen,
		CircuitOpenUntil: &until,
		CooldownUntil:    &until,
	}
	_ = as.AddAccount(ctx, acct)

	cb := NewDefaultReportCallback(ReportHandlerDeps{
		AccountStorage: as,
	})

	cb(ctx, &HealthReport{
		AccountID: "acc-1",
		Results: []*CheckResult{
			{
				CheckName:       "recovery",
				Status:          CheckPassed,
				SuggestedStatus: statusPtr(account.StatusAvailable),
			},
		},
	})

	updated, _ := as.GetAccount(ctx, "acc-1")
	if updated.Status != account.StatusAvailable {
		t.Fatalf("expected status Available, got %v", updated.Status)
	}
	if updated.CircuitOpenUntil != nil {
		t.Fatal("expected CircuitOpenUntil to be nil after recovery")
	}
	if updated.CooldownUntil != nil {
		t.Fatal("expected CooldownUntil to be nil after recovery")
	}
}

func TestReportCallback_SuggestedStatus_SameStatusNoUpdate(t *testing.T) {
	ctx := context.Background()
	as := storeMemory.NewStore()
	addTestAccount(ctx, as, "acc-1", account.StatusAvailable)

	cb := NewDefaultReportCallback(ReportHandlerDeps{
		AccountStorage: as,
	})

	original, _ := as.GetAccount(ctx, "acc-1")
	originalTime := original.UpdatedAt

	cb(ctx, &HealthReport{
		AccountID: "acc-1",
		Results: []*CheckResult{
			{
				CheckName:       "test",
				Status:          CheckPassed,
				SuggestedStatus: statusPtr(account.StatusAvailable), // 和当前状态相同
			},
		},
	})

	updated, _ := as.GetAccount(ctx, "acc-1")
	if updated.UpdatedAt != originalTime {
		t.Fatal("expected no update when suggested status equals current status")
	}
}

func TestReportCallback_SuggestedStatus_CoolingDown_WithCooldownManager(t *testing.T) {
	ctx := context.Background()
	as := storeMemory.NewStore()
	addTestAccount(ctx, as, "acc-1", account.StatusAvailable)

	cm := cooldown.NewCooldownManager(cooldown.WithDefaultDuration(60 * time.Second))

	cb := NewDefaultReportCallback(ReportHandlerDeps{
		AccountStorage:  as,
		CooldownManager: cm,
	})

	cb(ctx, &HealthReport{
		AccountID: "acc-1",
		Results: []*CheckResult{
			{
				CheckName:       "usage_quota",
				Status:          CheckFailed,
				Severity:        SeverityCritical,
				SuggestedStatus: statusPtr(account.StatusCoolingDown),
			},
		},
	})

	updated, _ := as.GetAccount(ctx, "acc-1")
	if updated.Status != account.StatusCoolingDown {
		t.Fatalf("expected status CoolingDown, got %v", updated.Status)
	}
	if updated.CooldownUntil == nil {
		t.Fatal("expected CooldownUntil to be set by CooldownManager")
	}
}

func TestReportCallback_SuggestedStatus_CoolingDown_WithExplicitTime(t *testing.T) {
	ctx := context.Background()
	as := storeMemory.NewStore()
	addTestAccount(ctx, as, "acc-1", account.StatusAvailable)

	cm := cooldown.NewCooldownManager()
	until := time.Now().Add(5 * time.Minute)

	cb := NewDefaultReportCallback(ReportHandlerDeps{
		AccountStorage:  as,
		CooldownManager: cm,
	})

	cb(ctx, &HealthReport{
		AccountID: "acc-1",
		Results: []*CheckResult{
			{
				CheckName:       "credential_refresh",
				Status:          CheckFailed,
				Severity:        SeverityCritical,
				SuggestedStatus: statusPtr(account.StatusCoolingDown),
				Data: map[string]any{
					ReportDataKeyCooldownUntil: &until,
				},
			},
		},
	})

	updated, _ := as.GetAccount(ctx, "acc-1")
	if updated.Status != account.StatusCoolingDown {
		t.Fatalf("expected status CoolingDown, got %v", updated.Status)
	}
	if updated.CooldownUntil == nil {
		t.Fatal("expected CooldownUntil to be set")
	}
	if !updated.CooldownUntil.Equal(until) {
		t.Fatalf("expected CooldownUntil = %v, got %v", until, *updated.CooldownUntil)
	}
}

func TestReportCallback_SuggestedStatus_CoolingDown_NoCooldownManager(t *testing.T) {
	ctx := context.Background()
	as := storeMemory.NewStore()
	addTestAccount(ctx, as, "acc-1", account.StatusAvailable)

	until := time.Now().Add(5 * time.Minute)

	// 没有 CooldownManager，直接设置 CooldownUntil
	cb := NewDefaultReportCallback(ReportHandlerDeps{
		AccountStorage: as,
	})

	cb(ctx, &HealthReport{
		AccountID: "acc-1",
		Results: []*CheckResult{
			{
				CheckName:       "usage_quota",
				Status:          CheckFailed,
				Severity:        SeverityCritical,
				SuggestedStatus: statusPtr(account.StatusCoolingDown),
				Data: map[string]any{
					ReportDataKeyCooldownUntil: &until,
				},
			},
		},
	})

	updated, _ := as.GetAccount(ctx, "acc-1")
	if updated.Status != account.StatusCoolingDown {
		t.Fatalf("expected status CoolingDown, got %v", updated.Status)
	}
	if updated.CooldownUntil == nil {
		t.Fatal("expected CooldownUntil to be set")
	}
}

func TestReportCallback_SuggestedStatus_CoolingDown_SkipIfAlreadyCooling(t *testing.T) {
	ctx := context.Background()
	as := storeMemory.NewStore()

	// 账号已经在 CoolingDown 状态
	until := time.Now().Add(10 * time.Minute)
	acct := &account.Account{
		ID:            "acc-1",
		ProviderType:  "test",
		ProviderName:  "default",
		Status:        account.StatusCoolingDown,
		CooldownUntil: &until,
	}
	_ = as.AddAccount(ctx, acct)

	cm := cooldown.NewCooldownManager()
	cb := NewDefaultReportCallback(ReportHandlerDeps{
		AccountStorage:  as,
		CooldownManager: cm,
	})

	cb(ctx, &HealthReport{
		AccountID: "acc-1",
		Results: []*CheckResult{
			{
				CheckName:       "usage_quota",
				Status:          CheckFailed,
				Severity:        SeverityCritical,
				SuggestedStatus: statusPtr(account.StatusCoolingDown),
			},
		},
	})

	// handleCooldown 中：只对 Available 和 CircuitOpen 状态触发冷却
	// CoolingDown → CoolingDown 不会再次触发
	updated, _ := as.GetAccount(ctx, "acc-1")
	if updated.Status != account.StatusCoolingDown {
		t.Fatalf("expected status to remain CoolingDown, got %v", updated.Status)
	}
}

func TestReportCallback_SuggestedStatus_Banned(t *testing.T) {
	ctx := context.Background()
	as := storeMemory.NewStore()
	addTestAccount(ctx, as, "acc-1", account.StatusAvailable)

	cb := NewDefaultReportCallback(ReportHandlerDeps{
		AccountStorage: as,
	})

	cb(ctx, &HealthReport{
		AccountID: "acc-1",
		Results: []*CheckResult{
			{
				CheckName:       "credential_refresh",
				Status:          CheckFailed,
				Severity:        SeverityCritical,
				SuggestedStatus: statusPtr(account.StatusBanned),
			},
		},
	})

	updated, _ := as.GetAccount(ctx, "acc-1")
	if updated.Status != account.StatusBanned {
		t.Fatalf("expected status Banned, got %v", updated.Status)
	}
}

// --- UsageStats 校准 ---

func TestReportCallback_UsageStats_Calibrate(t *testing.T) {
	ctx := context.Background()
	as := storeMemory.NewStore()
	addTestAccount(ctx, as, "acc-1", account.StatusAvailable)

	ut := usagetracker.NewUsageTracker()
	// 初始化规则
	_ = ut.InitRules(ctx, "acc-1", []*usagerule.UsageRule{
		{
			SourceType:      usagerule.SourceTypeRequest,
			Total:           100,
			TimeGranularity: usagerule.GranularityDay,
			WindowSize:      1,
		},
	})

	// 本地记录 10 次使用
	for i := 0; i < 10; i++ {
		_ = ut.RecordUsage(ctx, "acc-1", usagerule.SourceTypeRequest, 1.0)
	}

	cb := NewDefaultReportCallback(ReportHandlerDeps{
		AccountStorage: as,
		UsageTracker:   ut,
	})

	now := time.Now()
	start := now.Add(-12 * time.Hour)
	end := now.Add(12 * time.Hour)

	// 远端报告已使用 50
	cb(ctx, &HealthReport{
		AccountID: "acc-1",
		Results: []*CheckResult{
			{
				CheckName: "usage_quota",
				Status:    CheckPassed,
				Severity:  SeverityCritical,
				Data: map[string]any{
					ReportDataKeyUsageStats: []*usagerule.UsageStats{
						{
							Rule: &usagerule.UsageRule{
								SourceType:      usagerule.SourceTypeRequest,
								Total:           100,
								TimeGranularity: usagerule.GranularityDay,
								WindowSize:      1,
							},
							Used:      50,
							Remain:    50,
							StartTime: &start,
							EndTime:   &end,
						},
					},
				},
			},
		},
	})

	// 校准后，MinRemainRatio 应该反映远端数据
	ratio, _ := ut.MinRemainRatio(ctx, "acc-1")
	if ratio < 0.40 || ratio > 0.60 {
		t.Fatalf("expected ratio ~0.5 after calibration, got %f", ratio)
	}
}

// --- 组合场景 ---

func TestReportCallback_MultipleResults(t *testing.T) {
	ctx := context.Background()
	as := storeMemory.NewStore()
	addTestAccount(ctx, as, "acc-1", account.StatusAvailable)

	ut := usagetracker.NewUsageTracker()
	_ = ut.InitRules(ctx, "acc-1", []*usagerule.UsageRule{
		{
			SourceType:      usagerule.SourceTypeRequest,
			Total:           100,
			TimeGranularity: usagerule.GranularityDay,
			WindowSize:      1,
		},
	})

	cm := cooldown.NewCooldownManager()

	cb := NewDefaultReportCallback(ReportHandlerDeps{
		AccountStorage:  as,
		UsageTracker:    ut,
		CooldownManager: cm,
	})

	now := time.Now()
	start := now.Add(-12 * time.Hour)
	end := now.Add(12 * time.Hour)

	// 多个检查结果：先是凭证通过，再是用量耗尽（触发冷却）
	cb(ctx, &HealthReport{
		AccountID: "acc-1",
		Results: []*CheckResult{
			{
				CheckName: "credential_validity",
				Status:    CheckPassed,
				Severity:  SeverityCritical,
				Message:   "credential is valid",
			},
			{
				CheckName: "usage_quota",
				Status:    CheckFailed,
				Severity:  SeverityCritical,
				SuggestedStatus: statusPtr(account.StatusCoolingDown),
				Data: map[string]any{
					ReportDataKeyUsageStats: []*usagerule.UsageStats{
						{
							Rule: &usagerule.UsageRule{
								SourceType:      usagerule.SourceTypeRequest,
								Total:           100,
								TimeGranularity: usagerule.GranularityDay,
								WindowSize:      1,
							},
							Used:      98,
							Remain:    2,
							StartTime: &start,
							EndTime:   &end,
						},
					},
					ReportDataKeyCooldownUntil: &end,
				},
			},
		},
	})

	updated, _ := as.GetAccount(ctx, "acc-1")
	if updated.Status != account.StatusCoolingDown {
		t.Fatalf("expected status CoolingDown, got %v", updated.Status)
	}
	if updated.CooldownUntil == nil {
		t.Fatal("expected CooldownUntil to be set")
	}
}

func TestReportCallback_NilResultInSlice(t *testing.T) {
	ctx := context.Background()
	as := storeMemory.NewStore()
	addTestAccount(ctx, as, "acc-1", account.StatusAvailable)

	cb := NewDefaultReportCallback(ReportHandlerDeps{
		AccountStorage: as,
	})

	// 结果切片中包含 nil，不应 panic
	cb(ctx, &HealthReport{
		AccountID: "acc-1",
		Results: []*CheckResult{
			nil,
			{
				CheckName: "test",
				Status:    CheckPassed,
			},
			nil,
		},
	})
}
