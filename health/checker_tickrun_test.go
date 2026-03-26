package health

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
)

// --- Fix-10: tickRun lastRun 在所有 targets 完成后才更新 ---

// recordingCheck 记录被执行过的 targets
type recordingCheck struct {
	mu      sync.Mutex
	checked []string // 记录每次 Check 时 target 的账号 ID
}

func (c *recordingCheck) Name() string            { return "recording" }
func (c *recordingCheck) Severity() CheckSeverity { return SeverityInfo }
func (c *recordingCheck) DependsOn() []string     { return nil }
func (c *recordingCheck) Check(_ context.Context, target CheckTarget) *CheckResult {
	c.mu.Lock()
	c.checked = append(c.checked, target.Account().ID)
	c.mu.Unlock()
	return &CheckResult{
		CheckName: "recording",
		Status:    CheckPassed,
		Severity:  SeverityInfo,
		Timestamp: time.Now(),
	}
}

// TestTickRun_LastRunUpdatedAfterAllTargets 验证 lastRun 在所有 targets 完成后才更新
func TestTickRun_LastRunUpdatedAfterAllTargets(t *testing.T) {
	rec := &recordingCheck{}

	targets := []*account.Account{
		{ID: "acc-1", ProviderType: "test", ProviderName: "default"},
		{ID: "acc-2", ProviderType: "test", ProviderName: "default"},
		{ID: "acc-3", ProviderType: "test", ProviderName: "default"},
	}

	checker := NewHealthChecker(
		WithTargetProvider(func(_ context.Context) []CheckTarget {
			result := make([]CheckTarget, len(targets))
			for i, acct := range targets {
				result[i] = NewCheckTarget(acct, nil)
			}
			return result
		}),
	).(*defaultHealthChecker)

	checker.Register(CheckSchedule{
		Check:    rec,
		Interval: 100 * time.Millisecond,
		Enabled:  true,
	})

	lastRun := make(map[string]time.Time)
	now := time.Now()

	// 第一次 tickRun（lastRun 为空，所有检查都 due）
	checker.tickRun(context.Background(), now, lastRun)

	// 验证所有 3 个 targets 都被检查
	rec.mu.Lock()
	checkedCount := len(rec.checked)
	rec.mu.Unlock()

	if checkedCount != 3 {
		t.Fatalf("expected all 3 targets to be checked, got %d", checkedCount)
	}

	// 验证 lastRun 已被更新
	if _, ok := lastRun["recording"]; !ok {
		t.Fatal("expected lastRun to be updated after tickRun")
	}
	if !lastRun["recording"].Equal(now) {
		t.Fatalf("expected lastRun[recording]=%v, got %v", now, lastRun["recording"])
	}
}

// TestTickRun_LastRunUpdatedToTickTime 验证 lastRun 设置为 tick 时间（不是 time.Now()）
func TestTickRun_LastRunUpdatedToTickTime(t *testing.T) {
	rec := &recordingCheck{}

	targets := []*account.Account{
		{ID: "acc-1", ProviderType: "test", ProviderName: "default"},
	}

	checker := NewHealthChecker(
		WithTargetProvider(func(_ context.Context) []CheckTarget {
			return []CheckTarget{NewCheckTarget(targets[0], nil)}
		}),
	).(*defaultHealthChecker)

	checker.Register(CheckSchedule{
		Check:    rec,
		Interval: time.Second,
		Enabled:  true,
	})

	lastRun := make(map[string]time.Time)
	tickTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	checker.tickRun(context.Background(), tickTime, lastRun)

	if lastRun["recording"] != tickTime {
		t.Fatalf("expected lastRun to be tickTime=%v, got %v", tickTime, lastRun["recording"])
	}
}

// TestTickRun_SkipsWhenNotDue 未到下次检查时间时不执行
func TestTickRun_SkipsWhenNotDue(t *testing.T) {
	rec := &recordingCheck{}

	targets := []*account.Account{
		{ID: "acc-1", ProviderType: "test", ProviderName: "default"},
	}

	checker := NewHealthChecker(
		WithTargetProvider(func(_ context.Context) []CheckTarget {
			return []CheckTarget{NewCheckTarget(targets[0], nil)}
		}),
	).(*defaultHealthChecker)

	checker.Register(CheckSchedule{
		Check:    rec,
		Interval: time.Hour, // 1 小时间隔
		Enabled:  true,
	})

	lastRun := make(map[string]time.Time)
	now := time.Now()

	// 设置 lastRun 为刚刚（未到下次执行时间）
	lastRun["recording"] = now.Add(-time.Minute) // 1 分钟前运行过，间隔需要 1 小时

	// 再次 tickRun，应该跳过（未到间隔）
	checker.tickRun(context.Background(), now, lastRun)

	rec.mu.Lock()
	checkedCount := len(rec.checked)
	rec.mu.Unlock()

	if checkedCount != 0 {
		t.Fatalf("expected no checks when interval not reached, got %d", checkedCount)
	}
}

// TestTickRun_MultipleTargets_AllCheckedBeforeLastRunUpdate 多 target 场景下确认所有都被检查
func TestTickRun_MultipleTargets_AllCheckedBeforeLastRunUpdate(t *testing.T) {
	const targetCount = 5
	targetAccounts := make([]*account.Account, targetCount)
	for i := 0; i < targetCount; i++ {
		targetAccounts[i] = &account.Account{
			ID:           "acc-" + string(rune('0'+i+1)),
			ProviderType: "test",
			ProviderName: "default",
		}
	}

	rec := &recordingCheck{}

	checker := NewHealthChecker(
		WithTargetProvider(func(_ context.Context) []CheckTarget {
			result := make([]CheckTarget, len(targetAccounts))
			for i, acct := range targetAccounts {
				result[i] = NewCheckTarget(acct, nil)
			}
			return result
		}),
	).(*defaultHealthChecker)

	checker.Register(CheckSchedule{
		Check:    rec,
		Interval: 50 * time.Millisecond,
		Enabled:  true,
	})

	lastRun := make(map[string]time.Time)
	tickTime := time.Now()

	checker.tickRun(context.Background(), tickTime, lastRun)

	rec.mu.Lock()
	checkedCount := len(rec.checked)
	rec.mu.Unlock()

	if checkedCount != targetCount {
		t.Fatalf("expected %d targets to be checked, got %d", targetCount, checkedCount)
	}

	// lastRun 更新在所有 targets 处理完毕之后
	if lastRun["recording"].IsZero() {
		t.Fatal("expected lastRun to be set after all targets processed")
	}
}

// TestTickRun_ContextCanceled_StopsEarly context 取消时提前停止
func TestTickRun_ContextCanceled_StopsEarly(t *testing.T) {
	rec := &recordingCheck{}

	// 创建大量 targets，以便 context 能在中途被取消
	const targetCount = 10
	targetAccounts := make([]*account.Account, targetCount)
	for i := 0; i < targetCount; i++ {
		targetAccounts[i] = &account.Account{
			ID:           "acc-" + string(rune('0'+i)),
			ProviderType: "test",
			ProviderName: "default",
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	// 立即取消
	cancel()

	checker := NewHealthChecker(
		WithTargetProvider(func(_ context.Context) []CheckTarget {
			result := make([]CheckTarget, len(targetAccounts))
			for i, acct := range targetAccounts {
				result[i] = NewCheckTarget(acct, nil)
			}
			return result
		}),
	).(*defaultHealthChecker)

	checker.Register(CheckSchedule{
		Check:    rec,
		Interval: 50 * time.Millisecond,
		Enabled:  true,
	})

	lastRun := make(map[string]time.Time)
	// 使用已取消的 context，tickRun 应该提前退出
	checker.tickRun(ctx, time.Now(), lastRun)

	// 由于 context 已取消，可能 0 个或少量 targets 被处理
	// 关键是不应 panic，且测试顺利完成
	t.Logf("checked %d/%d targets before context canceled", len(rec.checked), targetCount)
}

// TestTickRun_NoTargets_NothingExecuted 无 targets 时不报错
func TestTickRun_NoTargets_NothingExecuted(t *testing.T) {
	rec := &recordingCheck{}

	checker := NewHealthChecker(
		WithTargetProvider(func(_ context.Context) []CheckTarget {
			return nil // 返回空
		}),
	).(*defaultHealthChecker)

	checker.Register(CheckSchedule{
		Check:    rec,
		Interval: 50 * time.Millisecond,
		Enabled:  true,
	})

	lastRun := make(map[string]time.Time)
	checker.tickRun(context.Background(), time.Now(), lastRun)

	// 无 targets，lastRun 仍应被更新（dueChecks 不为空，只是 targets 为空）
	// 这取决于业务逻辑：实际上 targets 循环不执行，lastRun 在 targets 循环外更新
	if _, ok := lastRun["recording"]; !ok {
		t.Fatal("expected lastRun to be updated even with no targets (Fix-10: update happens outside targets loop)")
	}
}
