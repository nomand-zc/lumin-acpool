package group

import (
	"testing"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/selector"
	storememory "github.com/nomand-zc/lumin-acpool/storage/memory"
)

// newTestProviderWithWeight 创建带 Weight 的测试 provider
func newTestProviderWithWeight(provType, name string, priority, weight, available int) *account.ProviderInfo {
	return &account.ProviderInfo{
		ProviderType:          provType,
		ProviderName:          name,
		Status:                account.ProviderStatusActive,
		Priority:              priority,
		Weight:                weight,
		AvailableAccountCount: available,
		SupportedModels:       []string{"gpt-4"},
	}
}

// --- GroupRoundRobin 测试 ---

func TestGroupRoundRobin_EmptyCandidates(t *testing.T) {
	g := NewGroupRoundRobin()
	_, err := g.Select(nil, nil)
	if err != selector.ErrEmptyCandidates {
		t.Fatalf("expected ErrEmptyCandidates, got %v", err)
	}
}

func TestGroupRoundRobin_SingleCandidate(t *testing.T) {
	g := NewGroupRoundRobin()
	candidates := []*account.ProviderInfo{
		newTestProvider("kiro", "team-a", 5, 3),
	}

	result, err := g.Select(candidates, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ProviderName != "team-a" {
		t.Fatalf("expected team-a, got %s", result.ProviderName)
	}
}

func TestGroupRoundRobin_CyclesThroughAll(t *testing.T) {
	g := NewGroupRoundRobin()
	candidates := []*account.ProviderInfo{
		newTestProvider("kiro", "team-a", 5, 3),
		newTestProvider("kiro", "team-b", 3, 5),
		newTestProvider("openai", "default", 8, 10),
	}

	seen := make(map[string]int)
	for i := 0; i < 6; i++ {
		result, _ := g.Select(candidates, nil)
		seen[result.ProviderName]++
	}

	// 每个候选者应被选中 2 次
	for _, p := range candidates {
		if seen[p.ProviderName] != 2 {
			t.Fatalf("expected %s to be selected 2 times, got %d", p.ProviderName, seen[p.ProviderName])
		}
	}
}

func TestGroupRoundRobin_Name(t *testing.T) {
	g := NewGroupRoundRobin()
	if g.Name() != "group_round_robin" {
		t.Fatalf("expected 'group_round_robin', got '%s'", g.Name())
	}
}

// --- GroupWeighted 测试 ---

func TestGroupWeighted_EmptyCandidates(t *testing.T) {
	g := NewGroupWeighted()
	_, err := g.Select(nil, nil)
	if err != selector.ErrEmptyCandidates {
		t.Fatalf("expected ErrEmptyCandidates, got %v", err)
	}
}

func TestGroupWeighted_SingleCandidate(t *testing.T) {
	g := NewGroupWeighted()
	candidates := []*account.ProviderInfo{
		newTestProviderWithWeight("kiro", "team-a", 5, 10, 3),
	}

	result, err := g.Select(candidates, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ProviderName != "team-a" {
		t.Fatalf("expected team-a, got %s", result.ProviderName)
	}
}

func TestGroupWeighted_DistributionBiased(t *testing.T) {
	g := NewGroupWeighted()
	candidates := []*account.ProviderInfo{
		newTestProviderWithWeight("kiro", "heavy", 5, 100, 3),
		newTestProviderWithWeight("kiro", "light", 3, 1, 5),
	}

	heavyCount := 0
	runs := 1000
	for i := 0; i < runs; i++ {
		result, _ := g.Select(candidates, nil)
		if result.ProviderName == "heavy" {
			heavyCount++
		}
	}

	ratio := float64(heavyCount) / float64(runs)
	if ratio < 0.90 {
		t.Fatalf("expected heavy provider to be selected >90%% of time, got %.2f%%", ratio*100)
	}
}

func TestGroupWeighted_ZeroWeight_DefaultsToOne(t *testing.T) {
	g := NewGroupWeighted()
	candidates := []*account.ProviderInfo{
		newTestProviderWithWeight("kiro", "team-a", 5, 0, 3),
		newTestProviderWithWeight("kiro", "team-b", 3, 0, 5),
	}

	// 两个都是 0 权重，会被当做 1 处理，应该大致均匀分布
	countA := 0
	runs := 1000
	for i := 0; i < runs; i++ {
		result, _ := g.Select(candidates, nil)
		if result.ProviderName == "team-a" {
			countA++
		}
	}

	ratio := float64(countA) / float64(runs)
	if ratio < 0.35 || ratio > 0.65 {
		t.Fatalf("expected roughly equal distribution for zero weights, got %.2f%%", ratio*100)
	}
}

func TestGroupWeighted_Name(t *testing.T) {
	g := NewGroupWeighted()
	if g.Name() != "group_weighted" {
		t.Fatalf("expected 'group_weighted', got '%s'", g.Name())
	}
}

// --- GroupAffinity 测试 ---

func TestGroupAffinity_EmptyCandidates(t *testing.T) {
	a := NewGroupAffinity()
	_, err := a.Select(nil, nil)
	if err != selector.ErrEmptyCandidates {
		t.Fatalf("expected ErrEmptyCandidates, got %v", err)
	}
}

func TestGroupAffinity_NilRequest_FallbackToGroupPriority(t *testing.T) {
	a := NewGroupAffinity()
	candidates := []*account.ProviderInfo{
		newTestProvider("kiro", "team-a", 1, 3),
		newTestProvider("kiro", "team-b", 10, 5),
	}

	result, err := a.Select(candidates, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// fallback 默认为 GroupPriority，应选最高优先级
	if result.ProviderName != "team-b" {
		t.Fatalf("expected team-b (highest priority via fallback), got %s", result.ProviderName)
	}
}

func TestGroupAffinity_EmptyUserID_Fallback(t *testing.T) {
	a := NewGroupAffinity()
	candidates := []*account.ProviderInfo{
		newTestProvider("kiro", "team-a", 1, 3),
		newTestProvider("kiro", "team-b", 10, 5),
	}

	req := &selector.SelectRequest{
		UserID: "",
		Model:  "gpt-4",
	}
	result, err := a.Select(candidates, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ProviderName != "team-b" {
		t.Fatalf("expected team-b (priority fallback), got %s", result.ProviderName)
	}
}

func TestGroupAffinity_HitBoundProvider(t *testing.T) {
	store := storememory.NewStore()
	a := NewGroupAffinity(GroupAffinityWithStore(store))

	candidates := []*account.ProviderInfo{
		newTestProvider("kiro", "team-a", 10, 3),
		newTestProvider("kiro", "team-b", 1, 5),
		newTestProvider("openai", "default", 5, 8),
	}

	// 预先绑定到 team-b
	store.SetAffinity("user-A:gpt-4", "kiro/team-b")

	req := &selector.SelectRequest{
		UserID: "user-A",
		Model:  "gpt-4",
	}
	result, err := a.Select(candidates, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ProviderName != "team-b" {
		t.Fatalf("expected team-b (affinity hit), got %s", result.ProviderName)
	}
}

func TestGroupAffinity_MissFallbackAndUpdateBinding(t *testing.T) {
	store := storememory.NewStore()
	a := NewGroupAffinity(GroupAffinityWithStore(store))

	candidates := []*account.ProviderInfo{
		newTestProvider("kiro", "team-a", 10, 3),
	}

	req := &selector.SelectRequest{
		UserID: "user-B",
		Model:  "gpt-4",
	}

	result, err := a.Select(candidates, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ProviderName != "team-a" {
		t.Fatalf("expected team-a (only candidate), got %s", result.ProviderName)
	}

	// 验证映射已更新
	boundKey, exists := store.GetAffinity("user-B:gpt-4")
	if !exists {
		t.Fatal("expected binding to be created")
	}
	if boundKey != "kiro/team-a" {
		t.Fatalf("expected binding to kiro/team-a, got %s", boundKey)
	}
}

func TestGroupAffinity_BoundProviderNotInCandidates_Reselect(t *testing.T) {
	store := storememory.NewStore()
	a := NewGroupAffinity(GroupAffinityWithStore(store))

	candidates := []*account.ProviderInfo{
		newTestProvider("kiro", "team-c", 5, 3),
		newTestProvider("openai", "default", 8, 10),
	}

	// 预先绑定到 team-b，但 team-b 不在候选列表中
	store.SetAffinity("user-C:gpt-4", "kiro/team-b")

	req := &selector.SelectRequest{
		UserID: "user-C",
		Model:  "gpt-4",
	}
	result, err := a.Select(candidates, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 应该重新选择
	if result.ProviderName != "team-c" && result.ProviderName != "default" {
		t.Fatalf("expected one of team-c or default, got %s", result.ProviderName)
	}

	// 验证映射已更新
	boundKey, _ := store.GetAffinity("user-C:gpt-4")
	if boundKey != result.ProviderKey().String() {
		t.Fatalf("expected binding updated to %s, got %s", result.ProviderKey().String(), boundKey)
	}
}

func TestGroupAffinity_CustomFallback(t *testing.T) {
	a := NewGroupAffinity(GroupAffinityWithFallback(NewGroupMostAvailable()))

	candidates := []*account.ProviderInfo{
		newTestProvider("kiro", "team-a", 10, 3),
		newTestProvider("kiro", "team-b", 1, 20),
	}

	req := &selector.SelectRequest{
		UserID: "user-D",
		Model:  "gpt-4",
	}

	result, err := a.Select(candidates, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// GroupMostAvailable 应选 team-b（最多可用=20）
	if result.ProviderName != "team-b" {
		t.Fatalf("expected team-b (most available via fallback), got %s", result.ProviderName)
	}
}

func TestGroupAffinity_Name(t *testing.T) {
	a := NewGroupAffinity()
	if a.Name() != "group_affinity" {
		t.Fatalf("expected 'group_affinity', got '%s'", a.Name())
	}
}
