package circuitbreaker

import (
	"context"
	"testing"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	storeMemory "github.com/nomand-zc/lumin-acpool/storage/memory"
	"github.com/nomand-zc/lumin-client/usagerule"
)

func newTestAccount(id string) *account.Account {
	return &account.Account{
		ID:           id,
		ProviderType: "test",
		ProviderName: "default",
		Status:       account.StatusAvailable,
	}
}

func TestNewCircuitBreaker_RequiresStatsStore(t *testing.T) {
	_, err := NewCircuitBreaker()
	if err == nil {
		t.Fatal("expected error when StatsStore is not provided")
	}
}

func TestNewCircuitBreaker_Success(t *testing.T) {
	ss := storeMemory.NewStore()
	cb, err := NewCircuitBreaker(WithStatsStore(ss))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cb == nil {
		t.Fatal("expected non-nil CircuitBreaker")
	}
}

func TestRecordSuccess_ResetsConsecutiveFailures(t *testing.T) {
	ctx := context.Background()
	ss := storeMemory.NewStore()

	// 先注入一些失败
	_, _ = ss.IncrFailure(ctx, "acc-1", "err")
	_, _ = ss.IncrFailure(ctx, "acc-1", "err")

	cb, _ := NewCircuitBreaker(WithStatsStore(ss))
	acct := newTestAccount("acc-1")
	until := time.Now().Add(time.Hour)
	acct.CircuitOpenUntil = &until

	if err := cb.RecordSuccess(ctx, acct); err != nil {
		t.Fatalf("RecordSuccess failed: %v", err)
	}

	// 连续失败应该被重置
	failures, _ := ss.GetConsecutiveFailures(ctx, "acc-1")
	if failures != 0 {
		t.Fatalf("expected 0 consecutive failures after success, got %d", failures)
	}

	// CircuitOpenUntil 应该被清除
	if acct.CircuitOpenUntil != nil {
		t.Fatal("expected CircuitOpenUntil to be nil after success")
	}
}

func TestRecordFailure_BelowThreshold(t *testing.T) {
	ctx := context.Background()
	ss := storeMemory.NewStore()

	// 注入 2 次失败（默认阈值 5）
	_, _ = ss.IncrFailure(ctx, "acc-1", "err")
	_, _ = ss.IncrFailure(ctx, "acc-1", "err")

	cb, _ := NewCircuitBreaker(WithStatsStore(ss))
	acct := newTestAccount("acc-1")

	tripped, err := cb.RecordFailure(ctx, acct, 2)
	if err != nil {
		t.Fatalf("RecordFailure failed: %v", err)
	}
	if tripped {
		t.Fatal("should not trip with failures below threshold")
	}
	if acct.CircuitOpenUntil != nil {
		t.Fatal("CircuitOpenUntil should be nil when not tripped")
	}
}

func TestRecordFailure_ReachesThreshold(t *testing.T) {
	ctx := context.Background()
	ss := storeMemory.NewStore()

	// 注入 5 次失败（= 默认阈值 5）
	for i := 0; i < 5; i++ {
		_, _ = ss.IncrFailure(ctx, "acc-1", "err")
	}

	cb, _ := NewCircuitBreaker(WithStatsStore(ss))
	acct := newTestAccount("acc-1")

	tripped, err := cb.RecordFailure(ctx, acct, 5)
	if err != nil {
		t.Fatalf("RecordFailure failed: %v", err)
	}
	if !tripped {
		t.Fatal("should trip when failures reach threshold")
	}
	if acct.CircuitOpenUntil == nil {
		t.Fatal("CircuitOpenUntil should be set when tripped")
	}
	if !acct.CircuitOpenUntil.After(time.Now()) {
		t.Fatal("CircuitOpenUntil should be in the future")
	}
}

func TestRecordFailure_CustomThreshold(t *testing.T) {
	ctx := context.Background()
	ss := storeMemory.NewStore()

	// 自定义阈值为 2
	cb, _ := NewCircuitBreaker(WithStatsStore(ss), WithDefaultThreshold(2))

	_, _ = ss.IncrFailure(ctx, "acc-1", "err")
	_, _ = ss.IncrFailure(ctx, "acc-1", "err")

	acct := newTestAccount("acc-1")
	tripped, _ := cb.RecordFailure(ctx, acct, 2)
	if !tripped {
		t.Fatal("should trip with custom threshold of 2")
	}
}

func TestShouldAllow_NotExpired(t *testing.T) {
	ss := storeMemory.NewStore()
	cb, _ := NewCircuitBreaker(WithStatsStore(ss))

	acct := newTestAccount("acc-1")
	until := time.Now().Add(time.Hour)
	acct.CircuitOpenUntil = &until

	if cb.ShouldAllow(acct) {
		t.Fatal("should not allow when circuit open has not expired")
	}
}

func TestShouldAllow_Expired(t *testing.T) {
	ss := storeMemory.NewStore()
	cb, _ := NewCircuitBreaker(WithStatsStore(ss))

	acct := newTestAccount("acc-1")
	until := time.Now().Add(-time.Second)
	acct.CircuitOpenUntil = &until

	if !cb.ShouldAllow(acct) {
		t.Fatal("should allow when circuit open has expired")
	}
}

func TestShouldAllow_NilCircuitOpenUntil(t *testing.T) {
	ss := storeMemory.NewStore()
	cb, _ := NewCircuitBreaker(WithStatsStore(ss))

	acct := newTestAccount("acc-1")
	if !cb.ShouldAllow(acct) {
		t.Fatal("should allow when CircuitOpenUntil is nil")
	}
}

func TestDynamicThreshold_WithUsageRules(t *testing.T) {
	ctx := context.Background()
	ss := storeMemory.NewStore()

	// 设置 ThresholdRatio = 0.5, MinThreshold = 3
	cb, _ := NewCircuitBreaker(
		WithStatsStore(ss),
		WithThresholdRatio(0.5),
		WithMinThreshold(3),
	)

	acct := newTestAccount("acc-1")
	acct.UsageRules = []*usagerule.UsageRule{
		{SourceType: usagerule.SourceTypeRequest, Total: 20}, // threshold = 20 * 0.5 = 10
	}

	// 注入 10 次失败
	for i := 0; i < 10; i++ {
		_, _ = ss.IncrFailure(ctx, "acc-1", "err")
	}

	tripped, _ := cb.RecordFailure(ctx, acct, 10)
	if !tripped {
		t.Fatal("should trip when failures reach dynamic threshold (10)")
	}
}

func TestDynamicThreshold_MinThresholdFloor(t *testing.T) {
	ctx := context.Background()
	ss := storeMemory.NewStore()

	// 设置 ThresholdRatio = 0.5, MinThreshold = 3
	// Total = 4 → 4 * 0.5 = 2 → 应提升到 MinThreshold = 3
	cb, _ := NewCircuitBreaker(
		WithStatsStore(ss),
		WithThresholdRatio(0.5),
		WithMinThreshold(3),
	)

	acct := newTestAccount("acc-1")
	acct.UsageRules = []*usagerule.UsageRule{
		{SourceType: usagerule.SourceTypeRequest, Total: 4},
	}

	// 注入 2 次失败（低于 MinThreshold 3）
	_, _ = ss.IncrFailure(ctx, "acc-1", "err")
	_, _ = ss.IncrFailure(ctx, "acc-1", "err")

	tripped, _ := cb.RecordFailure(ctx, acct, 2)
	if tripped {
		t.Fatal("should not trip when failures (2) below MinThreshold (3)")
	}

	// 再注入 1 次（达到 3 = MinThreshold）
	_, _ = ss.IncrFailure(ctx, "acc-1", "err")
	tripped, _ = cb.RecordFailure(ctx, acct, 3)
	if !tripped {
		t.Fatal("should trip when failures reach MinThreshold (3)")
	}
}
