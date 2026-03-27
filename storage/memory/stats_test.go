package memory

import (
	"context"
	"testing"
	"time"
)

func TestGetStats_NotFound_ReturnsZeroValue(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	stats, err := store.GetStats(ctx, "no-such-account")
	if err != nil {
		t.Fatalf("GetStats returned unexpected error: %v", err)
	}
	if stats == nil {
		t.Fatal("expected non-nil stats (zero value)")
	}
	if stats.TotalCalls != 0 || stats.SuccessCalls != 0 || stats.FailedCalls != 0 {
		t.Errorf("expected zero-value stats, got %+v", stats)
	}
}

func TestIncrSuccess_Basic(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.IncrSuccess(ctx, "acc-1")
	_ = store.IncrSuccess(ctx, "acc-1")

	stats, err := store.GetStats(ctx, "acc-1")
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}
	if stats.TotalCalls != 2 {
		t.Errorf("expected TotalCalls=2, got %d", stats.TotalCalls)
	}
	if stats.SuccessCalls != 2 {
		t.Errorf("expected SuccessCalls=2, got %d", stats.SuccessCalls)
	}
	if stats.LastUsedAt == nil {
		t.Error("expected LastUsedAt to be set")
	}
}

func TestIncrSuccess_ResetsConsecutiveFailures(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	// 先制造几次失败
	_, _ = store.IncrFailure(ctx, "acc-1", "err")
	_, _ = store.IncrFailure(ctx, "acc-1", "err")
	_, _ = store.IncrFailure(ctx, "acc-1", "err")

	stats, _ := store.GetStats(ctx, "acc-1")
	if stats.ConsecutiveFailures != 3 {
		t.Fatalf("expected 3 consecutive failures, got %d", stats.ConsecutiveFailures)
	}

	// 成功后应重置
	_ = store.IncrSuccess(ctx, "acc-1")

	stats, _ = store.GetStats(ctx, "acc-1")
	if stats.ConsecutiveFailures != 0 {
		t.Errorf("expected ConsecutiveFailures=0 after IncrSuccess, got %d", stats.ConsecutiveFailures)
	}
}

func TestIncrFailure_Basic(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_, _ = store.IncrFailure(ctx, "acc-1", "timeout")

	stats, err := store.GetStats(ctx, "acc-1")
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}
	if stats.TotalCalls != 1 {
		t.Errorf("expected TotalCalls=1, got %d", stats.TotalCalls)
	}
	if stats.FailedCalls != 1 {
		t.Errorf("expected FailedCalls=1, got %d", stats.FailedCalls)
	}
	if stats.ConsecutiveFailures != 1 {
		t.Errorf("expected ConsecutiveFailures=1, got %d", stats.ConsecutiveFailures)
	}
	if stats.LastErrorMsg != "timeout" {
		t.Errorf("expected LastErrorMsg=timeout, got %s", stats.LastErrorMsg)
	}
	if stats.LastErrorAt == nil {
		t.Error("expected LastErrorAt to be set")
	}
}

func TestIncrFailure_ReturnsConsecutiveCount(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	for i := 1; i <= 5; i++ {
		n, err := store.IncrFailure(ctx, "acc-1", "err")
		if err != nil {
			t.Fatalf("IncrFailure failed: %v", err)
		}
		if n != i {
			t.Errorf("iteration %d: expected return value=%d, got %d", i, i, n)
		}
	}
}

func TestUpdateLastUsed_Basic(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	ts := time.Now().Add(-time.Hour)
	err := store.UpdateLastUsed(ctx, "acc-1", ts)
	if err != nil {
		t.Fatalf("UpdateLastUsed failed: %v", err)
	}

	stats, _ := store.GetStats(ctx, "acc-1")
	if stats.LastUsedAt == nil {
		t.Fatal("expected LastUsedAt to be set")
	}
	if !stats.LastUsedAt.Equal(ts) {
		t.Errorf("expected LastUsedAt=%v, got %v", ts, stats.LastUsedAt)
	}
}

func TestResetConsecutiveFailures_Basic(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_, _ = store.IncrFailure(ctx, "acc-1", "err")
	_, _ = store.IncrFailure(ctx, "acc-1", "err")

	err := store.ResetConsecutiveFailures(ctx, "acc-1")
	if err != nil {
		t.Fatalf("ResetConsecutiveFailures failed: %v", err)
	}

	stats, _ := store.GetStats(ctx, "acc-1")
	if stats.ConsecutiveFailures != 0 {
		t.Errorf("expected ConsecutiveFailures=0, got %d", stats.ConsecutiveFailures)
	}
	// 其他统计不受影响
	if stats.TotalCalls != 2 {
		t.Errorf("expected TotalCalls=2 unchanged, got %d", stats.TotalCalls)
	}
}

func TestGetConsecutiveFailures_Basic(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	n, err := store.GetConsecutiveFailures(ctx, "acc-1")
	if err != nil {
		t.Fatalf("GetConsecutiveFailures failed: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 for new account, got %d", n)
	}

	_, _ = store.IncrFailure(ctx, "acc-1", "err")
	_, _ = store.IncrFailure(ctx, "acc-1", "err")

	n, err = store.GetConsecutiveFailures(ctx, "acc-1")
	if err != nil {
		t.Fatalf("GetConsecutiveFailures failed: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2, got %d", n)
	}
}

func TestRemoveStats_Basic(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.IncrSuccess(ctx, "acc-1")
	_ = store.RemoveStats(ctx, "acc-1")

	// 删除后 GetStats 应返回零值
	stats, _ := store.GetStats(ctx, "acc-1")
	if stats.TotalCalls != 0 {
		t.Errorf("expected zero stats after RemoveStats, got TotalCalls=%d", stats.TotalCalls)
	}
}

func TestGetStats_ReturnsCopy(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.IncrSuccess(ctx, "acc-1")

	stats1, _ := store.GetStats(ctx, "acc-1")
	// 修改返回的副本
	stats1.TotalCalls = 999

	stats2, _ := store.GetStats(ctx, "acc-1")
	if stats2.TotalCalls == 999 {
		t.Error("GetStats should return independent copy, not reference to internal state")
	}
}
