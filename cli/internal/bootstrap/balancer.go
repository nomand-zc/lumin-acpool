package bootstrap

import (
	"fmt"

	"github.com/nomand-zc/lumin-acpool/balancer"
	"github.com/nomand-zc/lumin-acpool/balancer/occupancy"
	"github.com/nomand-zc/lumin-acpool/circuitbreaker"
	"github.com/nomand-zc/lumin-acpool/cli/internal/config"
	"github.com/nomand-zc/lumin-acpool/cooldown"
	"github.com/nomand-zc/lumin-acpool/selector"
	accountStrategies "github.com/nomand-zc/lumin-acpool/selector/strategies/account"
	groupStrategies "github.com/nomand-zc/lumin-acpool/selector/strategies/group"
	"github.com/nomand-zc/lumin-acpool/usagetracker"
)

// initBalancer 根据 BalancerConfig 构建 Balancer 及所有子组件。
// 初始化顺序：
//  1. Selector / GroupSelector
//  2. UsageTracker（Occupancy.Adaptive 依赖它）
//  3. CircuitBreaker / CooldownManager
//  4. OccupancyController
//  5. 组装 Balancer
func initBalancer(cfg config.BalancerConfig, deps *Dependencies) error {
	opts := []balancer.Option{
		balancer.WithAccountStorage(deps.Storage),
		balancer.WithProviderStorage(deps.Storage),
		balancer.WithDefaultMaxRetries(cfg.DefaultMaxRetries),
		balancer.WithDefaultFailover(cfg.DefaultEnableFailover),
	}

	// StatsStore
	if deps.Storage != nil {
		opts = append(opts, balancer.WithStatsStore(deps.Storage))
	}

	// ---- Selector ----
	sel, err := buildSelector(cfg.Selector, deps)
	if err != nil {
		return fmt.Errorf("selector: %w", err)
	}
	opts = append(opts, balancer.WithSelector(sel))

	// ---- GroupSelector ----
	gSel, err := buildGroupSelector(cfg.GroupSelector, deps)
	if err != nil {
		return fmt.Errorf("group_selector: %w", err)
	}
	opts = append(opts, balancer.WithGroupSelector(gSel))

	// ---- UsageTracker（必须在 Occupancy 之前，Adaptive 依赖它） ----
	if cfg.UsageTracker != nil {
		ut := buildUsageTracker(cfg.UsageTracker, deps)
		deps.UsageTracker = ut
		opts = append(opts, balancer.WithUsageTracker(ut))
	}

	// ---- CircuitBreaker ----
	if cfg.CircuitBreaker != nil {
		cb, err := buildCircuitBreaker(cfg.CircuitBreaker, deps)
		if err != nil {
			return fmt.Errorf("circuit_breaker: %w", err)
		}
		opts = append(opts, balancer.WithCircuitBreaker(cb))
	}

	// ---- Cooldown ----
	if cfg.Cooldown != nil {
		cm := buildCooldown(cfg.Cooldown)
		opts = append(opts, balancer.WithCooldownManager(cm))
	}

	// ---- Occupancy ----
	oc, err := buildOccupancy(cfg.Occupancy, deps)
	if err != nil {
		return fmt.Errorf("occupancy: %w", err)
	}
	opts = append(opts, balancer.WithOccupancyController(oc))

	// ---- 构建 Balancer ----
	b, err := balancer.New(opts...)
	if err != nil {
		return fmt.Errorf("create balancer: %w", err)
	}
	deps.Balancer = b

	return nil
}

// ===================== Selector 构建 =====================

// buildSelector 根据 TypedConfig 构建账号级选择器。
func buildSelector(tc *config.TypedConfig[config.SelectorStrategy], deps *Dependencies) (selector.Selector, error) {
	if tc == nil || tc.IsZero() {
		return accountStrategies.NewRoundRobin(), nil
	}

	switch tc.Strategy {
	case config.SelectorRoundRobin:
		return accountStrategies.NewRoundRobin(), nil

	case config.SelectorPriority:
		return accountStrategies.NewPriority(), nil

	case config.SelectorWeighted:
		return accountStrategies.NewWeighted(), nil

	case config.SelectorLeastUsed:
		return accountStrategies.NewLeastUsed(deps.Storage), nil

	case config.SelectorAffinity:
		var cfg config.AffinityConfig
		if err := tc.Decode(&cfg); err != nil {
			return nil, fmt.Errorf("decode affinity config: %w", err)
		}
		var opts []accountStrategies.AffinityOption
		if deps.Storage != nil {
			opts = append(opts, accountStrategies.AffinityWithStore(deps.Storage))
		}
		if cfg.Fallback != "" {
			fb, err := buildSimpleSelector(cfg.Fallback, deps)
			if err != nil {
				return nil, fmt.Errorf("build affinity fallback: %w", err)
			}
			opts = append(opts, accountStrategies.AffinityWithFallback(fb))
		}
		return accountStrategies.NewAffinity(opts...), nil

	default:
		return nil, fmt.Errorf("unknown selector strategy: %q", tc.Strategy)
	}
}

// buildSimpleSelector 构建无嵌套配置的简单选择器（用于 fallback，防止递归）。
func buildSimpleSelector(strategy config.SelectorStrategy, deps *Dependencies) (selector.Selector, error) {
	switch strategy {
	case config.SelectorRoundRobin, "":
		return accountStrategies.NewRoundRobin(), nil
	case config.SelectorPriority:
		return accountStrategies.NewPriority(), nil
	case config.SelectorWeighted:
		return accountStrategies.NewWeighted(), nil
	case config.SelectorLeastUsed:
		return accountStrategies.NewLeastUsed(deps.Storage), nil
	default:
		return nil, fmt.Errorf("unknown selector strategy for fallback: %q", strategy)
	}
}

// ===================== GroupSelector 构建 =====================

// buildGroupSelector 根据 TypedConfig 构建供应商级选择器。
func buildGroupSelector(tc *config.TypedConfig[config.GroupSelectorStrategy], deps *Dependencies) (selector.GroupSelector, error) {
	if tc == nil || tc.IsZero() {
		return groupStrategies.NewGroupPriority(), nil
	}

	switch tc.Strategy {
	case config.GroupSelectorPriority:
		return groupStrategies.NewGroupPriority(), nil

	case config.GroupSelectorRoundRobin:
		return groupStrategies.NewGroupRoundRobin(), nil

	case config.GroupSelectorWeighted:
		return groupStrategies.NewGroupWeighted(), nil

	case config.GroupSelectorMostAvailable:
		return groupStrategies.NewGroupMostAvailable(), nil

	case config.GroupSelectorAffinity:
		var cfg config.GroupAffinityConfig
		if err := tc.Decode(&cfg); err != nil {
			return nil, fmt.Errorf("decode group_affinity config: %w", err)
		}
		var opts []groupStrategies.GroupAffinityOption
		if deps.Storage != nil {
			opts = append(opts, groupStrategies.GroupAffinityWithStore(deps.Storage))
		}
		if cfg.Fallback != "" {
			fb, err := buildSimpleGroupSelector(cfg.Fallback)
			if err != nil {
				return nil, fmt.Errorf("build group_affinity fallback: %w", err)
			}
			opts = append(opts, groupStrategies.GroupAffinityWithFallback(fb))
		}
		return groupStrategies.NewGroupAffinity(opts...), nil

	default:
		return nil, fmt.Errorf("unknown group_selector strategy: %q", tc.Strategy)
	}
}

// buildSimpleGroupSelector 构建无嵌套配置的简单供应商选择器（用于 fallback，防止递归）。
func buildSimpleGroupSelector(strategy config.GroupSelectorStrategy) (selector.GroupSelector, error) {
	switch strategy {
	case config.GroupSelectorPriority, "":
		return groupStrategies.NewGroupPriority(), nil
	case config.GroupSelectorRoundRobin:
		return groupStrategies.NewGroupRoundRobin(), nil
	case config.GroupSelectorWeighted:
		return groupStrategies.NewGroupWeighted(), nil
	case config.GroupSelectorMostAvailable:
		return groupStrategies.NewGroupMostAvailable(), nil
	default:
		return nil, fmt.Errorf("unknown group_selector strategy for fallback: %q", strategy)
	}
}

// ===================== Occupancy 构建 =====================

// buildOccupancy 根据 TypedConfig 构建占用控制器。
func buildOccupancy(tc *config.TypedConfig[config.OccupancyStrategy], deps *Dependencies) (occupancy.Controller, error) {
	if tc == nil || tc.IsZero() {
		return occupancy.NewUnlimited(), nil
	}

	switch tc.Strategy {
	case config.OccupancyUnlimited:
		return occupancy.NewUnlimited(), nil

	case config.OccupancyFixed:
		var cfg config.FixedLimitConfig
		if err := tc.Decode(&cfg); err != nil {
			return nil, fmt.Errorf("decode fixed config: %w", err)
		}
		if cfg.DefaultLimit <= 0 {
			return nil, fmt.Errorf("fixed occupancy: default_limit must be > 0")
		}
		var opts []occupancy.FixedLimitOption
		if deps.Storage != nil {
			opts = append(opts, occupancy.WithStore(deps.Storage))
		}
		return occupancy.NewFixedLimit(cfg.DefaultLimit, opts...), nil

	case config.OccupancyAdaptive:
		var cfg config.AdaptiveLimitConfig
		if err := tc.Decode(&cfg); err != nil {
			return nil, fmt.Errorf("decode adaptive config: %w", err)
		}
		if deps.UsageTracker == nil {
			return nil, fmt.Errorf("adaptive occupancy requires usage_tracker to be configured")
		}
		var opts []occupancy.AdaptiveLimitOption
		if deps.Storage != nil {
			opts = append(opts, occupancy.WithAdaptiveStore(deps.Storage))
		}
		if cfg.Factor > 0 {
			opts = append(opts, occupancy.WithFactor(cfg.Factor))
		}
		if cfg.MinLimit > 0 {
			opts = append(opts, occupancy.WithMinLimit(cfg.MinLimit))
		}
		if cfg.MaxLimit > 0 {
			opts = append(opts, occupancy.WithMaxLimit(cfg.MaxLimit))
		}
		if cfg.FallbackLimit > 0 {
			opts = append(opts, occupancy.WithFallbackLimit(cfg.FallbackLimit))
		}
		return occupancy.NewAdaptiveLimit(deps.UsageTracker, opts...), nil

	default:
		return nil, fmt.Errorf("unknown occupancy strategy: %q", tc.Strategy)
	}
}

// ===================== 单实现组件构建 =====================

// buildUsageTracker 构建用量追踪器。
func buildUsageTracker(cfg *config.UsageTrackerConfig, deps *Dependencies) usagetracker.UsageTracker {
	var opts []usagetracker.Option
	if deps.Storage != nil {
		opts = append(opts, usagetracker.WithUsageStore(deps.Storage))
	}
	if cfg.SafetyRatio > 0 {
		opts = append(opts, usagetracker.WithSafetyRatio(cfg.SafetyRatio))
	}
	return usagetracker.NewUsageTracker(opts...)
}

// buildCircuitBreaker 构建熔断器。
func buildCircuitBreaker(cfg *config.CircuitBreakerConfig, deps *Dependencies) (circuitbreaker.CircuitBreaker, error) {
	var opts []circuitbreaker.Option
	if deps.Storage != nil {
		opts = append(opts, circuitbreaker.WithStatsStore(deps.Storage))
	}
	if cfg.DefaultThreshold > 0 {
		opts = append(opts, circuitbreaker.WithDefaultThreshold(cfg.DefaultThreshold))
	}
	if cfg.DefaultTimeout > 0 {
		opts = append(opts, circuitbreaker.WithDefaultTimeout(cfg.DefaultTimeout))
	}
	if cfg.ThresholdRatio > 0 {
		opts = append(opts, circuitbreaker.WithThresholdRatio(cfg.ThresholdRatio))
	}
	if cfg.MinThreshold > 0 {
		opts = append(opts, circuitbreaker.WithMinThreshold(cfg.MinThreshold))
	}
	return circuitbreaker.NewCircuitBreaker(opts...)
}

// buildCooldown 构建冷却管理器。
func buildCooldown(cfg *config.CooldownConfig) cooldown.CooldownManager {
	var opts []cooldown.Option
	if cfg.DefaultDuration > 0 {
		opts = append(opts, cooldown.WithDefaultDuration(cfg.DefaultDuration))
	}
	return cooldown.NewCooldownManager(opts...)
}
