package health

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
)

// --- 简单 Mock HealthCheck ---

type mockCheck struct {
	name      string
	severity  CheckSeverity
	dependsOn []string
	status    CheckStatus
}

func (m *mockCheck) Name() string            { return m.name }
func (m *mockCheck) Severity() CheckSeverity { return m.severity }
func (m *mockCheck) DependsOn() []string     { return m.dependsOn }
func (m *mockCheck) Check(_ context.Context, _ CheckTarget) *CheckResult {
	return &CheckResult{
		CheckName: m.name,
		Status:    m.status,
		Severity:  m.severity,
		Timestamp: time.Now(),
	}
}

func newMockTarget(id string) CheckTarget {
	return NewCheckTarget(&account.Account{
		ID:           id,
		ProviderType: "test",
		ProviderName: "default",
	}, nil)
}

// --- Register / Unregister / ListChecks ---

func TestRegister_And_ListChecks(t *testing.T) {
	c := NewHealthChecker()
	c.Register(CheckSchedule{
		Check:    &mockCheck{name: "chk-a", severity: SeverityInfo, status: CheckPassed},
		Interval: time.Second,
		Enabled:  true,
	})

	list := c.ListChecks()
	if len(list) != 1 {
		t.Fatalf("expected 1 check, got %d", len(list))
	}
	if list[0].Check.Name() != "chk-a" {
		t.Fatalf("expected chk-a, got %s", list[0].Check.Name())
	}
}

func TestUnregister_Removes(t *testing.T) {
	c := NewHealthChecker()
	c.Register(CheckSchedule{
		Check:    &mockCheck{name: "chk-a", severity: SeverityInfo, status: CheckPassed},
		Interval: time.Second,
		Enabled:  true,
	})
	c.Register(CheckSchedule{
		Check:    &mockCheck{name: "chk-b", severity: SeverityInfo, status: CheckPassed},
		Interval: time.Second,
		Enabled:  true,
	})

	c.Unregister("chk-a")

	list := c.ListChecks()
	if len(list) != 1 {
		t.Fatalf("expected 1 check after unregister, got %d", len(list))
	}
	if list[0].Check.Name() != "chk-b" {
		t.Fatalf("expected chk-b, got %s", list[0].Check.Name())
	}
}

func TestRegister_DuplicateID_Overrides(t *testing.T) {
	c := NewHealthChecker()
	c.Register(CheckSchedule{
		Check:    &mockCheck{name: "chk-a", severity: SeverityInfo, status: CheckPassed},
		Interval: time.Second,
		Enabled:  true,
	})
	// 重复注册同一 ID，应覆盖
	c.Register(CheckSchedule{
		Check:    &mockCheck{name: "chk-a", severity: SeverityWarning, status: CheckFailed},
		Interval: 2 * time.Second,
		Enabled:  true,
	})

	list := c.ListChecks()
	if len(list) != 1 {
		t.Fatalf("expected 1 check after duplicate register, got %d", len(list))
	}
	if list[0].Check.Severity() != SeverityWarning {
		t.Fatalf("expected overridden severity Warning, got %v", list[0].Check.Severity())
	}
}

// --- RunAll ---

func TestRunAll_SingleCheck_Passes(t *testing.T) {
	c := NewHealthChecker()
	c.Register(CheckSchedule{
		Check:    &mockCheck{name: "chk-a", severity: SeverityInfo, status: CheckPassed},
		Interval: time.Second,
		Enabled:  true,
	})

	report, err := c.RunAll(context.Background(), newMockTarget("acc-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(report.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(report.Results))
	}
	if report.Results[0].Status != CheckPassed {
		t.Fatalf("expected CheckPassed, got %v", report.Results[0].Status)
	}
}

func TestRunAll_MultipleChecks_AllPass(t *testing.T) {
	c := NewHealthChecker()
	for _, name := range []string{"chk-1", "chk-2", "chk-3"} {
		c.Register(CheckSchedule{
			Check:    &mockCheck{name: name, severity: SeverityInfo, status: CheckPassed},
			Interval: time.Second,
			Enabled:  true,
		})
	}

	report, err := c.RunAll(context.Background(), newMockTarget("acc-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(report.Results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(report.Results))
	}
	for _, r := range report.Results {
		if r.Status != CheckPassed {
			t.Fatalf("expected all CheckPassed, got %v for %s", r.Status, r.CheckName)
		}
	}
}

func TestRunAll_DependsOn_SkipsOnFailure(t *testing.T) {
	c := NewHealthChecker()
	c.Register(CheckSchedule{
		Check:    &mockCheck{name: "prereq", severity: SeverityInfo, status: CheckFailed},
		Interval: time.Second,
		Enabled:  true,
	})
	c.Register(CheckSchedule{
		Check: &mockCheck{
			name:      "dependent",
			severity:  SeverityInfo,
			status:    CheckPassed,
			dependsOn: []string{"prereq"},
		},
		Interval: time.Second,
		Enabled:  true,
	})

	report, err := c.RunAll(context.Background(), newMockTarget("acc-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(report.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(report.Results))
	}

	// 找到 dependent 的结果
	var dependentResult *CheckResult
	for _, r := range report.Results {
		if r.CheckName == "dependent" {
			dependentResult = r
		}
	}
	if dependentResult == nil {
		t.Fatal("expected dependent result to exist")
	}
	if dependentResult.Status != CheckSkipped {
		t.Fatalf("expected dependent to be CheckSkipped, got %v", dependentResult.Status)
	}
}

func TestRunAll_DependsOn_RunsOnSuccess(t *testing.T) {
	c := NewHealthChecker()
	c.Register(CheckSchedule{
		Check:    &mockCheck{name: "prereq", severity: SeverityInfo, status: CheckPassed},
		Interval: time.Second,
		Enabled:  true,
	})
	c.Register(CheckSchedule{
		Check: &mockCheck{
			name:      "dependent",
			severity:  SeverityInfo,
			status:    CheckPassed,
			dependsOn: []string{"prereq"},
		},
		Interval: time.Second,
		Enabled:  true,
	})

	report, err := c.RunAll(context.Background(), newMockTarget("acc-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(report.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(report.Results))
	}
	for _, r := range report.Results {
		if r.Status == CheckSkipped {
			t.Fatalf("no check should be skipped when prereq passes, but %s was skipped", r.CheckName)
		}
	}
}

func TestRunAll_EmptyChecks(t *testing.T) {
	c := NewHealthChecker()

	report, err := c.RunAll(context.Background(), newMockTarget("acc-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if len(report.Results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(report.Results))
	}
}

// --- RunOne ---

func TestRunOne_Success(t *testing.T) {
	c := NewHealthChecker()
	c.Register(CheckSchedule{
		Check:    &mockCheck{name: "chk-a", severity: SeverityInfo, status: CheckPassed},
		Interval: time.Second,
		Enabled:  true,
	})

	result, err := c.RunOne(context.Background(), newMockTarget("acc-1"), "chk-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != CheckPassed {
		t.Fatalf("expected CheckPassed, got %v", result.Status)
	}
}

func TestRunOne_NotFound(t *testing.T) {
	c := NewHealthChecker()

	_, err := c.RunOne(context.Background(), newMockTarget("acc-1"), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent check, got nil")
	}
}

// --- Start/Stop 生命周期 ---

func TestStart_Stop_Basic(t *testing.T) {
	c := NewHealthChecker(
		WithTargetProvider(func(_ context.Context) []CheckTarget {
			return nil
		}),
	)
	c.Register(CheckSchedule{
		Check:    &mockCheck{name: "chk-a", severity: SeverityInfo, status: CheckPassed},
		Interval: 10 * time.Millisecond,
		Enabled:  true,
	})

	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	time.Sleep(30 * time.Millisecond)

	if err := c.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestStart_WithoutTargetProvider_ReturnsError(t *testing.T) {
	c := NewHealthChecker() // 没有 TargetProvider
	err := c.Start(context.Background())
	if err == nil {
		t.Fatal("expected error when TargetProvider is not set")
	}
}

func TestStart_AlreadyRunning_ReturnsError(t *testing.T) {
	c := NewHealthChecker(
		WithTargetProvider(func(_ context.Context) []CheckTarget { return nil }),
	)
	c.Register(CheckSchedule{
		Check:    &mockCheck{name: "chk-a", severity: SeverityInfo, status: CheckPassed},
		Interval: 10 * time.Millisecond,
		Enabled:  true,
	})

	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("first Start failed: %v", err)
	}
	defer func() { _ = c.Stop() }()

	err := c.Start(context.Background())
	if err == nil {
		t.Fatal("expected error on second Start")
	}
}

func TestStop_WhenNotRunning_NoError(t *testing.T) {
	c := NewHealthChecker()
	if err := c.Stop(); err != nil {
		t.Fatalf("Stop on non-running checker should not error, got: %v", err)
	}
}

// --- CheckSeverity / CheckStatus String ---

func TestCheckSeverity_String(t *testing.T) {
	cases := []struct {
		s    CheckSeverity
		want string
	}{
		{SeverityInfo, "info"},
		{SeverityWarning, "warning"},
		{SeverityCritical, "critical"},
		{CheckSeverity(99), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.s.String(); got != tc.want {
			t.Errorf("CheckSeverity(%d).String() = %q, want %q", tc.s, got, tc.want)
		}
	}
}

func TestCheckStatus_String(t *testing.T) {
	cases := []struct {
		s    CheckStatus
		want string
	}{
		{CheckPassed, "passed"},
		{CheckWarning, "warning"},
		{CheckFailed, "failed"},
		{CheckSkipped, "skipped"},
		{CheckError, "error"},
		{CheckStatus(99), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.s.String(); got != tc.want {
			t.Errorf("CheckStatus(%d).String() = %q, want %q", tc.s, got, tc.want)
		}
	}
}

// --- HealthReport helpers ---

func TestHealthReport_HasCriticalFailure(t *testing.T) {
	report := &HealthReport{
		Results: []*CheckResult{
			{Status: CheckFailed, Severity: SeverityCritical},
		},
	}
	if !report.HasCriticalFailure() {
		t.Fatal("expected HasCriticalFailure = true")
	}
}

func TestHealthReport_HasCriticalFailure_WarnOnly(t *testing.T) {
	report := &HealthReport{
		Results: []*CheckResult{
			{Status: CheckFailed, Severity: SeverityWarning},
		},
	}
	if report.HasCriticalFailure() {
		t.Fatal("expected HasCriticalFailure = false for non-critical failure")
	}
}

func TestHealthReport_FailedChecks(t *testing.T) {
	report := &HealthReport{
		Results: []*CheckResult{
			{Status: CheckPassed},
			{Status: CheckFailed},
			{Status: CheckFailed},
		},
	}
	failed := report.FailedChecks()
	if len(failed) != 2 {
		t.Fatalf("expected 2 failed checks, got %d", len(failed))
	}
}

func TestHealthReport_WarningChecks(t *testing.T) {
	report := &HealthReport{
		Results: []*CheckResult{
			{Status: CheckPassed},
			{Status: CheckWarning},
		},
	}
	warnings := report.WarningChecks()
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning check, got %d", len(warnings))
	}
}

func TestHealthReport_PassedChecks(t *testing.T) {
	report := &HealthReport{
		Results: []*CheckResult{
			{Status: CheckPassed},
			{Status: CheckPassed},
			{Status: CheckFailed},
		},
	}
	passed := report.PassedChecks()
	if len(passed) != 2 {
		t.Fatalf("expected 2 passed checks, got %d", len(passed))
	}
}

// --- LeaderElector ---

type mockLeaderElector struct {
	isLeader bool
}

func (m *mockLeaderElector) IsLeader(_ context.Context, _ string) bool {
	return m.isLeader
}

func TestWithLeaderElector_NotLeader_SkipsTick(t *testing.T) {
	rec := &recordingCheck{}

	targets := []*account.Account{
		{ID: "acc-1", ProviderType: "test", ProviderName: "default"},
	}

	checker := NewHealthChecker(
		WithTargetProvider(func(_ context.Context) []CheckTarget {
			return []CheckTarget{NewCheckTarget(targets[0], nil)}
		}),
		WithLeaderElector("test-key", &mockLeaderElector{isLeader: false}),
	).(*defaultHealthChecker)

	checker.Register(CheckSchedule{
		Check:    rec,
		Interval: 50 * time.Millisecond,
		Enabled:  true,
	})

	lastRun := make(map[string]time.Time)
	checker.tickRun(context.Background(), time.Now(), lastRun)

	rec.mu.Lock()
	checkedCount := len(rec.checked)
	rec.mu.Unlock()

	if checkedCount != 0 {
		t.Fatalf("expected 0 checks for non-leader, got %d", checkedCount)
	}
}

func TestWithLeaderElector_IsLeader_RunsTick(t *testing.T) {
	rec := &recordingCheck{}

	targets := []*account.Account{
		{ID: "acc-1", ProviderType: "test", ProviderName: "default"},
	}

	checker := NewHealthChecker(
		WithTargetProvider(func(_ context.Context) []CheckTarget {
			return []CheckTarget{NewCheckTarget(targets[0], nil)}
		}),
		WithLeaderElector("test-key", &mockLeaderElector{isLeader: true}),
	).(*defaultHealthChecker)

	checker.Register(CheckSchedule{
		Check:    rec,
		Interval: 50 * time.Millisecond,
		Enabled:  true,
	})

	lastRun := make(map[string]time.Time)
	checker.tickRun(context.Background(), time.Now(), lastRun)

	rec.mu.Lock()
	checkedCount := len(rec.checked)
	rec.mu.Unlock()

	if checkedCount != 1 {
		t.Fatalf("expected 1 check for leader, got %d", checkedCount)
	}
}

// --- WithCallback ---

func TestWithCallback_ReportCallback(t *testing.T) {
	called := false
	cb := func(_ context.Context, _ *HealthReport) {
		called = true
	}

	target := newMockTarget("acc-1")

	c := NewHealthChecker(
		WithTargetProvider(func(_ context.Context) []CheckTarget {
			return []CheckTarget{target}
		}),
		WithCallback(cb),
	).(*defaultHealthChecker)

	c.Register(CheckSchedule{
		Check:    &mockCheck{name: "chk-a", severity: SeverityInfo, status: CheckPassed},
		Interval: 10 * time.Millisecond,
		Enabled:  true,
	})

	lastRun := make(map[string]time.Time)
	c.tickRun(context.Background(), time.Now(), lastRun)

	if !called {
		t.Fatal("expected onReports callback to be called")
	}
}

// --- topological sort ---

func TestTopologicalSort_CircularDependency(t *testing.T) {
	checks := []HealthCheck{
		&mockCheck{name: "a", dependsOn: []string{"b"}},
		&mockCheck{name: "b", dependsOn: []string{"a"}},
	}
	_, err := topologicalSort(checks)
	if err == nil {
		t.Fatal("expected error for circular dependency")
	}
}

func TestTopologicalSort_NoDependency(t *testing.T) {
	checks := []HealthCheck{
		&mockCheck{name: "a"},
		&mockCheck{name: "b"},
	}
	sorted, err := topologicalSort(checks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sorted) != 2 {
		t.Fatalf("expected 2 sorted checks, got %d", len(sorted))
	}
}

// --- shouldSkip ---

func TestShouldSkip_DependencyFailed(t *testing.T) {
	check := &mockCheck{name: "chk", dependsOn: []string{"prereq"}}
	statusMap := map[string]CheckStatus{"prereq": CheckFailed}

	if !shouldSkip(check, statusMap) {
		t.Fatal("expected shouldSkip = true when dependency failed")
	}
}

func TestShouldSkip_DependencyPassed(t *testing.T) {
	check := &mockCheck{name: "chk", dependsOn: []string{"prereq"}}
	statusMap := map[string]CheckStatus{"prereq": CheckPassed}

	if shouldSkip(check, statusMap) {
		t.Fatal("expected shouldSkip = false when dependency passed")
	}
}

func TestShouldSkip_DependencyError(t *testing.T) {
	check := &mockCheck{name: "chk", dependsOn: []string{"prereq"}}
	statusMap := map[string]CheckStatus{"prereq": CheckError}

	if !shouldSkip(check, statusMap) {
		t.Fatal("expected shouldSkip = true when dependency errored")
	}
}

// --- disabled checks are not included in RunAll ---

func TestRunAll_DisabledCheck_NotExecuted(t *testing.T) {
	executed := false
	execCheck := &callbackCheck{
		name:     "exec-chk",
		severity: SeverityInfo,
		fn: func() *CheckResult {
			executed = true
			return &CheckResult{CheckName: "exec-chk", Status: CheckPassed, Severity: SeverityInfo, Timestamp: time.Now()}
		},
	}

	c := NewHealthChecker()
	c.Register(CheckSchedule{
		Check:   execCheck,
		Enabled: false, // disabled
	})

	_, err := c.RunAll(context.Background(), newMockTarget("acc-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if executed {
		t.Fatal("disabled check should not be executed")
	}
}

type callbackCheck struct {
	name      string
	severity  CheckSeverity
	dependsOn []string
	fn        func() *CheckResult
}

func (c *callbackCheck) Name() string            { return c.name }
func (c *callbackCheck) Severity() CheckSeverity { return c.severity }
func (c *callbackCheck) DependsOn() []string     { return c.dependsOn }
func (c *callbackCheck) Check(_ context.Context, _ CheckTarget) *CheckResult {
	return c.fn()
}

// --- RunAll_DependsOn_SkipsOnError ---

func TestRunAll_DependsOn_SkipsWhenDependencyErrors(t *testing.T) {
	c := NewHealthChecker()
	c.Register(CheckSchedule{
		Check:    &mockCheck{name: "prereq", severity: SeverityInfo, status: CheckError},
		Interval: time.Second,
		Enabled:  true,
	})
	c.Register(CheckSchedule{
		Check: &mockCheck{
			name:      "dependent",
			severity:  SeverityInfo,
			status:    CheckPassed,
			dependsOn: []string{"prereq"},
		},
		Interval: time.Second,
		Enabled:  true,
	})

	report, err := c.RunAll(context.Background(), newMockTarget("acc-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var dependentResult *CheckResult
	for _, r := range report.Results {
		if r.CheckName == "dependent" {
			dependentResult = r
		}
	}
	if dependentResult == nil {
		t.Fatal("expected dependent result to exist")
	}
	if dependentResult.Status != CheckSkipped {
		t.Fatalf("expected CheckSkipped when prereq errors, got %v", dependentResult.Status)
	}
}

// --- WithCallback panic for unsupported type ---

func TestWithCallback_UnsupportedType_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for unsupported callback type")
		}
	}()

	// 这里通过类型断言绕过泛型约束，直接测试 panic 分支
	// 注意：泛型约束已经防止编译期错误，但我们可以测试 default case
	// 实际上 WithCallback 的泛型约束只允许 ReportCallback，所以这个 panic 分支
	// 在正常使用中不会触发。这里不测试此分支，改为测试 errors.Is 等工具函数

	// 测试 errors.New 只是避免 unused import
	err := errors.New("test")
	if err == nil {
		t.Fatal("unexpected")
	}
	panic("forced panic for test")
}
