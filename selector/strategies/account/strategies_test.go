package account

import (
	"context"
	"testing"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/selector"
	storeMemory "github.com/nomand-zc/lumin-acpool/storage/memory"
)

func newTestAccount(id string, priority int) *account.Account {
	return &account.Account{
		ID:           id,
		ProviderType: "test",
		ProviderName: "default",
		Status:       account.StatusAvailable,
		Priority:     priority,
	}
}

// --- RoundRobin 测试 ---

func TestRoundRobin_EmptyCandidates(t *testing.T) {
	rr := NewRoundRobin()
	_, err := rr.Select(nil, nil)
	if err != selector.ErrEmptyCandidates {
		t.Fatalf("expected ErrEmptyCandidates, got %v", err)
	}
}

func TestRoundRobin_SingleCandidate(t *testing.T) {
	rr := NewRoundRobin()
	acct := newTestAccount("acc-1", 5)

	result, err := rr.Select([]*account.Account{acct}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ID != "acc-1" {
		t.Fatalf("expected acc-1, got %s", result.ID)
	}
}

func TestRoundRobin_CyclesThroughAll(t *testing.T) {
	rr := NewRoundRobin()
	candidates := []*account.Account{
		newTestAccount("acc-1", 1),
		newTestAccount("acc-2", 2),
		newTestAccount("acc-3", 3),
	}

	seen := make(map[string]int)
	for i := 0; i < 6; i++ {
		result, _ := rr.Select(candidates, nil)
		seen[result.ID]++
	}

	// 每个候选者应被选中 2 次
	for _, acct := range candidates {
		if seen[acct.ID] != 2 {
			t.Fatalf("expected %s to be selected 2 times, got %d", acct.ID, seen[acct.ID])
		}
	}
}

func TestRoundRobin_Name(t *testing.T) {
	rr := NewRoundRobin()
	if rr.Name() != "round_robin" {
		t.Fatalf("expected 'round_robin', got '%s'", rr.Name())
	}
}

// --- Priority 测试 ---

func TestPriority_EmptyCandidates(t *testing.T) {
	p := NewPriority()
	_, err := p.Select(nil, nil)
	if err != selector.ErrEmptyCandidates {
		t.Fatalf("expected ErrEmptyCandidates, got %v", err)
	}
}

func TestPriority_SelectsHighest(t *testing.T) {
	p := NewPriority()
	candidates := []*account.Account{
		newTestAccount("acc-1", 1),
		newTestAccount("acc-2", 10),
		newTestAccount("acc-3", 5),
	}

	result, err := p.Select(candidates, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ID != "acc-2" {
		t.Fatalf("expected acc-2 (priority=10), got %s", result.ID)
	}
}

func TestPriority_SingleCandidate(t *testing.T) {
	p := NewPriority()
	acct := newTestAccount("acc-1", 5)

	result, _ := p.Select([]*account.Account{acct}, nil)
	if result.ID != "acc-1" {
		t.Fatalf("expected acc-1, got %s", result.ID)
	}
}

func TestPriority_Name(t *testing.T) {
	p := NewPriority()
	if p.Name() != "priority" {
		t.Fatalf("expected 'priority', got '%s'", p.Name())
	}
}

// --- Weighted 测试 ---

func TestWeighted_EmptyCandidates(t *testing.T) {
	w := NewWeighted()
	_, err := w.Select(nil, nil)
	if err != selector.ErrEmptyCandidates {
		t.Fatalf("expected ErrEmptyCandidates, got %v", err)
	}
}

func TestWeighted_SingleCandidate(t *testing.T) {
	w := NewWeighted()
	acct := newTestAccount("acc-1", 5)

	result, _ := w.Select([]*account.Account{acct}, nil)
	if result.ID != "acc-1" {
		t.Fatalf("expected acc-1, got %s", result.ID)
	}
}

func TestWeighted_DistributionBiased(t *testing.T) {
	w := NewWeighted()
	candidates := []*account.Account{
		newTestAccount("acc-high", 100),
		newTestAccount("acc-low", 1),
	}

	highCount := 0
	runs := 1000
	for i := 0; i < runs; i++ {
		result, _ := w.Select(candidates, nil)
		if result.ID == "acc-high" {
			highCount++
		}
	}

	// acc-high 权重 100, acc-low 权重 1，acc-high 应该被选中 ~99% 的时间
	ratio := float64(highCount) / float64(runs)
	if ratio < 0.90 {
		t.Fatalf("expected high-priority account to be selected >90%% of time, got %.2f%%", ratio*100)
	}
}

func TestWeighted_Name(t *testing.T) {
	w := NewWeighted()
	if w.Name() != "weighted" {
		t.Fatalf("expected 'weighted', got '%s'", w.Name())
	}
}

// --- LeastUsed 测试 ---

func TestLeastUsed_EmptyCandidates(t *testing.T) {
	lu := NewLeastUsed(nil)
	_, err := lu.Select(nil, nil)
	if err != selector.ErrEmptyCandidates {
		t.Fatalf("expected ErrEmptyCandidates, got %v", err)
	}
}

func TestLeastUsed_NilStatsStore_FallbackToPriority(t *testing.T) {
	lu := NewLeastUsed(nil)
	candidates := []*account.Account{
		newTestAccount("acc-1", 1),
		newTestAccount("acc-2", 10),
		newTestAccount("acc-3", 5),
	}

	result, err := lu.Select(candidates, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ID != "acc-2" {
		t.Fatalf("expected acc-2 (highest priority as fallback), got %s", result.ID)
	}
}

func TestLeastUsed_WithStatsStore_SelectsLeastCalls(t *testing.T) {
	ctx := context.Background()
	ss := storeMemory.NewStore()

	// acc-1: 10 次调用
	for i := 0; i < 10; i++ {
		_ = ss.IncrSuccess(ctx, "acc-1")
	}
	// acc-2: 3 次调用
	for i := 0; i < 3; i++ {
		_ = ss.IncrSuccess(ctx, "acc-2")
	}
	// acc-3: 7 次调用
	for i := 0; i < 7; i++ {
		_ = ss.IncrSuccess(ctx, "acc-3")
	}

	lu := NewLeastUsed(ss)
	candidates := []*account.Account{
		newTestAccount("acc-1", 10),
		newTestAccount("acc-2", 1),
		newTestAccount("acc-3", 5),
	}

	result, err := lu.Select(candidates, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ID != "acc-2" {
		t.Fatalf("expected acc-2 (least calls=3), got %s (priority doesn't matter here)", result.ID)
	}
}

func TestLeastUsed_SameCalls_SelectsHigherPriority(t *testing.T) {
	ss := storeMemory.NewStore()
	// 所有账号都是 0 次调用

	lu := NewLeastUsed(ss)
	candidates := []*account.Account{
		newTestAccount("acc-1", 1),
		newTestAccount("acc-2", 10),
		newTestAccount("acc-3", 5),
	}

	result, err := lu.Select(candidates, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ID != "acc-2" {
		t.Fatalf("expected acc-2 (highest priority as tiebreaker), got %s", result.ID)
	}
}

func TestLeastUsed_Name(t *testing.T) {
	lu := NewLeastUsed(nil)
	if lu.Name() != "least_used" {
		t.Fatalf("expected 'least_used', got '%s'", lu.Name())
	}
}
