package group

import (
	"testing"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/selector"
)

func newTestProvider(provType, name string, priority int, available int) *account.ProviderInfo {
	return &account.ProviderInfo{
		ProviderType:          provType,
		ProviderName:          name,
		Status:                account.ProviderStatusActive,
		Priority:              priority,
		AvailableAccountCount: available,
		SupportedModels:       []string{"gpt-4"},
	}
}

// --- GroupPriority 测试 ---

func TestGroupPriority_EmptyCandidates(t *testing.T) {
	g := NewGroupPriority()
	_, err := g.Select(nil, nil)
	if err != selector.ErrEmptyCandidates {
		t.Fatalf("expected ErrEmptyCandidates, got %v", err)
	}
}

func TestGroupPriority_SelectsHighest(t *testing.T) {
	g := NewGroupPriority()
	candidates := []*account.ProviderInfo{
		newTestProvider("kiro", "team-a", 1, 5),
		newTestProvider("kiro", "team-b", 10, 3),
		newTestProvider("openai", "default", 5, 8),
	}

	result, err := g.Select(candidates, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ProviderName != "team-b" {
		t.Fatalf("expected team-b (priority=10), got %s", result.ProviderName)
	}
}

func TestGroupPriority_SamePriority_SelectsMostAvailable(t *testing.T) {
	g := NewGroupPriority()
	candidates := []*account.ProviderInfo{
		newTestProvider("kiro", "team-a", 5, 3),
		newTestProvider("kiro", "team-b", 5, 10),
		newTestProvider("kiro", "team-c", 5, 7),
	}

	result, _ := g.Select(candidates, nil)
	if result.ProviderName != "team-b" {
		t.Fatalf("expected team-b (most available=10), got %s", result.ProviderName)
	}
}

func TestGroupPriority_Name(t *testing.T) {
	g := NewGroupPriority()
	if g.Name() != "group_priority" {
		t.Fatalf("expected 'group_priority', got '%s'", g.Name())
	}
}

// --- GroupMostAvailable 测试 ---

func TestGroupMostAvailable_EmptyCandidates(t *testing.T) {
	g := NewGroupMostAvailable()
	_, err := g.Select(nil, nil)
	if err != selector.ErrEmptyCandidates {
		t.Fatalf("expected ErrEmptyCandidates, got %v", err)
	}
}

func TestGroupMostAvailable_SelectsMostAvailable(t *testing.T) {
	g := NewGroupMostAvailable()
	candidates := []*account.ProviderInfo{
		newTestProvider("kiro", "team-a", 10, 3),
		newTestProvider("kiro", "team-b", 1, 20),
		newTestProvider("openai", "default", 5, 10),
	}

	result, err := g.Select(candidates, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ProviderName != "team-b" {
		t.Fatalf("expected team-b (most available=20), got %s", result.ProviderName)
	}
}

func TestGroupMostAvailable_SameAvailable_SelectsHigherPriority(t *testing.T) {
	g := NewGroupMostAvailable()
	candidates := []*account.ProviderInfo{
		newTestProvider("kiro", "team-a", 1, 5),
		newTestProvider("kiro", "team-b", 10, 5),
		newTestProvider("kiro", "team-c", 5, 5),
	}

	result, _ := g.Select(candidates, nil)
	if result.ProviderName != "team-b" {
		t.Fatalf("expected team-b (highest priority as tiebreaker), got %s", result.ProviderName)
	}
}

func TestGroupMostAvailable_Name(t *testing.T) {
	g := NewGroupMostAvailable()
	if g.Name() != "group_most_available" {
		t.Fatalf("expected 'group_most_available', got '%s'", g.Name())
	}
}
