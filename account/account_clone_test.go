package account

import (
	"testing"
	"time"
)

// --- ShallowClone 测试 ---

func TestShallowClone_NilReturnsNil(t *testing.T) {
	var a *Account
	if a.ShallowClone() != nil {
		t.Fatal("expected nil ShallowClone of nil Account")
	}
}

func TestShallowClone_BasicFields(t *testing.T) {
	src := &Account{
		ID:           "acc-1",
		ProviderType: "kiro",
		ProviderName: "kiro-team-a",
		Priority:     5,
		Status:       StatusAvailable,
	}

	dst := src.ShallowClone()

	if dst.ID != src.ID {
		t.Errorf("ID mismatch: got %s want %s", dst.ID, src.ID)
	}
	if dst.ProviderType != src.ProviderType {
		t.Errorf("ProviderType mismatch: got %s want %s", dst.ProviderType, src.ProviderType)
	}
	if dst.Priority != src.Priority {
		t.Errorf("Priority mismatch: got %d want %d", dst.Priority, src.Priority)
	}
	if dst.Status != src.Status {
		t.Errorf("Status mismatch: got %v want %v", dst.Status, src.Status)
	}
	// 是独立指针
	if dst == src {
		t.Fatal("ShallowClone should return a new pointer")
	}
}

// TestShallowClone_TagsShared 验证 Tags 是共享引用（浅拷贝语义）
func TestShallowClone_TagsShared(t *testing.T) {
	src := &Account{
		ID:   "acc-tags",
		Tags: map[string]string{"env": "prod"},
	}

	dst := src.ShallowClone()

	// Tags 共享同一个 map 引用，通过修改验证
	// 修改 dst.Tags 会影响 src.Tags（共享引用）
	dst.Tags["env"] = "staging"
	if src.Tags["env"] != "staging" {
		t.Error("expected Tags to be shared (shallow clone), but src was not affected")
	}
}

// TestShallowClone_TimePointersIndependent 验证时间指针是独立的
func TestShallowClone_TimePointersIndependent(t *testing.T) {
	now := time.Now()
	src := &Account{
		ID:               "acc-time",
		CooldownUntil:    &now,
		CircuitOpenUntil: &now,
	}

	dst := src.ShallowClone()

	if dst.CooldownUntil == src.CooldownUntil {
		t.Error("CooldownUntil should be an independent pointer after ShallowClone")
	}
	if dst.CircuitOpenUntil == src.CircuitOpenUntil {
		t.Error("CircuitOpenUntil should be an independent pointer after ShallowClone")
	}

	// 修改 dst 的时间指针不影响 src
	later := now.Add(time.Hour)
	dst.CooldownUntil = &later
	if *src.CooldownUntil != now {
		t.Error("modifying dst.CooldownUntil should not affect src.CooldownUntil")
	}

	// 修改指针指向的值也不影响 src（因为是独立拷贝）
	*dst.CircuitOpenUntil = later
	if *src.CircuitOpenUntil != now {
		t.Error("modifying *dst.CircuitOpenUntil should not affect *src.CircuitOpenUntil")
	}
}

// TestShallowClone_NilTimePointers 验证 nil 时间指针正确处理
func TestShallowClone_NilTimePointers(t *testing.T) {
	src := &Account{
		ID:               "acc-nil-time",
		CooldownUntil:    nil,
		CircuitOpenUntil: nil,
	}

	dst := src.ShallowClone()

	if dst.CooldownUntil != nil {
		t.Error("expected CooldownUntil to be nil")
	}
	if dst.CircuitOpenUntil != nil {
		t.Error("expected CircuitOpenUntil to be nil")
	}
}

// TestShallowClone_ModifyBasicFieldDoesNotAffectSrc 修改浅拷贝的基本字段不影响原始对象
func TestShallowClone_ModifyBasicFieldDoesNotAffectSrc(t *testing.T) {
	src := &Account{
		ID:       "acc-basic",
		Priority: 3,
		Status:   StatusAvailable,
	}

	dst := src.ShallowClone()
	dst.Priority = 99
	dst.Status = StatusCoolingDown

	if src.Priority != 3 {
		t.Errorf("src.Priority should be unchanged, got %d", src.Priority)
	}
	if src.Status != StatusAvailable {
		t.Errorf("src.Status should be unchanged, got %v", src.Status)
	}
}

// --- Benchmark ---

func BenchmarkClone_Full(b *testing.B) {
	src := buildBenchAccount()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = src.Clone()
	}
}

func BenchmarkClone_Shallow(b *testing.B) {
	src := buildBenchAccount()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = src.ShallowClone()
	}
}

func buildBenchAccount() *Account {
	now := time.Now()
	return &Account{
		ID:           "bench-acc",
		ProviderType: "kiro",
		ProviderName: "kiro-team-a",
		Priority:     5,
		Status:       StatusAvailable,
		Tags:         map[string]string{"env": "prod", "tier": "premium"},
		Metadata:     map[string]any{"region": "us-east-1", "quota": 1000},
		CooldownUntil: &now,
	}
}
