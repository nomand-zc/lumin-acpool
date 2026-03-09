package memory

import (
	"context"
	"testing"
	"time"

	"github.com/nomand-zc/lumin-acpool/filtercond"
	"github.com/nomand-zc/lumin-acpool/provider"
	"github.com/nomand-zc/lumin-acpool/storage"
)

func newTestProviderInfo(pType, pName string, status provider.ProviderStatus, priority, weight int, models []string) *provider.ProviderInfo {
	return &provider.ProviderInfo{
		Key:             provider.ProviderKey{Type: pType, Name: pName},
		Status:          status,
		Priority:        priority,
		Weight:          weight,
		SupportedModels: models,
		Tags:            map[string]string{"env": "test"},
	}
}

func TestProviderStore_AddAndGet(t *testing.T) {
	ctx := context.Background()
	store := NewProviderStore()

	info := newTestProviderInfo("kiro", "team-a", provider.ProviderStatusActive, 5, 10, []string{"claude-sonnet-4-20250514", "gpt-4"})

	// 正常添加
	if err := store.Add(ctx, info); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// 重复添加
	if err := store.Add(ctx, info); err != storage.ErrAlreadyExists {
		t.Fatalf("expected ErrAlreadyExists, got: %v", err)
	}

	// 获取
	key := provider.ProviderKey{Type: "kiro", Name: "team-a"}
	got, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.Key != key || got.Priority != 5 || got.Weight != 10 {
		t.Fatalf("Get returned wrong data: %+v", got)
	}
	if len(got.SupportedModels) != 2 {
		t.Fatalf("expected 2 models, got %d", len(got.SupportedModels))
	}

	// 验证深拷贝
	got.Priority = 999
	got2, _ := store.Get(ctx, key)
	if got2.Priority != 5 {
		t.Fatalf("expected priority 5, got %d (deep copy failed)", got2.Priority)
	}

	// 获取不存在的
	_, err = store.Get(ctx, provider.ProviderKey{Type: "none", Name: "none"})
	if err != storage.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestProviderStore_Update(t *testing.T) {
	ctx := context.Background()
	store := NewProviderStore()

	info := newTestProviderInfo("kiro", "team-a", provider.ProviderStatusActive, 5, 10, []string{"claude-sonnet-4-20250514"})
	_ = store.Add(ctx, info)

	// 更新：修改模型列表和权重
	updated := newTestProviderInfo("kiro", "team-a", provider.ProviderStatusActive, 5, 20, []string{"claude-sonnet-4-20250514", "gpt-4"})
	if err := store.Update(ctx, updated); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	got, _ := store.Get(ctx, provider.ProviderKey{Type: "kiro", Name: "team-a"})
	if got.Weight != 20 {
		t.Fatalf("expected weight 20, got %d", got.Weight)
	}
	if len(got.SupportedModels) != 2 {
		t.Fatalf("expected 2 models, got %d", len(got.SupportedModels))
	}

	// 验证模型索引已更新
	gpt4List, _ := store.ListByModel(ctx, "gpt-4", nil)
	if len(gpt4List) != 1 {
		t.Fatalf("expected 1 provider for gpt-4 after update, got %d", len(gpt4List))
	}

	// 更新不存在的
	notExist := newTestProviderInfo("none", "none", provider.ProviderStatusActive, 0, 0, nil)
	if err := store.Update(ctx, notExist); err != storage.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestProviderStore_UpdateModelIndex(t *testing.T) {
	ctx := context.Background()
	store := NewProviderStore()

	// 初始支持 model-a
	info := newTestProviderInfo("kiro", "team-a", provider.ProviderStatusActive, 5, 10, []string{"model-a"})
	_ = store.Add(ctx, info)

	// 更新为支持 model-b（不再支持 model-a）
	updated := newTestProviderInfo("kiro", "team-a", provider.ProviderStatusActive, 5, 10, []string{"model-b"})
	_ = store.Update(ctx, updated)

	// model-a 应该查不到了
	listA, _ := store.ListByModel(ctx, "model-a", nil)
	if len(listA) != 0 {
		t.Fatalf("expected 0 providers for model-a, got %d", len(listA))
	}

	// model-b 应该能查到
	listB, _ := store.ListByModel(ctx, "model-b", nil)
	if len(listB) != 1 {
		t.Fatalf("expected 1 provider for model-b, got %d", len(listB))
	}
}

func TestProviderStore_Remove(t *testing.T) {
	ctx := context.Background()
	store := NewProviderStore()

	info := newTestProviderInfo("kiro", "team-a", provider.ProviderStatusActive, 5, 10, []string{"claude-sonnet-4-20250514"})
	_ = store.Add(ctx, info)

	key := provider.ProviderKey{Type: "kiro", Name: "team-a"}
	if err := store.Remove(ctx, key); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	// 确认已删除
	_, err := store.Get(ctx, key)
	if err != storage.ErrNotFound {
		t.Fatalf("expected ErrNotFound after Remove, got: %v", err)
	}

	// 确认索引已清理
	typeList, _ := store.ListByType(ctx, "kiro", nil)
	if len(typeList) != 0 {
		t.Fatalf("expected 0 in type index after Remove, got %d", len(typeList))
	}
	modelList, _ := store.ListByModel(ctx, "claude-sonnet-4-20250514", nil)
	if len(modelList) != 0 {
		t.Fatalf("expected 0 in model index after Remove, got %d", len(modelList))
	}

	// 删除不存在的
	if err := store.Remove(ctx, key); err != storage.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestProviderStore_List(t *testing.T) {
	ctx := context.Background()
	store := NewProviderStore()

	_ = store.Add(ctx, newTestProviderInfo("kiro", "team-a", provider.ProviderStatusActive, 5, 10, []string{"claude-sonnet-4-20250514"}))
	_ = store.Add(ctx, newTestProviderInfo("kiro", "team-b", provider.ProviderStatusDisabled, 3, 5, []string{"claude-sonnet-4-20250514", "gpt-4"}))
	_ = store.Add(ctx, newTestProviderInfo("openai", "default", provider.ProviderStatusActive, 8, 20, []string{"gpt-4"}))

	// 全量
	all, err := store.List(ctx, nil)
	if err != nil {
		t.Fatalf("List(nil) failed: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3, got %d", len(all))
	}

	// 按状态过滤
	active, err := store.List(ctx, filtercond.Equal("provider_status", int(provider.ProviderStatusActive)))
	if err != nil {
		t.Fatalf("List(status=active) failed: %v", err)
	}
	if len(active) != 2 {
		t.Fatalf("expected 2 active, got %d", len(active))
	}

	// 按优先级过滤
	highPrio, err := store.List(ctx, filtercond.GreaterThanOrEqual("priority", 5))
	if err != nil {
		t.Fatalf("List(priority>=5) failed: %v", err)
	}
	if len(highPrio) != 2 {
		t.Fatalf("expected 2 with priority>=5, got %d", len(highPrio))
	}
}

func TestProviderStore_ListByType(t *testing.T) {
	ctx := context.Background()
	store := NewProviderStore()

	_ = store.Add(ctx, newTestProviderInfo("kiro", "team-a", provider.ProviderStatusActive, 5, 10, nil))
	_ = store.Add(ctx, newTestProviderInfo("kiro", "team-b", provider.ProviderStatusDisabled, 3, 5, nil))
	_ = store.Add(ctx, newTestProviderInfo("openai", "default", provider.ProviderStatusActive, 8, 20, nil))

	// 查 kiro 类型
	kiroList, err := store.ListByType(ctx, "kiro", nil)
	if err != nil {
		t.Fatalf("ListByType failed: %v", err)
	}
	if len(kiroList) != 2 {
		t.Fatalf("expected 2 kiro, got %d", len(kiroList))
	}

	// 查 kiro 类型 + 只要 Active
	activeKiro, err := store.ListByType(ctx, "kiro", filtercond.Equal("provider_status", int(provider.ProviderStatusActive)))
	if err != nil {
		t.Fatalf("ListByType(active) failed: %v", err)
	}
	if len(activeKiro) != 1 {
		t.Fatalf("expected 1 active kiro, got %d", len(activeKiro))
	}

	// 不存在的类型
	empty, err := store.ListByType(ctx, "nonexistent", nil)
	if err != nil {
		t.Fatalf("ListByType(nonexistent) failed: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected 0, got %d", len(empty))
	}
}

func TestProviderStore_ListByModel(t *testing.T) {
	ctx := context.Background()
	store := NewProviderStore()

	_ = store.Add(ctx, newTestProviderInfo("kiro", "team-a", provider.ProviderStatusActive, 5, 10, []string{"claude-sonnet-4-20250514"}))
	_ = store.Add(ctx, newTestProviderInfo("kiro", "team-b", provider.ProviderStatusDisabled, 3, 5, []string{"claude-sonnet-4-20250514", "gpt-4"}))
	_ = store.Add(ctx, newTestProviderInfo("openai", "default", provider.ProviderStatusActive, 8, 20, []string{"gpt-4"}))

	// 查支持 claude-sonnet-4-20250514 的
	claudeList, err := store.ListByModel(ctx, "claude-sonnet-4-20250514", nil)
	if err != nil {
		t.Fatalf("ListByModel(claude) failed: %v", err)
	}
	if len(claudeList) != 2 {
		t.Fatalf("expected 2 providers for claude, got %d", len(claudeList))
	}

	// 查支持 gpt-4 的 + 只要 Active
	activeGpt4, err := store.ListByModel(ctx, "gpt-4", filtercond.Equal("provider_status", int(provider.ProviderStatusActive)))
	if err != nil {
		t.Fatalf("ListByModel(gpt-4, active) failed: %v", err)
	}
	if len(activeGpt4) != 1 {
		t.Fatalf("expected 1 active for gpt-4, got %d", len(activeGpt4))
	}

	// 不存在的模型
	empty, err := store.ListByModel(ctx, "nonexistent-model", nil)
	if err != nil {
		t.Fatalf("ListByModel(nonexistent) failed: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected 0, got %d", len(empty))
	}
}

func TestProviderStore_TimeAutoSet(t *testing.T) {
	ctx := context.Background()
	store := NewProviderStore()

	before := time.Now()
	info := newTestProviderInfo("kiro", "team-a", provider.ProviderStatusActive, 5, 10, nil)
	_ = store.Add(ctx, info)
	after := time.Now()

	got, _ := store.Get(ctx, provider.ProviderKey{Type: "kiro", Name: "team-a"})
	if got.CreatedAt.Before(before) || got.CreatedAt.After(after) {
		t.Fatalf("CreatedAt not auto-set properly: %v", got.CreatedAt)
	}
	if got.UpdatedAt.Before(before) || got.UpdatedAt.After(after) {
		t.Fatalf("UpdatedAt not auto-set properly: %v", got.UpdatedAt)
	}
}

func TestProviderStore_FilterBySupportedModel(t *testing.T) {
	ctx := context.Background()
	store := NewProviderStore()

	_ = store.Add(ctx, newTestProviderInfo("kiro", "team-a", provider.ProviderStatusActive, 5, 10, []string{"model-a", "model-b"}))
	_ = store.Add(ctx, newTestProviderInfo("kiro", "team-b", provider.ProviderStatusActive, 3, 5, []string{"model-b", "model-c"}))
	_ = store.Add(ctx, newTestProviderInfo("openai", "default", provider.ProviderStatusActive, 8, 20, []string{"model-c"}))

	// 通过 filtercond 查 supported_model = "model-a"
	result, err := store.List(ctx, filtercond.Equal("supported_model", "model-a"))
	if err != nil {
		t.Fatalf("List(supported_model=model-a) failed: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1, got %d", len(result))
	}

	// 通过 filtercond 查 supported_model in ["model-a", "model-c"]
	result2, err := store.List(ctx, filtercond.In("supported_model", "model-a", "model-c"))
	if err != nil {
		t.Fatalf("List(supported_model in) failed: %v", err)
	}
	if len(result2) != 3 {
		t.Fatalf("expected 3, got %d", len(result2))
	}
}

func TestProviderStore_CombinedFilter(t *testing.T) {
	ctx := context.Background()
	store := NewProviderStore()

	_ = store.Add(ctx, newTestProviderInfo("kiro", "team-a", provider.ProviderStatusActive, 5, 10, []string{"model-a"}))
	_ = store.Add(ctx, newTestProviderInfo("kiro", "team-b", provider.ProviderStatusDisabled, 3, 5, []string{"model-a"}))
	_ = store.Add(ctx, newTestProviderInfo("openai", "default", provider.ProviderStatusActive, 8, 20, []string{"model-b"}))

	// And: active + 支持 model-a
	result, err := store.List(ctx, filtercond.And(
		filtercond.Equal("provider_status", int(provider.ProviderStatusActive)),
		filtercond.Equal("supported_model", "model-a"),
	))
	if err != nil {
		t.Fatalf("List(and) failed: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1, got %d", len(result))
	}
	if result[0].Key.Name != "team-a" {
		t.Fatalf("expected team-a, got %s", result[0].Key.Name)
	}

	// Or: weight >= 20 或 priority >= 5
	orResult, err := store.List(ctx, filtercond.Or(
		filtercond.GreaterThanOrEqual("weight", 20),
		filtercond.GreaterThanOrEqual("priority", 5),
	))
	if err != nil {
		t.Fatalf("List(or) failed: %v", err)
	}
	if len(orResult) != 2 {
		t.Fatalf("expected 2, got %d", len(orResult))
	}
}

func TestProviderStore_InvalidField(t *testing.T) {
	ctx := context.Background()
	store := NewProviderStore()

	_, err := store.List(ctx, filtercond.Equal("nonexistent_field", "value"))
	if err == nil {
		t.Fatal("expected error for invalid field, got nil")
	}
}
