package account

import (
	"testing"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/selector"
	storeMemory "github.com/nomand-zc/lumin-acpool/storage/memory"
)

// --- Affinity 测试 ---

func TestAffinity_EmptyCandidates(t *testing.T) {
	a := NewAffinity()
	_, err := a.Select(nil, nil)
	if err != selector.ErrEmptyCandidates {
		t.Fatalf("expected ErrEmptyCandidates, got %v", err)
	}
}

func TestAffinity_NilRequest_FallbackToRoundRobin(t *testing.T) {
	a := NewAffinity()
	candidates := []*account.Account{
		newTestAccount("acc-1", 5),
		newTestAccount("acc-2", 10),
	}

	result, err := a.Select(candidates, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestAffinity_EmptyUserID_FallbackToRoundRobin(t *testing.T) {
	a := NewAffinity()
	candidates := []*account.Account{
		newTestAccount("acc-1", 5),
		newTestAccount("acc-2", 10),
	}

	req := &selector.SelectRequest{
		UserID: "",
		Model:  "gpt-4",
	}
	result, err := a.Select(candidates, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestAffinity_HitBoundAccount(t *testing.T) {
	store := storeMemory.NewStore()
	a := NewAffinity(AffinityWithStore(store))

	candidates := []*account.Account{
		newTestAccount("acc-1", 5),
		newTestAccount("acc-2", 10),
		newTestAccount("acc-3", 3),
	}

	// 预先设置亲和绑定
	store.SetAffinity("user-A:gpt-4", "acc-2")

	req := &selector.SelectRequest{
		UserID: "user-A",
		Model:  "gpt-4",
	}
	result, err := a.Select(candidates, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ID != "acc-2" {
		t.Fatalf("expected acc-2 (affinity hit), got %s", result.ID)
	}
}

func TestAffinity_MissFallbackAndUpdateBinding(t *testing.T) {
	store := storeMemory.NewStore()
	a := NewAffinity(AffinityWithStore(store))

	candidates := []*account.Account{
		newTestAccount("acc-1", 5),
	}

	req := &selector.SelectRequest{
		UserID: "user-B",
		Model:  "gpt-4",
	}

	// 没有预设绑定，应该 fallback 选中并更新映射
	result, err := a.Select(candidates, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ID != "acc-1" {
		t.Fatalf("expected acc-1 (only candidate), got %s", result.ID)
	}

	// 验证映射已更新
	boundID, exists := store.GetAffinity("user-B:gpt-4")
	if !exists {
		t.Fatal("expected binding to be created")
	}
	if boundID != "acc-1" {
		t.Fatalf("expected binding to acc-1, got %s", boundID)
	}
}

func TestAffinity_BoundAccountNotInCandidates_Reselect(t *testing.T) {
	store := storeMemory.NewStore()
	a := NewAffinity(AffinityWithStore(store))

	candidates := []*account.Account{
		newTestAccount("acc-3", 5),
		newTestAccount("acc-4", 10),
	}

	// 预先绑定到 acc-2，但 acc-2 不在候选列表中
	store.SetAffinity("user-C:gpt-4", "acc-2")

	req := &selector.SelectRequest{
		UserID: "user-C",
		Model:  "gpt-4",
	}
	result, err := a.Select(candidates, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 应该从候选中重新选择，并更新映射
	if result.ID != "acc-3" && result.ID != "acc-4" {
		t.Fatalf("expected one of acc-3 or acc-4, got %s", result.ID)
	}

	// 验证映射已更新
	boundID, _ := store.GetAffinity("user-C:gpt-4")
	if boundID != result.ID {
		t.Fatalf("expected binding updated to %s, got %s", result.ID, boundID)
	}
}

func TestAffinity_CustomFallback(t *testing.T) {
	// 使用 Priority 作为 fallback
	a := NewAffinity(AffinityWithFallback(NewPriority()))

	candidates := []*account.Account{
		newTestAccount("acc-1", 1),
		newTestAccount("acc-2", 100),
		newTestAccount("acc-3", 50),
	}

	req := &selector.SelectRequest{
		UserID: "user-D",
		Model:  "gpt-4",
	}

	// 首次选择：应该使用 Priority fallback，选中 acc-2（最高优先级）
	result, err := a.Select(candidates, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ID != "acc-2" {
		t.Fatalf("expected acc-2 (highest priority via fallback), got %s", result.ID)
	}
}

func TestAffinity_Name(t *testing.T) {
	a := NewAffinity()
	if a.Name() != "affinity" {
		t.Fatalf("expected 'affinity', got '%s'", a.Name())
	}
}
