package memory

import (
	"testing"
)

func TestGetAffinity_NotFound(t *testing.T) {
	store := NewStore()

	_, exists := store.GetAffinity("no-such-key")
	if exists {
		t.Error("expected not found, but got exists=true")
	}
}

func TestSetAffinity_AndGet(t *testing.T) {
	store := NewStore()

	store.SetAffinity("user-123", "account-456")

	targetID, exists := store.GetAffinity("user-123")
	if !exists {
		t.Fatal("expected binding to exist after SetAffinity")
	}
	if targetID != "account-456" {
		t.Errorf("expected account-456, got %s", targetID)
	}
}

func TestSetAffinity_Overwrite(t *testing.T) {
	store := NewStore()

	store.SetAffinity("key-1", "old-target")
	store.SetAffinity("key-1", "new-target")

	targetID, exists := store.GetAffinity("key-1")
	if !exists {
		t.Fatal("expected binding to exist")
	}
	if targetID != "new-target" {
		t.Errorf("expected new-target, got %s", targetID)
	}
}

func TestSetAffinity_MultipleKeys(t *testing.T) {
	store := NewStore()

	store.SetAffinity("key-a", "target-a")
	store.SetAffinity("key-b", "target-b")
	store.SetAffinity("key-c", "target-c")

	for _, tc := range []struct{ key, expected string }{
		{"key-a", "target-a"},
		{"key-b", "target-b"},
		{"key-c", "target-c"},
	} {
		got, exists := store.GetAffinity(tc.key)
		if !exists {
			t.Errorf("key %s: expected binding to exist", tc.key)
			continue
		}
		if got != tc.expected {
			t.Errorf("key %s: expected %s, got %s", tc.key, tc.expected, got)
		}
	}
}

func TestSetAffinity_MaxEntriesEviction(t *testing.T) {
	store := NewStore(WithMaxAffinityEntries(5))

	// 填满
	for i := 0; i < 5; i++ {
		store.SetAffinity(string(rune('a'+i)), "target")
	}

	// 第 6 个插入应触发清空
	store.SetAffinity("overflow-key", "target-overflow")

	// overflow-key 应存在（清空后重建）
	_, exists := store.GetAffinity("overflow-key")
	if !exists {
		t.Error("overflow-key should exist after eviction")
	}
}
