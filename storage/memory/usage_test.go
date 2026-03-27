package memory

import (
	"context"
	"testing"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
)

func TestGetCurrentUsages_Empty(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	usages, err := store.GetCurrentUsages(ctx, "no-such-account")
	if err != nil {
		t.Fatalf("GetCurrentUsages returned unexpected error: %v", err)
	}
	if len(usages) != 0 {
		t.Errorf("expected empty result, got %d usages", len(usages))
	}
}

func TestSaveUsages_AndGet(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	usages := []*account.TrackedUsage{
		{RemoteRemain: 100, RemoteUsed: 50},
		{RemoteRemain: 200, RemoteUsed: 10},
	}

	err := store.SaveUsages(ctx, "acc-1", usages)
	if err != nil {
		t.Fatalf("SaveUsages failed: %v", err)
	}

	result, err := store.GetCurrentUsages(ctx, "acc-1")
	if err != nil {
		t.Fatalf("GetCurrentUsages failed: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 usages, got %d", len(result))
	}
	if result[0].RemoteRemain != 100 {
		t.Errorf("expected RemoteRemain=100, got %f", result[0].RemoteRemain)
	}
	if result[1].RemoteUsed != 10 {
		t.Errorf("expected RemoteUsed=10, got %f", result[1].RemoteUsed)
	}
}

func TestSaveUsages_OverwritesPrevious(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.SaveUsages(ctx, "acc-1", []*account.TrackedUsage{{RemoteRemain: 100}})
	_ = store.SaveUsages(ctx, "acc-1", []*account.TrackedUsage{{RemoteRemain: 999}})

	result, _ := store.GetCurrentUsages(ctx, "acc-1")
	if len(result) != 1 || result[0].RemoteRemain != 999 {
		t.Errorf("expected SaveUsages to overwrite, got %+v", result)
	}
}

func TestIncrLocalUsed_Basic(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.SaveUsages(ctx, "acc-1", []*account.TrackedUsage{
		{RemoteRemain: 100, LocalUsed: 0},
	})

	err := store.IncrLocalUsed(ctx, "acc-1", 0, 5.0)
	if err != nil {
		t.Fatalf("IncrLocalUsed failed: %v", err)
	}

	err = store.IncrLocalUsed(ctx, "acc-1", 0, 3.5)
	if err != nil {
		t.Fatalf("IncrLocalUsed failed: %v", err)
	}

	result, _ := store.GetCurrentUsages(ctx, "acc-1")
	if len(result) != 1 {
		t.Fatalf("expected 1 usage, got %d", len(result))
	}
	if result[0].LocalUsed != 8.5 {
		t.Errorf("expected LocalUsed=8.5, got %f", result[0].LocalUsed)
	}
}

func TestIncrLocalUsed_NotInitialized(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	// 未初始化时静默忽略
	err := store.IncrLocalUsed(ctx, "no-such-account", 0, 1.0)
	if err != nil {
		t.Fatalf("expected no error for uninitialized account, got %v", err)
	}
}

func TestIncrLocalUsed_IndexOutOfRange(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.SaveUsages(ctx, "acc-1", []*account.TrackedUsage{{RemoteRemain: 100}})

	err := store.IncrLocalUsed(ctx, "acc-1", 5, 1.0) // index 5 out of range
	if err == nil {
		t.Error("expected error for out-of-range index")
	}
}

func TestRemoveUsages_Basic(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.SaveUsages(ctx, "acc-1", []*account.TrackedUsage{{RemoteRemain: 100}})

	err := store.RemoveUsages(ctx, "acc-1")
	if err != nil {
		t.Fatalf("RemoveUsages failed: %v", err)
	}

	result, _ := store.GetCurrentUsages(ctx, "acc-1")
	if len(result) != 0 {
		t.Errorf("expected empty after RemoveUsages, got %d", len(result))
	}
}

func TestGetCurrentUsages_FiltersExpiredWindow(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)

	_ = store.SaveUsages(ctx, "acc-1", []*account.TrackedUsage{
		{RemoteRemain: 100, WindowEnd: &past},   // expired
		{RemoteRemain: 200, WindowEnd: &future}, // valid
		{RemoteRemain: 300},                     // no window (always valid)
	})

	result, err := store.GetCurrentUsages(ctx, "acc-1")
	if err != nil {
		t.Fatalf("GetCurrentUsages failed: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 valid usages (expired filtered out), got %d", len(result))
	}
}

func TestCalibrateRule_Basic(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.SaveUsages(ctx, "acc-1", []*account.TrackedUsage{
		{RemoteRemain: 100, LocalUsed: 50},
	})

	newUsage := &account.TrackedUsage{
		RemoteUsed:   200,
		RemoteRemain: 800,
	}
	err := store.CalibrateRule(ctx, "acc-1", 0, newUsage)
	if err != nil {
		t.Fatalf("CalibrateRule failed: %v", err)
	}

	result, _ := store.GetCurrentUsages(ctx, "acc-1")
	if len(result) != 1 {
		t.Fatalf("expected 1 usage after calibrate, got %d", len(result))
	}
	if result[0].RemoteUsed != 200 {
		t.Errorf("expected RemoteUsed=200, got %f", result[0].RemoteUsed)
	}
	if result[0].RemoteRemain != 800 {
		t.Errorf("expected RemoteRemain=800, got %f", result[0].RemoteRemain)
	}
	// LocalUsed 应被重置为 0
	if result[0].LocalUsed != 0 {
		t.Errorf("expected LocalUsed=0 after calibrate, got %f", result[0].LocalUsed)
	}
}

func TestCalibrateRule_NotInitialized(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	err := store.CalibrateRule(ctx, "no-such-account", 0, &account.TrackedUsage{})
	if err != nil {
		t.Fatalf("expected no error for uninitialized account, got %v", err)
	}
}

func TestCalibrateRule_IndexOutOfRange(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.SaveUsages(ctx, "acc-1", []*account.TrackedUsage{{RemoteRemain: 100}})

	err := store.CalibrateRule(ctx, "acc-1", 9, &account.TrackedUsage{})
	if err == nil {
		t.Error("expected error for out-of-range index")
	}
}
