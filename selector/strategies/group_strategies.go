package group

import (
	rand "math/rand/v2"
	"sort"
	"sync/atomic"

	"github.com/nomand-zc/lumin-acpool/provider"
	"github.com/nomand-zc/lumin-acpool/selector"
)

// GroupPriority 供应商级优先级选择策略
// 选择优先级最高的供应商；优先级相同时选可用账号数最多的
type GroupPriority struct{}

// NewGroupPriority 创建供应商级优先级策略实例
func NewGroupPriority() *GroupPriority {
	return &GroupPriority{}
}

// Name 返回策略名称
func (g *GroupPriority) Name() string {
	return "group_priority"
}

// Select 选择优先级最高的供应商
func (g *GroupPriority) Select(candidates []*provider.ProviderInfo, _ *selector.SelectRequest) (*provider.ProviderInfo, error) {
	if len(candidates) == 0 {
		return nil, selector.ErrEmptyCandidates
	}

	sorted := make([]*provider.ProviderInfo, len(candidates))
	copy(sorted, candidates)
	sort.SliceStable(sorted, func(i, j int) bool {
		// 先按优先级降序
		if sorted[i].Priority != sorted[j].Priority {
			return sorted[i].Priority > sorted[j].Priority
		}
		// 优先级相同，按可用账号数降序
		return sorted[i].AvailableAccountCount > sorted[j].AvailableAccountCount
	})

	return sorted[0], nil
}

// GroupMostAvailable 供应商级最多可用账号选择策略
// 选择可用账号数最多的供应商，适用于希望负载均衡的场景
type GroupMostAvailable struct{}

// NewGroupMostAvailable 创建供应商级最多可用账号策略实例
func NewGroupMostAvailable() *GroupMostAvailable {
	return &GroupMostAvailable{}
}

// Name 返回策略名称
func (g *GroupMostAvailable) Name() string {
	return "group_most_available"
}

// Select 选择可用账号数最多的供应商
func (g *GroupMostAvailable) Select(candidates []*provider.ProviderInfo, _ *selector.SelectRequest) (*provider.ProviderInfo, error) {
	if len(candidates) == 0 {
		return nil, selector.ErrEmptyCandidates
	}

	best := candidates[0]
	for _, p := range candidates[1:] {
		if p.AvailableAccountCount > best.AvailableAccountCount {
			best = p
		} else if p.AvailableAccountCount == best.AvailableAccountCount && p.Priority > best.Priority {
			best = p
		}
	}

	return best, nil
}

// GroupWeighted 供应商级加权随机选择策略
// 按供应商权重随机选择
type GroupWeighted struct{}

// NewGroupWeighted 创建供应商级加权随机策略实例
func NewGroupWeighted() *GroupWeighted {
	return &GroupWeighted{}
}

// Name 返回策略名称
func (g *GroupWeighted) Name() string {
	return "group_weighted"
}

// Select 按权重随机选择一个供应商
func (g *GroupWeighted) Select(candidates []*provider.ProviderInfo, _ *selector.SelectRequest) (*provider.ProviderInfo, error) {
	if len(candidates) == 0 {
		return nil, selector.ErrEmptyCandidates
	}

	if len(candidates) == 1 {
		return candidates[0], nil
	}

	totalWeight := 0
	for _, p := range candidates {
		w := p.Weight
		if w <= 0 {
			w = 1
		}
		totalWeight += w
	}

	r := rand.IntN(totalWeight)
	cumulative := 0
	for _, p := range candidates {
		w := p.Weight
		if w <= 0 {
			w = 1
		}
		cumulative += w
		if r < cumulative {
			return p, nil
		}
	}

	return candidates[len(candidates)-1], nil
}

// GroupRoundRobin 供应商级轮询选择策略
type GroupRoundRobin struct {
	counter uint64
}

// NewGroupRoundRobin 创建供应商级轮询策略实例
func NewGroupRoundRobin() *GroupRoundRobin {
	return &GroupRoundRobin{}
}

// Name 返回策略名称
func (g *GroupRoundRobin) Name() string {
	return "group_round_robin"
}

// Select 轮询选择一个供应商
func (g *GroupRoundRobin) Select(candidates []*provider.ProviderInfo, _ *selector.SelectRequest) (*provider.ProviderInfo, error) {
	if len(candidates) == 0 {
		return nil, selector.ErrEmptyCandidates
	}

	idx := atomic.AddUint64(&g.counter, 1) - 1
	return candidates[idx%uint64(len(candidates))], nil
}
