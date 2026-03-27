//go:build integration

package group

import (
	"context"
	"testing"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/selector"
	storememory "github.com/nomand-zc/lumin-acpool/storage/memory"
)

// ========== 基准测试辅助函数 ==========

// createCandidateProviders 创建指定数量的测试 Provider。
func createCandidateProviders(count int) []*account.ProviderInfo {
	providers := make([]*account.ProviderInfo, count)
	for i := 0; i < count; i++ {
		providers[i] = &account.ProviderInfo{
			ProviderType:          "provider-type-0",
			ProviderName:          "provider-" + string(rune('a'+(i%26))),
			Status:                account.ProviderStatusActive,
			Priority:              (count - i) % 100, // 递减优先级
			Weight:                (i % 10) + 1,
			AccountCount:          (i%50) + 1,
			AvailableAccountCount: (i % 30) + 1,
		}
	}
	return providers
}

// createProvidersWithAffinity 创建带亲和绑定的 Provider 列表。
func createProvidersWithAffinity(store *storememory.Store, count int) []*account.ProviderInfo {
	providers := createCandidateProviders(count)

	// 为每个 Provider 创建亲和绑定
	for _, p := range providers {
		key := p.ProviderKey()
		store.SetAffinity(key.String(), p.ProviderName) // 简化示例
	}

	return providers
}

// ========== 1. GroupRoundRobin 基准测试 ==========

// BenchmarkGroupRoundRobin_Small 测试轮转选择在小 Provider 集上的性能。
func BenchmarkGroupRoundRobin_Small(b *testing.B) {
	grr := NewGroupRoundRobin()
	candidates := createCandidateProviders(5) // 5 个候选 Provider

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = grr.Select(candidates, nil)
	}
}

// BenchmarkGroupRoundRobin_Medium 测试轮转选择在中等 Provider 集上的性能。
func BenchmarkGroupRoundRobin_Medium(b *testing.B) {
	grr := NewGroupRoundRobin()
	candidates := createCandidateProviders(50) // 50 个候选 Provider

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = grr.Select(candidates, nil)
	}
}

// BenchmarkGroupRoundRobin_Large 测试轮转选择在大 Provider 集上的性能。
func BenchmarkGroupRoundRobin_Large(b *testing.B) {
	grr := NewGroupRoundRobin()
	candidates := createCandidateProviders(200) // 200 个候选 Provider

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = grr.Select(candidates, nil)
	}
}

// BenchmarkGroupRoundRobin_Parallel 测试轮转选择在并发场景下的性能。
func BenchmarkGroupRoundRobin_Parallel(b *testing.B) {
	grr := NewGroupRoundRobin()
	candidates := createCandidateProviders(100)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = grr.Select(candidates, nil)
		}
	})
}

// ========== 2. GroupPriority 基准测试 ==========

// BenchmarkGroupPriority_Small 测试优先级选择在小 Provider 集上的性能。
func BenchmarkGroupPriority_Small(b *testing.B) {
	gp := NewGroupPriority()
	candidates := createCandidateProviders(5)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = gp.Select(candidates, nil)
	}
}

// BenchmarkGroupPriority_Medium 测试优先级选择在中等 Provider 集上的性能。
func BenchmarkGroupPriority_Medium(b *testing.B) {
	gp := NewGroupPriority()
	candidates := createCandidateProviders(50)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = gp.Select(candidates, nil)
	}
}

// BenchmarkGroupPriority_Large 测试优先级选择在大 Provider 集上的性能。
// GroupPriority 需要排序，大规模时性能下降明显。
func BenchmarkGroupPriority_Large(b *testing.B) {
	gp := NewGroupPriority()
	candidates := createCandidateProviders(200)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = gp.Select(candidates, nil)
	}
}

// BenchmarkGroupPriority_Parallel 测试优先级选择在并发场景下的性能。
func BenchmarkGroupPriority_Parallel(b *testing.B) {
	gp := NewGroupPriority()
	candidates := createCandidateProviders(100)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = gp.Select(candidates, nil)
		}
	})
}

// ========== 3. GroupWeighted 基准测试 ==========

// BenchmarkGroupWeighted_Small 测试加权随机选择在小 Provider 集上的性能。
func BenchmarkGroupWeighted_Small(b *testing.B) {
	gw := NewGroupWeighted()
	candidates := createCandidateProviders(5)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = gw.Select(candidates, nil)
	}
}

// BenchmarkGroupWeighted_Medium 测试加权随机选择在中等 Provider 集上的性能。
func BenchmarkGroupWeighted_Medium(b *testing.B) {
	gw := NewGroupWeighted()
	candidates := createCandidateProviders(50)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = gw.Select(candidates, nil)
	}
}

// BenchmarkGroupWeighted_Large 测试加权随机选择在大 Provider 集上的性能。
func BenchmarkGroupWeighted_Large(b *testing.B) {
	gw := NewGroupWeighted()
	candidates := createCandidateProviders(200)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = gw.Select(candidates, nil)
	}
}

// BenchmarkGroupWeighted_Parallel 测试加权随机选择在并发场景下的性能。
func BenchmarkGroupWeighted_Parallel(b *testing.B) {
	gw := NewGroupWeighted()
	candidates := createCandidateProviders(100)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = gw.Select(candidates, nil)
		}
	})
}

// ========== 4. GroupMostAvailable 基准测试 ==========

// BenchmarkGroupMostAvailable_Small 测试最多可用账号策略在小 Provider 集上的性能。
func BenchmarkGroupMostAvailable_Small(b *testing.B) {
	gma := NewGroupMostAvailable()
	candidates := createCandidateProviders(5)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = gma.Select(candidates, nil)
	}
}

// BenchmarkGroupMostAvailable_Medium 测试最多可用账号策略在中等 Provider 集上的性能。
func BenchmarkGroupMostAvailable_Medium(b *testing.B) {
	gma := NewGroupMostAvailable()
	candidates := createCandidateProviders(50)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = gma.Select(candidates, nil)
	}
}

// BenchmarkGroupMostAvailable_Large 测试最多可用账号策略在大 Provider 集上的性能。
func BenchmarkGroupMostAvailable_Large(b *testing.B) {
	gma := NewGroupMostAvailable()
	candidates := createCandidateProviders(200)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = gma.Select(candidates, nil)
	}
}

// BenchmarkGroupMostAvailable_Parallel 测试最多可用账号策略在并发场景下的性能。
func BenchmarkGroupMostAvailable_Parallel(b *testing.B) {
	gma := NewGroupMostAvailable()
	candidates := createCandidateProviders(100)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = gma.Select(candidates, nil)
		}
	})
}

// ========== 5. GroupAffinity 基准测试 ==========

// BenchmarkGroupAffinity_HitRate_Zero 测试 GroupAffinity 在零命中率下的性能。
func BenchmarkGroupAffinity_HitRate_Zero(b *testing.B) {
	store := storememory.NewStore()
	candidates := createCandidateProviders(50)

	affinity := NewGroupAffinity(
		GroupAffinityWithStore(store),
		GroupAffinityWithFallback(NewGroupPriority()),
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := &selector.SelectRequest{
			UserID: "user-" + string(rune('0'+(i%100))),
			Model:  "gpt-4",
		}
		_, _ = affinity.Select(candidates, req)
	}
}

// BenchmarkGroupAffinity_HitRate_High 测试 GroupAffinity 在高命中率下的性能。
func BenchmarkGroupAffinity_HitRate_High(b *testing.B) {
	store := storememory.NewStore()
	candidates := createCandidateProviders(50)

	affinity := NewGroupAffinity(
		GroupAffinityWithStore(store),
		GroupAffinityWithFallback(NewGroupPriority()),
	)

	// 预热
	req := &selector.SelectRequest{
		UserID: "user-1",
		Model:  "gpt-4",
	}
	_, _ = affinity.Select(candidates, req)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = affinity.Select(candidates, req)
	}
}

// BenchmarkGroupAffinity_Parallel 测试 GroupAffinity 在并发场景下的性能。
func BenchmarkGroupAffinity_Parallel(b *testing.B) {
	store := storememory.NewStore()
	candidates := createCandidateProviders(100)

	affinity := NewGroupAffinity(
		GroupAffinityWithStore(store),
		GroupAffinityWithFallback(NewGroupPriority()),
	)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		userID := 0
		for pb.Next() {
			req := &selector.SelectRequest{
				UserID: "user-" + string(rune('0'+(userID%20))),
				Model:  "gpt-4",
			}
			_, _ = affinity.Select(candidates, req)
			userID++
		}
	})
}

// ========== 6. 策略对比基准测试 ==========

// BenchmarkAllGroupStrategies_50Candidates 对比所有 GroupSelector 策略在 50 个 Provider 上的性能。
func BenchmarkAllGroupStrategies_50Candidates(b *testing.B) {
	candidates := createCandidateProviders(50)

	b.Run("GroupRoundRobin", func(b *testing.B) {
		grr := NewGroupRoundRobin()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = grr.Select(candidates, nil)
		}
	})

	b.Run("GroupPriority", func(b *testing.B) {
		gp := NewGroupPriority()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = gp.Select(candidates, nil)
		}
	})

	b.Run("GroupWeighted", func(b *testing.B) {
		gw := NewGroupWeighted()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = gw.Select(candidates, nil)
		}
	})

	b.Run("GroupMostAvailable", func(b *testing.B) {
		gma := NewGroupMostAvailable()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = gma.Select(candidates, nil)
		}
	})

	b.Run("GroupAffinity-HitRate-0", func(b *testing.B) {
		store := storememory.NewStore()
		affinity := NewGroupAffinity(
			GroupAffinityWithStore(store),
			GroupAffinityWithFallback(NewGroupPriority()),
		)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			req := &selector.SelectRequest{
				UserID: "user-" + string(rune('0'+(i%100))),
				Model:  "gpt-4",
			}
			_, _ = affinity.Select(candidates, req)
		}
	})

	b.Run("GroupAffinity-HitRate-100", func(b *testing.B) {
		store := storememory.NewStore()
		affinity := NewGroupAffinity(
			GroupAffinityWithStore(store),
			GroupAffinityWithFallback(NewGroupPriority()),
		)
		req := &selector.SelectRequest{UserID: "user-1", Model: "gpt-4"}
		// 预热
		_, _ = affinity.Select(candidates, req)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = affinity.Select(candidates, req)
		}
	})
}

// BenchmarkAllGroupStrategies_200Candidates 对比所有 GroupSelector 策略在 200 个 Provider 上的性能。
// 用于检验在大规模 Provider 集上各策略的性能表现，特别是排序相关策略的性能衰减。
func BenchmarkAllGroupStrategies_200Candidates(b *testing.B) {
	candidates := createCandidateProviders(200)

	b.Run("GroupRoundRobin", func(b *testing.B) {
		grr := NewGroupRoundRobin()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = grr.Select(candidates, nil)
		}
	})

	b.Run("GroupPriority", func(b *testing.B) {
		gp := NewGroupPriority()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = gp.Select(candidates, nil)
		}
	})

	b.Run("GroupWeighted", func(b *testing.B) {
		gw := NewGroupWeighted()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = gw.Select(candidates, nil)
		}
	})

	b.Run("GroupMostAvailable", func(b *testing.B) {
		gma := NewGroupMostAvailable()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = gma.Select(candidates, nil)
		}
	})
}

// ========== 7. 账号级 + 供应商级两层选择对比 ==========

// BenchmarkTwoLevelSelection 演示完整的两层选择流程。
// 第一层：GroupSelector 从 Provider 列表中选择一个
// 第二层：Selector 从选中的 Provider 的账号中选择一个
func BenchmarkTwoLevelSelection(b *testing.B) {
	ctx := context.Background()
	store := storememory.NewStore()

	// 添加 5 个 Provider，每个 20 个账号
	providers := createCandidateProviders(5)
	for _, p := range providers {
		_ = store.AddProvider(ctx, p)
		for i := 0; i < 20; i++ {
			_ = store.AddAccount(ctx, &account.Account{
				ID:           p.ProviderName + "-acc-" + string(rune('0'+i%10)),
				ProviderType: p.ProviderType,
				ProviderName: p.ProviderName,
				Status:       account.StatusAvailable,
				Priority:     i % 100,
			})
		}
	}

	b.Run("Priority+RoundRobin", func(b *testing.B) {
		gp := NewGroupPriority()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// 第一层：选择 Provider
			p, _ := gp.Select(providers, nil)
			// 第二层：从该 Provider 选择账号
			_ = p // 实际应用中查询 p 下的账号后选择
		}
	})

	b.Run("MostAvailable+Priority", func(b *testing.B) {
		gma := NewGroupMostAvailable()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			p, _ := gma.Select(providers, nil)
			_ = p
		}
	})
}
