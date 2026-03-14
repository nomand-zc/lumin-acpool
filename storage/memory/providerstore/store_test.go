package providerstore

import (
	"context"
	"testing"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-acpool/storage/filtercond"
)

func newTestProviderInfo(pType, pName string, status account.ProviderStatus, priority, weight int, models []string) *account.ProviderInfo {
	return &account.ProviderInfo{
		ProviderType:    pType,
		ProviderName:    pName,
		Status:          status,
		Priority:        priority,
		Weight:          weight,
		SupportedModels: models,
		Tags:            map[string]string{"env": "test"},
	}
}

func TestStore_AddAndGet(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	info := newTestProviderInfo("kiro", "team-a", account.ProviderStatusActive, 5, 10, []string{"claude-sonnet-4-20250514", "gpt-4"})

	// 正常添加
	if err := store.AddProvider(ctx, info); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// 重复添加
	if err := store.AddProvider(ctx, info); err != storage.ErrAlreadyExists {
		t.Fatalf("expected ErrAlreadyExists, got: %v", err)
	}

	// 获取
	key := account.BuildProviderKey("kiro", "team-a")
	got, err := store.GetProvider(ctx, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.ProviderKey() != key || got.Priority != 5 || got.Weight != 10 {
		t.Fatalf("Get returned wrong data: %+v", got)
	}
	if len(got.SupportedModels) != 2 {
		t.Fatalf("expected 2 models, got %d", len(got.SupportedModels))
	}

	// 验证深拷贝
	got.Priority = 999
	got2, _ := store.GetProvider(ctx, key)
	if got2.Priority != 5 {
		t.Fatalf("expected priority 5, got %d (deep copy failed)", got2.Priority)
	}

	// 获取不存在的
	_, err = store.GetProvider(ctx, account.BuildProviderKey("none", "none"))
	if err != storage.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestStore_Update(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	info := newTestProviderInfo("kiro", "team-a", account.ProviderStatusActive, 5, 10, []string{"claude-sonnet-4-20250514"})
	_ = store.AddProvider(ctx, info)

	// 更新：修改模型列表和权重
	updated := newTestProviderInfo("kiro", "team-a", account.ProviderStatusActive, 5, 20, []string{"claude-sonnet-4-20250514", "gpt-4"})
	if err := store.UpdateProvider(ctx, updated); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	got, _ := store.GetProvider(ctx, account.BuildProviderKey("kiro", "team-a"))
	if got.Weight != 20 {
		t.Fatalf("expected weight 20, got %d", got.Weight)
	}
	if len(got.SupportedModels) != 2 {
		t.Fatalf("expected 2 models, got %d", len(got.SupportedModels))
	}

	// 验证模型索引已更新
	gpt4List, _ := store.SearchProviders(ctx, &storage.SearchFilter{
		ExtraCond: filtercond.Equal("supported_model", "gpt-4"),
	})
	if len(gpt4List) != 1 {
		t.Fatalf("expected 1 provider for gpt-4 after update, got %d", len(gpt4List))
	}

	// 更新不存在的
	notExist := newTestProviderInfo("none", "none", account.ProviderStatusActive, 0, 0, nil)
	if err := store.UpdateProvider(ctx, notExist); err != storage.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestStore_UpdateModelIndex(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	// 初始支持 model-a
	info := newTestProviderInfo("kiro", "team-a", account.ProviderStatusActive, 5, 10, []string{"model-a"})
	_ = store.AddProvider(ctx, info)

	// 更新为支持 model-b（不再支持 model-a）
	updated := newTestProviderInfo("kiro", "team-a", account.ProviderStatusActive, 5, 10, []string{"model-b"})
	_ = store.UpdateProvider(ctx, updated)

	// model-a 应该查不到了
	listA, _ := store.SearchProviders(ctx, &storage.SearchFilter{
		ExtraCond: filtercond.Equal("supported_model", "model-a"),
	})
	if len(listA) != 0 {
		t.Fatalf("expected 0 providers for model-a, got %d", len(listA))
	}

	// model-b 应该能查到
	listB, _ := store.SearchProviders(ctx, &storage.SearchFilter{
		ExtraCond: filtercond.Equal("supported_model", "model-b"),
	})
	if len(listB) != 1 {
		t.Fatalf("expected 1 provider for model-b, got %d", len(listB))
	}
}

func TestStore_Remove(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	info := newTestProviderInfo("kiro", "team-a", account.ProviderStatusActive, 5, 10, []string{"claude-sonnet-4-20250514"})
	_ = store.AddProvider(ctx, info)

	key := account.BuildProviderKey("kiro", "team-a")
	if err := store.RemoveProvider(ctx, key); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	// 确认已删除
	_, err := store.GetProvider(ctx, key)
	if err != storage.ErrNotFound {
		t.Fatalf("expected ErrNotFound after Remove, got: %v", err)
	}

	// 确认索引已清理
	typeList, _ := store.SearchProviders(ctx, &storage.SearchFilter{
		ProviderType: "kiro",
	})
	if len(typeList) != 0 {
		t.Fatalf("expected 0 in type index after Remove, got %d", len(typeList))
	}
	modelList, _ := store.SearchProviders(ctx, &storage.SearchFilter{
		ExtraCond: filtercond.Equal("supported_model", "claude-sonnet-4-20250514"),
	})
	if len(modelList) != 0 {
		t.Fatalf("expected 0 in model index after Remove, got %d", len(modelList))
	}

	// 删除不存在的
	if err := store.RemoveProvider(ctx, key); err != storage.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestStore_List(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, newTestProviderInfo("kiro", "team-a", account.ProviderStatusActive, 5, 10, []string{"claude-sonnet-4-20250514"}))
	_ = store.AddProvider(ctx, newTestProviderInfo("kiro", "team-b", account.ProviderStatusDisabled, 3, 5, []string{"claude-sonnet-4-20250514", "gpt-4"}))
	_ = store.AddProvider(ctx, newTestProviderInfo("openai", "default", account.ProviderStatusActive, 8, 20, []string{"gpt-4"}))

	// 全量
	all, err := store.SearchProviders(ctx, nil)
	if err != nil {
		t.Fatalf("List(nil) failed: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3, got %d", len(all))
	}

	// 按状态过滤
	active, err := store.SearchProviders(ctx, &storage.SearchFilter{
		Status: int(account.ProviderStatusActive),
	})
	if err != nil {
		t.Fatalf("List(status=active) failed: %v", err)
	}
	if len(active) != 2 {
		t.Fatalf("expected 2 active, got %d", len(active))
	}

	// 按优先级过滤
	highPrio, err := store.SearchProviders(ctx, &storage.SearchFilter{
		ExtraCond: filtercond.GreaterThanOrEqual("priority", 5),
	})
	if err != nil {
		t.Fatalf("List(priority>=5) failed: %v", err)
	}
	if len(highPrio) != 2 {
		t.Fatalf("expected 2 with priority>=5, got %d", len(highPrio))
	}
}

func TestStore_TimeAutoSet(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	before := time.Now()
	info := newTestProviderInfo("kiro", "team-a", account.ProviderStatusActive, 5, 10, nil)
	_ = store.AddProvider(ctx, info)
	after := time.Now()

	got, _ := store.GetProvider(ctx, account.BuildProviderKey("kiro", "team-a"))
	if got.CreatedAt.Before(before) || got.CreatedAt.After(after) {
		t.Fatalf("CreatedAt not auto-set properly: %v", got.CreatedAt)
	}
	if got.UpdatedAt.Before(before) || got.UpdatedAt.After(after) {
		t.Fatalf("UpdatedAt not auto-set properly: %v", got.UpdatedAt)
	}
}

func TestStore_FilterBySupportedModel(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, newTestProviderInfo("kiro", "team-a", account.ProviderStatusActive, 5, 10, []string{"model-a", "model-b"}))
	_ = store.AddProvider(ctx, newTestProviderInfo("kiro", "team-b", account.ProviderStatusActive, 3, 5, []string{"model-b", "model-c"}))
	_ = store.AddProvider(ctx, newTestProviderInfo("openai", "default", account.ProviderStatusActive, 8, 20, []string{"model-c"}))

	// 通过 filtercond 查 supported_model = "model-a"
	result, err := store.SearchProviders(ctx, &storage.SearchFilter{
		ExtraCond: filtercond.Equal("supported_model", "model-a"),
	})
	if err != nil {
		t.Fatalf("List(supported_model=model-a) failed: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1, got %d", len(result))
	}

	// 通过 filtercond 查 supported_model in ["model-a", "model-c"]
	result2, err := store.SearchProviders(ctx, &storage.SearchFilter{
		ExtraCond: filtercond.In("supported_model", "model-a", "model-c"),
	})
	if err != nil {
		t.Fatalf("List(supported_model in) failed: %v", err)
	}
	if len(result2) != 3 {
		t.Fatalf("expected 3, got %d", len(result2))
	}
}

func TestStore_CombinedFilter(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_ = store.AddProvider(ctx, newTestProviderInfo("kiro", "team-a", account.ProviderStatusActive, 5, 10, []string{"model-a"}))
	_ = store.AddProvider(ctx, newTestProviderInfo("kiro", "team-b", account.ProviderStatusDisabled, 3, 5, []string{"model-a"}))
	_ = store.AddProvider(ctx, newTestProviderInfo("openai", "default", account.ProviderStatusActive, 8, 20, []string{"model-b"}))

	// And: active + 支持 model-a
	result, err := store.SearchProviders(ctx, &storage.SearchFilter{
		Status:    int(account.ProviderStatusActive),
		ExtraCond: filtercond.Equal("supported_model", "model-a"),
	})
	if err != nil {
		t.Fatalf("List(and) failed: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1, got %d", len(result))
	}
	if result[0].ProviderName != "team-a" {
		t.Fatalf("expected team-a, got %s", result[0].ProviderName)
	}

	// Or: weight >= 20 或 priority >= 5
	orResult, err := store.SearchProviders(ctx, &storage.SearchFilter{
		ExtraCond: filtercond.Or(
			filtercond.GreaterThanOrEqual("weight", 20),
			filtercond.GreaterThanOrEqual("priority", 5),
		),
	})
	if err != nil {
		t.Fatalf("List(or) failed: %v", err)
	}
	if len(orResult) != 2 {
		t.Fatalf("expected 2, got %d", len(orResult))
	}
}

func TestStore_InvalidField(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	_, err := store.SearchProviders(ctx, &storage.SearchFilter{
		ExtraCond: filtercond.Equal("nonexistent_field", "value"),
	})
	if err == nil {
		t.Fatal("expected error for invalid field, got nil")
	}
}
