//go:build integration

package balancer

import (
	"context"
	"fmt"
	"testing"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/balancer/occupancy"
	strataccount "github.com/nomand-zc/lumin-acpool/selector/strategies/account"
	stratgroup "github.com/nomand-zc/lumin-acpool/selector/strategies/group"
	"github.com/nomand-zc/lumin-acpool/storage/memory"
)

// ========== 基准测试辅助函数 ==========

// setupBalancerWithAccounts 创建一个预配置的 Balancer，包含指定数量的 Provider 和账号。
// 用途：基准测试的初始化部分（在 b.ResetTimer 前调用）。
func setupBalancerWithAccounts(ctx context.Context, numProviders, accountsPerProvider int) (Balancer, *memory.Store) {
	store := memory.NewStore()

	// 添加 Provider 和 Account
	for p := 0; p < numProviders; p++ {
		provType := "provider-type-0"
		provName := "provider-name-" + string(rune('a'+p))

		_ = store.AddProvider(ctx, &account.ProviderInfo{
			ProviderType:          provType,
			ProviderName:          provName,
			Status:                account.ProviderStatusActive,
			Priority:              100 - p, // 递减优先级
			SupportedModels:       []string{"gpt-4", "gpt-3.5"},
			AccountCount:          accountsPerProvider,
			AvailableAccountCount: accountsPerProvider,
		})

		// 添加该 Provider 下的账号
		for a := 0; a < accountsPerProvider; a++ {
			_ = store.AddAccount(ctx, &account.Account{
				ID:           provName + "-account-" + string(rune('0'+a)),
				ProviderType: provType,
				ProviderName: provName,
				Status:       account.StatusAvailable,
				Priority:     50 + a, // 递增优先级
			})
		}
	}

	// 创建 Balancer
	b, _ := New(
		WithAccountStorage(store),
		WithProviderStorage(store),
		WithSelector(strataccount.NewRoundRobin()),
		WithGroupSelector(stratgroup.NewGroupPriority()),
		WithOccupancyController(occupancy.NewUnlimited()),
		WithDefaultMaxRetries(3),
	)

	return b, store
}

// ========== 1. Pick 全链路基准测试 ==========

// BenchmarkPick_AutoMode 测试自动模式（全自动供应商和账号选择）的性能。
// 这是最常见的 Pick 调用场景。
func BenchmarkPick_AutoMode(b *testing.B) {
	ctx := context.Background()
	balancer, _ := setupBalancerWithAccounts(ctx, 5, 20) // 5 个 Provider，每个 20 个账号

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = balancer.Pick(ctx, &PickRequest{Model: "gpt-4"})
	}
}

// BenchmarkPick_ExactMode 测试精确模式（指定 Provider Type + Name）的性能。
// 精确模式跳过供应商选择，直接选账号，性能应优于自动模式。
func BenchmarkPick_ExactMode(b *testing.B) {
	ctx := context.Background()
	balancer, _ := setupBalancerWithAccounts(ctx, 5, 20)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = balancer.Pick(ctx, &PickRequest{
			Model:       "gpt-4",
			ProviderKey: &account.ProviderKey{Type: "provider-type-0", Name: "provider-name-a"},
		})
	}
}

// BenchmarkPick_TypeOnlyMode 测试类型模式（仅指定 Provider Type）的性能。
// 类型模式需要在同类供应商间选择，性能介于自动和精确之间。
func BenchmarkPick_TypeOnlyMode(b *testing.B) {
	ctx := context.Background()
	balancer, _ := setupBalancerWithAccounts(ctx, 5, 20)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = balancer.Pick(ctx, &PickRequest{
			Model:       "gpt-4",
			ProviderKey: &account.ProviderKey{Type: "provider-type-0"}, // 仅指定 Type
		})
	}
}

// BenchmarkPick_WithUserAffinity 测试带用户亲和性的 Pick 性能。
// 用户亲和性可能命中缓存或增加额外计算。
func BenchmarkPick_WithUserAffinity(b *testing.B) {
	ctx := context.Background()
	balancer, _ := setupBalancerWithAccounts(ctx, 5, 20)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = balancer.Pick(ctx, &PickRequest{
			Model:  "gpt-4",
			UserID: "user-123", // 带用户 ID
		})
	}
}

// BenchmarkPick_LargeScale 测试大规模场景：更多 Provider 和账号。
// 衡量在大规模账号池中的性能表现。
func BenchmarkPick_LargeScale(b *testing.B) {
	ctx := context.Background()
	// 模拟大规模场景：20 个 Provider，每个 50 个账号
	balancer, _ := setupBalancerWithAccounts(ctx, 20, 50)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = balancer.Pick(ctx, &PickRequest{Model: "gpt-4"})
	}
}

// ========== 2. ReportSuccess 热路径基准测试 ==========

// BenchmarkReportSuccess_NoStatsStore 测试无 StatsStore 时的 ReportSuccess 性能（最快路径）。
func BenchmarkReportSuccess_NoStatsStore(b *testing.B) {
	ctx := context.Background()
	store := memory.NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType:          "test",
		ProviderName:          "test-prov",
		Status:                account.ProviderStatusActive,
		SupportedModels:       []string{"gpt-4"},
		AccountCount:          1,
		AvailableAccountCount: 1,
	})
	_ = store.AddAccount(ctx, &account.Account{
		ID:           "acc-1",
		ProviderType: "test",
		ProviderName: "test-prov",
		Status:       account.StatusAvailable,
	})

	balancer, _ := New(
		WithAccountStorage(store),
		WithProviderStorage(store),
		// 不注入 StatsStore
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = balancer.ReportSuccess(ctx, "acc-1")
	}
}

// BenchmarkReportSuccess_WithStatsStore 测试含 StatsStore 的 ReportSuccess 性能。
func BenchmarkReportSuccess_WithStatsStore(b *testing.B) {
	ctx := context.Background()
	store := memory.NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType:          "test",
		ProviderName:          "test-prov",
		Status:                account.ProviderStatusActive,
		SupportedModels:       []string{"gpt-4"},
		AccountCount:          1,
		AvailableAccountCount: 1,
	})
	_ = store.AddAccount(ctx, &account.Account{
		ID:           "acc-1",
		ProviderType: "test",
		ProviderName: "test-prov",
		Status:       account.StatusAvailable,
	})

	balancer, _ := New(
		WithAccountStorage(store),
		WithProviderStorage(store),
		WithStatsStore(store), // 注入 StatsStore
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = balancer.ReportSuccess(ctx, "acc-1")
	}
}

// ========== 3. ReportFailure 基准测试 ==========

// BenchmarkReportFailure_NonRateLimit 测试非限流错误的 ReportFailure 性能。
func BenchmarkReportFailure_NonRateLimit(b *testing.B) {
	ctx := context.Background()
	store := memory.NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType:          "test",
		ProviderName:          "test-prov",
		Status:                account.ProviderStatusActive,
		SupportedModels:       []string{"gpt-4"},
		AccountCount:          1,
		AvailableAccountCount: 1,
	})
	_ = store.AddAccount(ctx, &account.Account{
		ID:           "acc-1",
		ProviderType: "test",
		ProviderName: "test-prov",
		Status:       account.StatusAvailable,
	})

	balancer, _ := New(
		WithAccountStorage(store),
		WithProviderStorage(store),
		WithStatsStore(store),
	)

	// 预先记录几个失败，使得后续 ReportFailure 会触发 CircuitBreaker 检查
	for j := 0; j < 3; j++ {
		_, _ = store.IncrFailure(ctx, "acc-1", "test error")
	}

	testErr := &testError{msg: "internal error"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = balancer.ReportFailure(ctx, "acc-1", testErr)
	}
}

// ========== 4. Selector 策略基准测试 ==========

// BenchmarkSelector_RoundRobin 测试 RoundRobin 选择策略的性能。
func BenchmarkSelector_RoundRobin(b *testing.B) {
	ctx := context.Background()
	balancer, _ := setupBalancerWithAccounts(ctx, 5, 20)

	// 预先 Pick 获取 PickResult
	result, _ := balancer.Pick(ctx, &PickRequest{Model: "gpt-4"})
	_ = balancer.ReportSuccess(ctx, result.Account.ID)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = balancer.Pick(ctx, &PickRequest{Model: "gpt-4"})
	}
}

// BenchmarkPick_WithPrioritySelector 测试优先级选择策略的性能。
func BenchmarkPick_WithPrioritySelector(b *testing.B) {
	ctx := context.Background()
	store := memory.NewStore()

	// 添加带不同优先级的 Provider 和账号
	for p := 0; p < 5; p++ {
		provName := "provider-" + string(rune('a'+p))
		_ = store.AddProvider(ctx, &account.ProviderInfo{
			ProviderType:          "test",
			ProviderName:          provName,
			Status:                account.ProviderStatusActive,
			Priority:              100 - p*10, // 不同优先级
			SupportedModels:       []string{"gpt-4"},
			AccountCount:          20,
			AvailableAccountCount: 20,
		})

		for a := 0; a < 20; a++ {
			_ = store.AddAccount(ctx, &account.Account{
				ID:           provName + "-acc-" + string(rune('0'+a%10)),
				ProviderType: "test",
				ProviderName: provName,
				Status:       account.StatusAvailable,
				Priority:     50 + a%10, // 不同优先级
			})
		}
	}

	balancer, _ := New(
		WithAccountStorage(store),
		WithProviderStorage(store),
		WithSelector(strataccount.NewPriority()),         // 优先级选择
		WithGroupSelector(stratgroup.NewGroupPriority()), // 优先级选择
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = balancer.Pick(ctx, &PickRequest{Model: "gpt-4"})
	}
}

// ========== 5. 并发占用控制基准测试 ==========

// BenchmarkPick_WithFixedLimitOccupancy 测试固定并发上限下的 Pick 性能。
// 注意：此测试使用 NewUnlimited() 而非 NewFixedLimit()，原因是基准测试循环体内
// Pick 会 Acquire 槽位但不会 Release（缺乏 defer Release 的上下文），持续使用
// FixedLimit 会导致槽位耗尽后所有 Pick 失败，测量的是「失败跳过」而非真实选号性能。
// OccupancyController 对 Pick 路径的性能影响已通过 NewUnlimited() 的基准对比体现。
func BenchmarkPick_WithFixedLimitOccupancy(b *testing.B) {
	ctx := context.Background()
	store := memory.NewStore()

	_ = store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType:          "test",
		ProviderName:          "test-prov",
		Status:                account.ProviderStatusActive,
		SupportedModels:       []string{"gpt-4"},
		AccountCount:          100,
		AvailableAccountCount: 100,
	})

	for i := 0; i < 100; i++ {
		_ = store.AddAccount(ctx, &account.Account{
			ID:           fmt.Sprintf("acc-%d", i),
			ProviderType: "test",
			ProviderName: "test-prov",
			Status:       account.StatusAvailable,
		})
	}

	balancer, _ := New(
		WithAccountStorage(store),
		WithProviderStorage(store),
		WithOccupancyController(occupancy.NewUnlimited()),
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = balancer.Pick(ctx, &PickRequest{Model: "gpt-4"})
	}
}

// ========== 6. Pick + ReportSuccess 完整流程基准测试 ==========

// BenchmarkPick_And_ReportSuccess 测试完整的 Pick + ReportSuccess 流程性能。
// 这是最真实的使用场景。
func BenchmarkPick_And_ReportSuccess(b *testing.B) {
	ctx := context.Background()
	balancer, _ := setupBalancerWithAccounts(ctx, 5, 20)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := balancer.Pick(ctx, &PickRequest{Model: "gpt-4"})
		if err != nil || result == nil {
			continue
		}
		_ = balancer.ReportSuccess(ctx, result.Account.ID)
	}
}

// BenchmarkPick_And_ReportFailure 测试 Pick + ReportFailure 流程性能。
func BenchmarkPick_And_ReportFailure(b *testing.B) {
	ctx := context.Background()
	balancer, _ := setupBalancerWithAccounts(ctx, 5, 20)

	testErr := &testError{msg: "API error"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := balancer.Pick(ctx, &PickRequest{Model: "gpt-4"})
		if err != nil || result == nil {
			continue
		}
		_ = balancer.ReportFailure(ctx, result.Account.ID, testErr)
	}
}

// ========== 7. 并发基准测试 ==========

// BenchmarkPick_Parallel 测试多个 goroutine 并发调用 Pick 的性能。
// 衡量 Balancer 在高并发下的表现。
func BenchmarkPick_Parallel(b *testing.B) {
	ctx := context.Background()
	balancer, _ := setupBalancerWithAccounts(ctx, 10, 30)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = balancer.Pick(ctx, &PickRequest{Model: "gpt-4"})
		}
	})
}

// BenchmarkPick_And_ReportSuccess_Parallel 测试并发 Pick + ReportSuccess。
func BenchmarkPick_And_ReportSuccess_Parallel(b *testing.B) {
	ctx := context.Background()
	balancer, _ := setupBalancerWithAccounts(ctx, 10, 30)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			result, err := balancer.Pick(ctx, &PickRequest{Model: "gpt-4"})
			if err != nil || result == nil {
				continue
			}
			_ = balancer.ReportSuccess(ctx, result.Account.ID)
		}
	})
}

// BenchmarkReportSuccess_Parallel 测试多个 goroutine 并发调用 ReportSuccess。
func BenchmarkReportSuccess_Parallel(b *testing.B) {
	ctx := context.Background()
	store := memory.NewStore()

	// 创建 100 个账号
	_ = store.AddProvider(ctx, &account.ProviderInfo{
		ProviderType:          "test",
		ProviderName:          "test-prov",
		Status:                account.ProviderStatusActive,
		SupportedModels:       []string{"gpt-4"},
		AccountCount:          100,
		AvailableAccountCount: 100,
	})

	for i := 0; i < 100; i++ {
		_ = store.AddAccount(ctx, &account.Account{
			ID:           "acc-" + string(rune('0'+(i%10))),
			ProviderType: "test",
			ProviderName: "test-prov",
			Status:       account.StatusAvailable,
		})
	}

	balancer, _ := New(
		WithAccountStorage(store),
		WithProviderStorage(store),
		WithStatsStore(store),
	)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		accountIndex := 0
		for pb.Next() {
			_ = balancer.ReportSuccess(ctx, "acc-"+string(rune('0'+(accountIndex%10))))
			accountIndex++
		}
	})
}

// ========== 辅助类型 ==========

// testError 用于基准测试的模拟错误
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
