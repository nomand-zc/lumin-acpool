//go:build integration

package account

import (
	"context"
	"fmt"
	"testing"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/selector"
	storememory "github.com/nomand-zc/lumin-acpool/storage/memory"
)

// ========== 基准测试辅助函数 ==========

// createCandidateAccounts 创建指定数量的测试账号。
func createCandidateAccounts(count int) []*account.Account {
	accounts := make([]*account.Account, count)
	for i := 0; i < count; i++ {
		accounts[i] = &account.Account{
			ID:           fmt.Sprintf("acc-%d", i),
			ProviderType: "test",
			ProviderName: fmt.Sprintf("provider-%c", rune('a'+(i/10)%26)),
			Status:       account.StatusAvailable,
			Priority:     (count - i) % 100, // 递减优先级
		}
	}
	return accounts
}

// createAccountsWithStats 创建带统计信息的账号，每个账号有独立且递增的调用次数。
func createAccountsWithStats(store *storememory.Store, count int) []*account.Account {
	accounts := make([]*account.Account, count)

	for i := 0; i < count; i++ {
		accounts[i] = &account.Account{
			ID:           fmt.Sprintf("acc-%d", i),
			ProviderType: "test",
			ProviderName: fmt.Sprintf("provider-%c", rune('a'+(i/10)%26)),
			Status:       account.StatusAvailable,
			Priority:     (count - i) % 100,
		}

		// 向 StatsStore 记录不同的调用次数，模拟各账号历史使用量不同
		for j := 0; j < i; j++ {
			store.IncrSuccess(context.Background(), accounts[i].ID)
		}
	}
	return accounts
}

// ========== 1. RoundRobin 基准测试 ==========

// BenchmarkRoundRobin_Small 测试轮转选择在小候选集上的性能。
func BenchmarkRoundRobin_Small(b *testing.B) {
	rr := NewRoundRobin()
	candidates := createCandidateAccounts(5) // 5 个候选账号

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = rr.Select(candidates, nil)
	}
}

// BenchmarkRoundRobin_Medium 测试轮转选择在中等候选集上的性能。
func BenchmarkRoundRobin_Medium(b *testing.B) {
	rr := NewRoundRobin()
	candidates := createCandidateAccounts(50) // 50 个候选账号

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = rr.Select(candidates, nil)
	}
}

// BenchmarkRoundRobin_Large 测试轮转选择在大候选集上的性能。
func BenchmarkRoundRobin_Large(b *testing.B) {
	rr := NewRoundRobin()
	candidates := createCandidateAccounts(500) // 500 个候选账号

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = rr.Select(candidates, nil)
	}
}

// BenchmarkRoundRobin_Parallel 测试轮转选择在并发场景下的性能。
func BenchmarkRoundRobin_Parallel(b *testing.B) {
	rr := NewRoundRobin()
	candidates := createCandidateAccounts(100)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = rr.Select(candidates, nil)
		}
	})
}

// ========== 2. Priority 基准测试 ==========

// BenchmarkPriority_Small 测试优先级选择在小候选集上的性能。
func BenchmarkPriority_Small(b *testing.B) {
	p := NewPriority()
	candidates := createCandidateAccounts(5)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = p.Select(candidates, nil)
	}
}

// BenchmarkPriority_Medium 测试优先级选择在中等候选集上的性能。
func BenchmarkPriority_Medium(b *testing.B) {
	p := NewPriority()
	candidates := createCandidateAccounts(50)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = p.Select(candidates, nil)
	}
}

// BenchmarkPriority_Large 测试优先级选择在大候选集上的性能。
func BenchmarkPriority_Large(b *testing.B) {
	p := NewPriority()
	candidates := createCandidateAccounts(500)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = p.Select(candidates, nil)
	}
}

// BenchmarkPriority_Parallel 测试优先级选择在并发场景下的性能。
func BenchmarkPriority_Parallel(b *testing.B) {
	p := NewPriority()
	candidates := createCandidateAccounts(100)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = p.Select(candidates, nil)
		}
	})
}

// ========== 3. Weighted 基准测试 ==========

// BenchmarkWeighted_Small 测试加权随机选择在小候选集上的性能。
func BenchmarkWeighted_Small(b *testing.B) {
	w := NewWeighted()
	candidates := createCandidateAccounts(5)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = w.Select(candidates, nil)
	}
}

// BenchmarkWeighted_Medium 测试加权随机选择在中等候选集上的性能。
func BenchmarkWeighted_Medium(b *testing.B) {
	w := NewWeighted()
	candidates := createCandidateAccounts(50)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = w.Select(candidates, nil)
	}
}

// BenchmarkWeighted_Large 测试加权随机选择在大候选集上的性能。
func BenchmarkWeighted_Large(b *testing.B) {
	w := NewWeighted()
	candidates := createCandidateAccounts(500)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = w.Select(candidates, nil)
	}
}

// BenchmarkWeighted_Parallel 测试加权随机选择在并发场景下的性能。
func BenchmarkWeighted_Parallel(b *testing.B) {
	w := NewWeighted()
	candidates := createCandidateAccounts(100)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = w.Select(candidates, nil)
		}
	})
}

// ========== 4. LeastUsed 基准测试 ==========

// BenchmarkLeastUsed_NoStatsStore 测试 LeastUsed 在无 StatsStore 时的性能（退化为 Priority）。
func BenchmarkLeastUsed_NoStatsStore(b *testing.B) {
	lu := NewLeastUsed(nil) // 无 StatsStore，退化为 Priority
	candidates := createCandidateAccounts(50)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = lu.Select(candidates, nil)
	}
}

// BenchmarkLeastUsed_WithStatsStore 测试 LeastUsed 在有 StatsStore 时的性能。
func BenchmarkLeastUsed_WithStatsStore(b *testing.B) {
	store := storememory.NewStore()
	candidates := createAccountsWithStats(store, 50)
	lu := NewLeastUsed(store)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = lu.Select(candidates, nil)
	}
}

// BenchmarkLeastUsed_Parallel 测试 LeastUsed 在并发场景下的性能。
func BenchmarkLeastUsed_Parallel(b *testing.B) {
	store := storememory.NewStore()
	candidates := createAccountsWithStats(store, 100)
	lu := NewLeastUsed(store)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = lu.Select(candidates, nil)
		}
	})
}

// ========== 5. Affinity 基准测试 ==========

// BenchmarkAffinity_HitRate_Zero 测试 Affinity 在零命中率下的性能（每次都要更新映射）。
func BenchmarkAffinity_HitRate_Zero(b *testing.B) {
	store := storememory.NewStore()
	candidates := createCandidateAccounts(50)

	affinity := NewAffinity(
		AffinityWithStore(store),
		AffinityWithFallback(NewRoundRobin()),
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

// BenchmarkAffinity_HitRate_High 测试 Affinity 在高命中率下的性能（命中缓存）。
func BenchmarkAffinity_HitRate_High(b *testing.B) {
	store := storememory.NewStore()
	candidates := createCandidateAccounts(50)

	affinity := NewAffinity(
		AffinityWithStore(store),
		AffinityWithFallback(NewRoundRobin()),
	)

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

// BenchmarkAffinity_EmptyUserID 测试 Affinity 在空 UserID 下的性能（退化为 Fallback）。
func BenchmarkAffinity_EmptyUserID(b *testing.B) {
	store := storememory.NewStore()
	candidates := createCandidateAccounts(50)

	affinity := NewAffinity(
		AffinityWithStore(store),
		AffinityWithFallback(NewRoundRobin()),
	)

	req := &selector.SelectRequest{
		UserID: "",
		Model:  "gpt-4",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = affinity.Select(candidates, req)
	}
}

// BenchmarkAffinity_Parallel 测试 Affinity 在并发场景下的性能（多个 UserID 并发访问）。
func BenchmarkAffinity_Parallel(b *testing.B) {
	store := storememory.NewStore()
	candidates := createCandidateAccounts(100)

	affinity := NewAffinity(
		AffinityWithStore(store),
		AffinityWithFallback(NewRoundRobin()),
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

// BenchmarkAllStrategies_50Candidates 对比所有账号级策略在 50 个候选的性能。
func BenchmarkAllStrategies_50Candidates(b *testing.B) {
	candidates := createCandidateAccounts(50)

	b.Run("RoundRobin", func(b *testing.B) {
		rr := NewRoundRobin()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = rr.Select(candidates, nil)
		}
	})

	b.Run("Priority", func(b *testing.B) {
		p := NewPriority()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = p.Select(candidates, nil)
		}
	})

	b.Run("Weighted", func(b *testing.B) {
		w := NewWeighted()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = w.Select(candidates, nil)
		}
	})

	b.Run("LeastUsed", func(b *testing.B) {
		lu := NewLeastUsed(nil)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = lu.Select(candidates, nil)
		}
	})

	b.Run("Affinity-HitRate-0", func(b *testing.B) {
		store := storememory.NewStore()
		affinity := NewAffinity(
			AffinityWithStore(store),
			AffinityWithFallback(NewRoundRobin()),
		)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			req := &selector.SelectRequest{UserID: "user-" + string(rune('0'+(i%100))), Model: "gpt-4"}
			_, _ = affinity.Select(candidates, req)
		}
	})

	b.Run("Affinity-HitRate-100", func(b *testing.B) {
		store := storememory.NewStore()
		affinity := NewAffinity(
			AffinityWithStore(store),
			AffinityWithFallback(NewRoundRobin()),
		)
		req := &selector.SelectRequest{UserID: "user-1", Model: "gpt-4"}
		_, _ = affinity.Select(candidates, req)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = affinity.Select(candidates, req)
		}
	})
}

// BenchmarkAllStrategies_500Candidates 对比所有账号级策略在 500 个候选的性能。
func BenchmarkAllStrategies_500Candidates(b *testing.B) {
	candidates := createCandidateAccounts(500)

	b.Run("RoundRobin", func(b *testing.B) {
		rr := NewRoundRobin()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = rr.Select(candidates, nil)
		}
	})

	b.Run("Priority", func(b *testing.B) {
		p := NewPriority()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = p.Select(candidates, nil)
		}
	})

	b.Run("Weighted", func(b *testing.B) {
		w := NewWeighted()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = w.Select(candidates, nil)
		}
	})

	b.Run("LeastUsed", func(b *testing.B) {
		lu := NewLeastUsed(nil)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = lu.Select(candidates, nil)
		}
	})

	b.Run("Affinity-HitRate-0", func(b *testing.B) {
		store := storememory.NewStore()
		affinity := NewAffinity(
			AffinityWithStore(store),
			AffinityWithFallback(NewRoundRobin()),
		)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			req := &selector.SelectRequest{UserID: "user-" + string(rune('0'+(i%100))), Model: "gpt-4"}
			_, _ = affinity.Select(candidates, req)
		}
	})

	b.Run("Affinity-HitRate-100", func(b *testing.B) {
		store := storememory.NewStore()
		affinity := NewAffinity(
			AffinityWithStore(store),
			AffinityWithFallback(NewRoundRobin()),
		)
		req := &selector.SelectRequest{UserID: "user-1", Model: "gpt-4"}
		_, _ = affinity.Select(candidates, req)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = affinity.Select(candidates, req)
		}
	})
}
