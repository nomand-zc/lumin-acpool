package group

import (
	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/selector"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-acpool/storage/memory/affinitystore"
)

// GroupAffinity 是供应商级别的亲和选择策略。
// 尽可能将同一用户（UserID）对同一模型的多次请求路由到同一个供应商上，
// 以充分利用 LLM 的 system prompt caching 能力。
//
// 工作原理：
//  1. 使用 UserID + Model 作为亲和键，通过 AffinityStore 维护 用户→ProviderKey 的映射关系
//  2. 如果映射中存在记录，且该供应商仍在候选列表中，则直接返回该供应商
//  3. 如果映射中无记录，或该供应商已不可用，则退化为 fallback 策略选择，
//     并将新选中的供应商记录到映射中
//
// 注意事项：
//   - UserID 为空时，直接退化为 fallback 策略
//   - 默认使用内存存储（MemoryAffinityStore），进程重启后会丢失（这是可接受的）
//   - 集群部署时，可通过 GroupAffinityWithStore 注入基于 Redis 等共享存储的实现，
//     使多个实例共享绑定关系，充分发挥亲和策略的效果
type GroupAffinity struct {
	store    storage.AffinityStore
	fallback selector.GroupSelector
}

// GroupAffinityOption 是 GroupAffinity 策略的配置选项。
type GroupAffinityOption func(*GroupAffinity)

// GroupAffinityWithFallback 设置亲和未命中时的退化策略（默认：GroupPriority）。
func GroupAffinityWithFallback(s selector.GroupSelector) GroupAffinityOption {
	return func(a *GroupAffinity) {
		if s != nil {
			a.fallback = s
		}
	}
}

// GroupAffinityWithStore 设置亲和绑定关系的存储实现。
// 默认使用 MemoryAffinityStore（内存存储）；
// 集群部署时，建议注入基于 Redis/数据库等共享存储的实现。
func GroupAffinityWithStore(store storage.AffinityStore) GroupAffinityOption {
	return func(a *GroupAffinity) {
		if store != nil {
			a.store = store
		}
	}
}

// NewGroupAffinity 创建供应商亲和策略实例。
//
// 参数:
//   - opts: 可选配置项，支持 GroupAffinityWithFallback 和 GroupAffinityWithStore
//
// 示例:
//
//	// 使用默认配置（fallback=GroupPriority, store=MemoryAffinityStore）
//	s := NewGroupAffinity()
//
//	// 使用加权随机作为退化策略
//	s := NewGroupAffinity(GroupAffinityWithFallback(NewGroupWeighted()))
//
//	// 集群部署：注入 Redis 存储
//	s := NewGroupAffinity(GroupAffinityWithStore(redisAffinityStore))
func NewGroupAffinity(opts ...GroupAffinityOption) *GroupAffinity {
	a := &GroupAffinity{
		store:    affinitystore.NewStore(),
		fallback: NewGroupPriority(),
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Name 返回策略名称。
func (a *GroupAffinity) Name() string {
	return "group_affinity"
}

// Select 基于用户亲和性选择供应商。
//
// 如果 UserID 为空，直接退化为 fallback 策略。
// 如果 UserID 非空，先查找映射：
//   - 命中且供应商在候选列表中 → 返回该供应商（亲和命中）
//   - 未命中或供应商不在候选列表中 → 使用 fallback 策略选择，并更新映射
func (a *GroupAffinity) Select(candidates []*account.ProviderInfo, req *selector.SelectRequest) (*account.ProviderInfo, error) {
	if len(candidates) == 0 {
		return nil, selector.ErrEmptyCandidates
	}

	// UserID 为空时退化为 fallback
	if req == nil || req.UserID == "" {
		return a.fallback.Select(candidates, req)
	}

	affinityKey := buildGroupAffinityKey(req.UserID, req.Model)

	// 查找亲和记录
	boundKeyStr, exists := a.store.GetAffinity(affinityKey)

	if exists {
		// 在候选列表中查找绑定的供应商
		for _, p := range candidates {
			if p.ProviderKey().String() == boundKeyStr {
				return p, nil
			}
		}
		// 绑定的供应商不在候选列表中（可能被禁用/降级），需要重新选择
	}

	// 使用 fallback 策略选择
	chosen, err := a.fallback.Select(candidates, req)
	if err != nil {
		return nil, err
	}

	// 更新映射
	a.store.SetAffinity(affinityKey, chosen.ProviderKey().String())

	return chosen, nil
}

// buildGroupAffinityKey 构建供应商亲和键。
// 使用 UserID + Model 的组合，使得不同模型的请求不会互相干扰。
func buildGroupAffinityKey(userID, model string) string {
	return userID + ":" + model
}
