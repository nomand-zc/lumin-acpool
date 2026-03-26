package memory

import (
	"context"
	"sync"
	"testing"
)

func TestOccupancyIncrDecr_Basic(t *testing.T) {
	s := NewStore()
	ctx := context.Background()

	val, err := s.IncrOccupancy(ctx, "acc1")
	if err != nil {
		t.Fatalf("IncrOccupancy error: %v", err)
	}
	if val != 1 {
		t.Errorf("expected 1, got %d", val)
	}

	val, err = s.IncrOccupancy(ctx, "acc1")
	if err != nil {
		t.Fatalf("IncrOccupancy error: %v", err)
	}
	if val != 2 {
		t.Errorf("expected 2, got %d", val)
	}

	if err := s.DecrOccupancy(ctx, "acc1"); err != nil {
		t.Fatalf("DecrOccupancy error: %v", err)
	}

	got, err := s.GetOccupancy(ctx, "acc1")
	if err != nil {
		t.Fatalf("GetOccupancy error: %v", err)
	}
	if got != 1 {
		t.Errorf("expected 1 after decr, got %d", got)
	}
}

func TestOccupancyDecr_NoNegative(t *testing.T) {
	s := NewStore()
	ctx := context.Background()

	// 从 0 开始 Decr，不应变为负数
	if err := s.DecrOccupancy(ctx, "acc2"); err != nil {
		t.Fatalf("DecrOccupancy error: %v", err)
	}
	got, err := s.GetOccupancy(ctx, "acc2")
	if err != nil {
		t.Fatalf("GetOccupancy error: %v", err)
	}
	if got < 0 {
		t.Errorf("occupancy should not be negative, got %d", got)
	}

	// Incr 一次再 Decr 两次，也不应变为负数
	if _, err := s.IncrOccupancy(ctx, "acc3"); err != nil {
		t.Fatalf("IncrOccupancy error: %v", err)
	}
	if err := s.DecrOccupancy(ctx, "acc3"); err != nil {
		t.Fatalf("DecrOccupancy error: %v", err)
	}
	if err := s.DecrOccupancy(ctx, "acc3"); err != nil {
		t.Fatalf("DecrOccupancy error: %v", err)
	}
	got, err = s.GetOccupancy(ctx, "acc3")
	if err != nil {
		t.Fatalf("GetOccupancy error: %v", err)
	}
	if got < 0 {
		t.Errorf("occupancy should not be negative, got %d", got)
	}
}

func TestOccupancyGetOccupancies_Batch(t *testing.T) {
	s := NewStore()
	ctx := context.Background()

	accounts := []string{"a1", "a2", "a3"}
	counts := []int{1, 3, 2}
	for i, id := range accounts {
		for j := 0; j < counts[i]; j++ {
			if _, err := s.IncrOccupancy(ctx, id); err != nil {
				t.Fatalf("IncrOccupancy(%s) error: %v", id, err)
			}
		}
	}

	result, err := s.GetOccupancies(ctx, accounts)
	if err != nil {
		t.Fatalf("GetOccupancies error: %v", err)
	}
	for i, id := range accounts {
		if result[id] != int64(counts[i]) {
			t.Errorf("account %s: expected %d, got %d", id, counts[i], result[id])
		}
	}

	// 不存在的账号不应出现在结果中
	result2, err := s.GetOccupancies(ctx, []string{"nonexistent"})
	if err != nil {
		t.Fatalf("GetOccupancies error: %v", err)
	}
	if _, ok := result2["nonexistent"]; ok {
		t.Errorf("nonexistent account should not appear in result")
	}
}

func TestOccupancyConcurrency(t *testing.T) {
	s := NewStore()
	ctx := context.Background()
	const n = 100
	const id = "concurrent_acc"

	var wg sync.WaitGroup

	// 100 个 goroutine 同时 Incr
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if _, err := s.IncrOccupancy(ctx, id); err != nil {
				t.Errorf("IncrOccupancy error: %v", err)
			}
		}()
	}
	wg.Wait()

	got, err := s.GetOccupancy(ctx, id)
	if err != nil {
		t.Fatalf("GetOccupancy error: %v", err)
	}
	if got != n {
		t.Errorf("after %d incr, expected %d, got %d", n, n, got)
	}

	// 100 个 goroutine 同时 Decr
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if err := s.DecrOccupancy(ctx, id); err != nil {
				t.Errorf("DecrOccupancy error: %v", err)
			}
		}()
	}
	wg.Wait()

	got, err = s.GetOccupancy(ctx, id)
	if err != nil {
		t.Fatalf("GetOccupancy error: %v", err)
	}
	if got != 0 {
		t.Errorf("after %d decr, expected 0, got %d", n, got)
	}
	if got < 0 {
		t.Errorf("occupancy must not be negative, got %d", got)
	}
}

func TestOccupancyZeroCleanup(t *testing.T) {
	s := NewStore()
	ctx := context.Background()
	const id = "cleanup_acc"

	// Incr 3 次
	for i := 0; i < 3; i++ {
		if _, err := s.IncrOccupancy(ctx, id); err != nil {
			t.Fatalf("IncrOccupancy error: %v", err)
		}
	}
	// Decr 3 次，降回 0
	for i := 0; i < 3; i++ {
		if err := s.DecrOccupancy(ctx, id); err != nil {
			t.Fatalf("DecrOccupancy error: %v", err)
		}
	}

	got, err := s.GetOccupancy(ctx, id)
	if err != nil {
		t.Fatalf("GetOccupancy error: %v", err)
	}
	if got != 0 {
		t.Errorf("expected 0 after cleanup, got %d", got)
	}

	// sync.Map 中 key 应已被清理
	if _, ok := s.occupancyStore.Load(id); ok {
		t.Errorf("key %q should have been deleted from sync.Map after reaching 0", id)
	}
}

func BenchmarkOccupancyIncr(b *testing.B) {
	s := NewStore()
	ctx := context.Background()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := s.IncrOccupancy(ctx, "bench_acc"); err != nil {
				b.Fatal(err)
			}
		}
	})
}
