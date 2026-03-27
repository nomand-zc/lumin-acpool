package account

import (
	"testing"
	"time"

	"github.com/nomand-zc/lumin-client/usagerule"
)

// --- ProviderKey 测试 ---

func TestProviderKey_String(t *testing.T) {
	pk := ProviderKey{Type: "kiro", Name: "kiro-team-a"}
	got := pk.String()
	want := "kiro/kiro-team-a"
	if got != want {
		t.Errorf("ProviderKey.String() = %q, want %q", got, want)
	}
}

func TestProviderKey_StringEmpty(t *testing.T) {
	pk := ProviderKey{}
	got := pk.String()
	if got != "/" {
		t.Errorf("ProviderKey{}.String() = %q, want \"/\"", got)
	}
}

func TestBuildProviderKey(t *testing.T) {
	pk := BuildProviderKey("gemini", "gemini-prod")
	if pk.Type != "gemini" || pk.Name != "gemini-prod" {
		t.Errorf("BuildProviderKey() = %+v, want {gemini, gemini-prod}", pk)
	}
	if pk.String() != "gemini/gemini-prod" {
		t.Errorf("BuildProviderKey().String() = %q, want \"gemini/gemini-prod\"", pk.String())
	}
}

// --- ProviderInfo 测试 ---

func TestProviderInfo_ProviderKey(t *testing.T) {
	p := &ProviderInfo{ProviderType: "kiro", ProviderName: "kiro-a"}
	pk := p.ProviderKey()
	if pk.Type != "kiro" || pk.Name != "kiro-a" {
		t.Errorf("ProviderInfo.ProviderKey() = %+v, want {kiro, kiro-a}", pk)
	}
}

func TestProviderInfo_SupportsModel(t *testing.T) {
	p := &ProviderInfo{
		SupportedModels: []string{"gpt-4", "claude-3"},
	}
	if !p.SupportsModel("gpt-4") {
		t.Error("expected SupportsModel(\"gpt-4\") = true")
	}
	if !p.SupportsModel("claude-3") {
		t.Error("expected SupportsModel(\"claude-3\") = true")
	}
	if p.SupportsModel("llama-3") {
		t.Error("expected SupportsModel(\"llama-3\") = false")
	}
}

func TestProviderInfo_SupportsModel_Empty(t *testing.T) {
	p := &ProviderInfo{SupportedModels: nil}
	if p.SupportsModel("any-model") {
		t.Error("expected SupportsModel to return false for empty SupportedModels")
	}
}

func TestProviderInfo_IsActive(t *testing.T) {
	cases := []struct {
		status   ProviderStatus
		expected bool
	}{
		{ProviderStatusActive, true},
		{ProviderStatusDegraded, true},
		{ProviderStatusDisabled, false},
	}
	for _, c := range cases {
		p := &ProviderInfo{Status: c.status}
		got := p.IsActive()
		if got != c.expected {
			t.Errorf("ProviderInfo{Status: %d}.IsActive() = %v, want %v", c.status, got, c.expected)
		}
	}
}

func TestProviderInfo_Clone(t *testing.T) {
	p := &ProviderInfo{
		ProviderType:    "kiro",
		ProviderName:    "kiro-a",
		Status:          ProviderStatusActive,
		SupportedModels: []string{"gpt-4"},
		Tags:            map[string]string{"env": "prod"},
		Metadata:        map[string]any{"region": "us-east-1"},
	}
	c := p.Clone()
	if c.ProviderType != p.ProviderType || c.ProviderName != p.ProviderName {
		t.Error("Clone() basic fields mismatch")
	}
	// 深拷贝：修改 clone 不影响原始
	c.SupportedModels[0] = "modified"
	if p.SupportedModels[0] == "modified" {
		t.Error("Clone() SupportedModels should be a deep copy")
	}
	c.Tags["env"] = "staging"
	if p.Tags["env"] == "staging" {
		t.Error("Clone() Tags should be a deep copy")
	}
}

// --- Account.Clone 测试 ---

func TestAccount_Clone_Nil(t *testing.T) {
	var a *Account
	if a.Clone() != nil {
		t.Fatal("expected nil Clone of nil Account")
	}
}

func TestAccount_Clone_DeepCopy(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)
	src := &Account{
		ID:               "acc-1",
		ProviderType:     "kiro",
		ProviderName:     "kiro-a",
		Status:           StatusAvailable,
		Priority:         5,
		Tags:             map[string]string{"env": "prod"},
		Metadata:         map[string]any{"region": "us-east-1"},
		CooldownUntil:    &past,
		CircuitOpenUntil: &future,
	}
	dst := src.Clone()

	if dst == src {
		t.Fatal("Clone should return a new pointer")
	}
	if dst.ID != src.ID || dst.ProviderType != src.ProviderType {
		t.Error("Clone basic fields mismatch")
	}

	// Tags 深拷贝
	dst.Tags["env"] = "staging"
	if src.Tags["env"] == "staging" {
		t.Error("Clone() Tags should be independent")
	}

	// Metadata 深拷贝
	dst.Metadata["region"] = "eu-west-1"
	if src.Metadata["region"] == "eu-west-1" {
		t.Error("Clone() Metadata should be independent")
	}

	// 时间指针深拷贝
	if dst.CooldownUntil == src.CooldownUntil {
		t.Error("Clone() CooldownUntil should be independent pointer")
	}
	if dst.CircuitOpenUntil == src.CircuitOpenUntil {
		t.Error("Clone() CircuitOpenUntil should be independent pointer")
	}
}

func TestAccount_Clone_NilFields(t *testing.T) {
	src := &Account{ID: "acc-nil"}
	dst := src.Clone()
	if dst.Tags != nil || dst.Metadata != nil || dst.UsageRules != nil {
		t.Error("Clone() nil fields should remain nil")
	}
	if dst.CooldownUntil != nil || dst.CircuitOpenUntil != nil {
		t.Error("Clone() nil time pointers should remain nil")
	}
}

func TestProviderInfo_Clone_NilFields(t *testing.T) {
	p := &ProviderInfo{
		ProviderType: "kiro",
		ProviderName: "kiro-b",
	}
	c := p.Clone()
	if c.Tags != nil || c.SupportedModels != nil || c.Metadata != nil {
		t.Error("Clone() nil fields should remain nil")
	}
}

// --- Account 方法测试 ---

func TestAccount_IsCooldownExpired_NilPointer(t *testing.T) {
	a := &Account{}
	if !a.IsCooldownExpired() {
		t.Error("expected IsCooldownExpired() = true when CooldownUntil is nil")
	}
}

func TestAccount_IsCooldownExpired_PastTime(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	a := &Account{CooldownUntil: &past}
	if !a.IsCooldownExpired() {
		t.Error("expected IsCooldownExpired() = true for past time")
	}
}

func TestAccount_IsCooldownExpired_FutureTime(t *testing.T) {
	future := time.Now().Add(time.Hour)
	a := &Account{CooldownUntil: &future}
	if a.IsCooldownExpired() {
		t.Error("expected IsCooldownExpired() = false for future time")
	}
}

func TestAccount_IsCircuitOpenExpired_NilPointer(t *testing.T) {
	a := &Account{}
	if !a.IsCircuitOpenExpired() {
		t.Error("expected IsCircuitOpenExpired() = true when CircuitOpenUntil is nil")
	}
}

func TestAccount_IsCircuitOpenExpired_PastTime(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	a := &Account{CircuitOpenUntil: &past}
	if !a.IsCircuitOpenExpired() {
		t.Error("expected IsCircuitOpenExpired() = true for past time")
	}
}

func TestAccount_IsCircuitOpenExpired_FutureTime(t *testing.T) {
	future := time.Now().Add(time.Hour)
	a := &Account{CircuitOpenUntil: &future}
	if a.IsCircuitOpenExpired() {
		t.Error("expected IsCircuitOpenExpired() = false for future time")
	}
}

func TestAccount_ProviderKey(t *testing.T) {
	a := &Account{ProviderType: "kiro", ProviderName: "kiro-team-a"}
	pk := a.ProviderKey()
	if pk.String() != "kiro/kiro-team-a" {
		t.Errorf("Account.ProviderKey().String() = %q, want \"kiro/kiro-team-a\"", pk.String())
	}
}

// --- AccountStats 测试 ---

func TestAccountStats_SuccessRate_NoCallsReturnsOne(t *testing.T) {
	s := &AccountStats{}
	if s.SuccessRate() != 1.0 {
		t.Errorf("SuccessRate() with no calls = %v, want 1.0", s.SuccessRate())
	}
}

func TestAccountStats_SuccessRate_Normal(t *testing.T) {
	s := &AccountStats{TotalCalls: 10, SuccessCalls: 7}
	got := s.SuccessRate()
	want := 0.7
	if got < want-0.001 || got > want+0.001 {
		t.Errorf("SuccessRate() = %v, want %v", got, want)
	}
}

func TestAccountStats_SuccessRate_AllSuccess(t *testing.T) {
	s := &AccountStats{TotalCalls: 5, SuccessCalls: 5}
	if s.SuccessRate() != 1.0 {
		t.Errorf("SuccessRate() all success = %v, want 1.0", s.SuccessRate())
	}
}

func TestAccountStats_SuccessRate_AllFailed(t *testing.T) {
	s := &AccountStats{TotalCalls: 5, SuccessCalls: 0}
	if s.SuccessRate() != 0.0 {
		t.Errorf("SuccessRate() all failed = %v, want 0.0", s.SuccessRate())
	}
}

// --- TrackedUsage 测试 ---

func TestTrackedUsage_EstimatedUsed(t *testing.T) {
	tu := &TrackedUsage{RemoteUsed: 30, LocalUsed: 10}
	if tu.EstimatedUsed() != 40 {
		t.Errorf("EstimatedUsed() = %v, want 40", tu.EstimatedUsed())
	}
}

func TestTrackedUsage_EstimatedRemain(t *testing.T) {
	tu := &TrackedUsage{RemoteRemain: 70, LocalUsed: 10}
	if tu.EstimatedRemain() != 60 {
		t.Errorf("EstimatedRemain() = %v, want 60", tu.EstimatedRemain())
	}
}

func TestTrackedUsage_IsExhausted_True(t *testing.T) {
	tu := &TrackedUsage{RemoteRemain: 5, LocalUsed: 10}
	if !tu.IsExhausted() {
		t.Error("expected IsExhausted() = true when RemoteRemain < LocalUsed")
	}
}

func TestTrackedUsage_IsExhausted_False(t *testing.T) {
	tu := &TrackedUsage{RemoteRemain: 100, LocalUsed: 10}
	if tu.IsExhausted() {
		t.Error("expected IsExhausted() = false when RemoteRemain > LocalUsed")
	}
}

func TestTrackedUsage_RemainRatio_NilRule(t *testing.T) {
	tu := &TrackedUsage{Rule: nil, RemoteRemain: 50, LocalUsed: 10}
	if tu.RemainRatio() != 1.0 {
		t.Errorf("RemainRatio() with nil Rule = %v, want 1.0", tu.RemainRatio())
	}
}

func TestTrackedUsage_RemainRatio_Normal(t *testing.T) {
	tu := &TrackedUsage{
		Rule:         &usagerule.UsageRule{Total: 100},
		RemoteRemain: 70,
		LocalUsed:    10,
	}
	// EstimatedRemain = 70-10 = 60, ratio = 60/100 = 0.6
	got := tu.RemainRatio()
	want := 0.6
	if got < want-0.001 || got > want+0.001 {
		t.Errorf("RemainRatio() = %v, want %v", got, want)
	}
}

func TestTrackedUsage_RemainRatio_NegativeClampedToZero(t *testing.T) {
	// EstimatedRemain = 5-10 = -5, ratio = -5/100 clamp to 0
	tu := &TrackedUsage{
		Rule:         &usagerule.UsageRule{Total: 100},
		RemoteRemain: 5,
		LocalUsed:    10,
	}
	got := tu.RemainRatio()
	if got != 0 {
		t.Errorf("RemainRatio() negative = %v, want 0", got)
	}
}

func TestTrackedUsage_RemainRatio_ZeroTotal(t *testing.T) {
	tu := &TrackedUsage{
		Rule:         &usagerule.UsageRule{Total: 0},
		RemoteRemain: 50,
		LocalUsed:    0,
	}
	if tu.RemainRatio() != 1.0 {
		t.Errorf("RemainRatio() with zero Total = %v, want 1.0", tu.RemainRatio())
	}
}
