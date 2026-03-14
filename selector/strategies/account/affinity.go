package account

import (
	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/selector"
	"github.com/nomand-zc/lumin-acpool/storage"
	storeMemory "github.com/nomand-zc/lumin-acpool/storage/memory"
)

// Affinity 是账号级别的亲和选择策略。
// 尽可能将同一用户（UserID）对同一模型的多次请求路由到同一个账号上，
// 以充分利用 LLM 的 system prompt caching 能力。
//
// 工作原理：
//  1. 使用 UserID + Model 作为亲和键，通过 AffinityStore 维护 用户→AccountID 的映射关系
//  2. 如果映射中存在记录，且该账号仍在候选列表中，则直接返回该账号
//  3. 如果映射中无记录，或该账号已不可用，则退化为 fallback 策略选号，
//     并将新选中的账号记录到映射中
//
// 注意事项：
//   - UserID 为空时，直接退化为 fallback 策略
//   - 默认使用内存存储（MemoryAffinityStore），进程重启后会丢失（这是可接受的）
//   - 集群部署时，可通过 AffinityWithStore 注入基于 Redis 等共享存储的实现，
//     使多个实例共享绑定关系，充分发挥亲和策略的效果
type Affinity struct {
	store    storage.AffinityStore
	fallback selector.Selector
}

// AffinityOption 是 Affinity 策略的配置选项。
type AffinityOption func(*Affinity)

// AffinityWithFallback 设置亲和未命中时的退化策略（默认：RoundRobin）。
func AffinityWithFallback(s selector.Selector) AffinityOption {
	return func(a *Affinity) {
		if s != nil {
			a.fallback = s
		}
	}
}

// AffinityWithStore 设置亲和绑定关系的存储实现。
// 默认使用 MemoryAffinityStore（内存存储）；
// 集群部署时，建议注入基于 Redis/数据库等共享存储的实现。
func AffinityWithStore(store storage.AffinityStore) AffinityOption {
	return func(a *Affinity) {
		if store != nil {
			a.store = store
		}
	}
}

// NewAffinity 创建账号亲和策略实例。
//
// 参数:
//   - opts: 可选配置项，支持 AffinityWithFallback 和 AffinityWithStore
//
// 示例:
//
//	// 使用默认配置（fallback=RoundRobin, store=MemoryAffinityStore）
//	s := NewAffinity()
//
//	// 使用最少使用策略作为退化策略
//	s := NewAffinity(AffinityWithFallback(account.NewLeastUsed()))
//
//	// 集群部署：注入 Redis 存储
//	s := NewAffinity(AffinityWithStore(redisAffinityStore))
func NewAffinity(opts ...AffinityOption) *Affinity {
	a := &Affinity{
	store:    storeMemory.NewStore(),
		fallback: NewRoundRobin(),
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Name 返回策略名称。
func (a *Affinity) Name() string {
	return "affinity"
}

// Select 基于用户亲和性选择账号。
//
// 如果 UserID 为空，直接退化为 fallback 策略。
// 如果 UserID 非空，先查找映射：
//   - 命中且账号在候选列表中 → 返回该账号（亲和命中）
//   - 未命中或账号不在候选列表中 → 使用 fallback 策略选择，并更新映射
func (a *Affinity) Select(candidates []*account.Account, req *selector.SelectRequest) (*account.Account, error) {
	if len(candidates) == 0 {
		return nil, selector.ErrEmptyCandidates
	}

	// UserID 为空时退化为 fallback
	if req == nil || req.UserID == "" {
		return a.fallback.Select(candidates, req)
	}

	affinityKey := buildAffinityKey(req.UserID, req.Model)

	// 查找亲和记录
	boundID, exists := a.store.GetAffinity(affinityKey)

	if exists {
		// 在候选列表中查找绑定的账号
		for _, acct := range candidates {
			if acct.ID == boundID {
				return acct, nil
			}
		}
		// 绑定的账号不在候选列表中（可能被禁用/冷却/熔断），需要重新选择
	}

	// 使用 fallback 策略选择
	chosen, err := a.fallback.Select(candidates, req)
	if err != nil {
		return nil, err
	}

	// 更新映射
	a.store.SetAffinity(affinityKey, chosen.ID)

	return chosen, nil
}

// buildAffinityKey 构建账号亲和键。
// 使用 UserID + Model 的组合，使得不同模型的请求不会互相干扰。
func buildAffinityKey(userID, model string) string {
	return userID + ":" + model
}
