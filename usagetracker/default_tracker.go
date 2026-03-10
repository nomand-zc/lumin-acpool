package usagetracker

import (
	"context"
	"time"

	"github.com/nomand-zc/lumin-client/usagerule"
)

// Compile-time interface compliance check.
var _ UsageTracker = (*defaultUsageTracker)(nil)

// defaultUsageTracker 是 UsageTracker 接口的默认实现。
// 通过 UsageStore 接口管理追踪数据，支持内存和 Redis 等多种后端。
type defaultUsageTracker struct {
	opts  options
	store UsageStore
}

// NewUsageTracker 创建一个 UsageTracker 实例。
// 若未通过 WithUsageStore 注入存储后端，默认使用 MemoryUsageStore。
func NewUsageTracker(opts ...Option) UsageTracker {
	o := defaultOpts
	for _, opt := range opts {
		opt(&o)
	}
	store := o.store
	if store == nil {
		store = NewMemoryUsageStore()
	}
	return &defaultUsageTracker{
		opts:  o,
		store: store,
	}
}

func (t *defaultUsageTracker) RecordUsage(ctx context.Context, accountID string, sourceType usagerule.SourceType, amount float64) error {
	usages, err := t.store.GetAll(ctx, accountID)
	if err != nil {
		return err
	}
	if len(usages) == 0 {
		return nil // 未初始化规则，静默忽略
	}

	for i, u := range usages {
		if u.Rule != nil && u.Rule.SourceType == sourceType {
			if err := t.store.IncrLocalUsed(ctx, accountID, i, amount); err != nil {
				return err
			}
		}
	}
	return nil
}

func (t *defaultUsageTracker) IsQuotaAvailable(ctx context.Context, accountID string) (bool, error) {
	usages, err := t.store.GetAll(ctx, accountID)
	if err != nil {
		return false, err
	}
	if len(usages) == 0 {
		return true, nil // 未初始化规则，默认可用
	}

	now := time.Now()
	for _, u := range usages {
		if u.Rule == nil || u.Rule.Total <= 0 {
			continue
		}
		// 检查是否在窗口内
		if u.WindowEnd != nil && now.After(*u.WindowEnd) {
			continue // 窗口已过期，跳过
		}
		usedRatio := u.EstimatedUsed() / u.Rule.Total
		if usedRatio >= t.opts.safetyRatio {
			return false, nil
		}
	}
	return true, nil
}

func (t *defaultUsageTracker) Calibrate(ctx context.Context, accountID string, stats []*usagerule.UsageStats) error {
	usages, err := t.store.GetAll(ctx, accountID)
	if err != nil {
		return err
	}

	if len(usages) == 0 {
		// 根据远端 stats 初始化
		newUsages := make([]*TrackedUsage, 0, len(stats))
		for _, s := range stats {
			if s == nil {
				continue
			}
			newUsages = append(newUsages, &TrackedUsage{
				Rule:         s.Rule,
				LocalUsed:    0,
				RemoteUsed:   s.Used,
				RemoteRemain: s.Remain,
				WindowStart:  s.StartTime,
				WindowEnd:    s.EndTime,
				LastSyncAt:   time.Now(),
			})
		}
		return t.store.Save(ctx, accountID, newUsages)
	}

	// 校准已有规则
	for _, s := range stats {
		if s == nil || s.Rule == nil {
			continue
		}
		matched := false
		for _, u := range usages {
			if u.Rule != nil && u.Rule.SourceType == s.Rule.SourceType &&
				u.Rule.TimeGranularity == s.Rule.TimeGranularity &&
				u.Rule.WindowSize == s.Rule.WindowSize {
				// 匹配到，校准数据
				u.RemoteUsed = s.Used
				u.RemoteRemain = s.Remain
				u.LocalUsed = 0 // 重置本地计数
				u.WindowStart = s.StartTime
				u.WindowEnd = s.EndTime
				u.LastSyncAt = time.Now()
				matched = true
				break
			}
		}
		if !matched {
			// 新增规则
			usages = append(usages, &TrackedUsage{
				Rule:         s.Rule,
				LocalUsed:    0,
				RemoteUsed:   s.Used,
				RemoteRemain: s.Remain,
				WindowStart:  s.StartTime,
				WindowEnd:    s.EndTime,
				LastSyncAt:   time.Now(),
			})
		}
	}
	return t.store.Save(ctx, accountID, usages)
}

func (t *defaultUsageTracker) CalibrateFromResponse(ctx context.Context, accountID string, sourceType usagerule.SourceType) error {
	usages, err := t.store.GetAll(ctx, accountID)
	if err != nil {
		return err
	}
	if len(usages) == 0 {
		return nil
	}

	for _, u := range usages {
		if u.Rule != nil && u.Rule.SourceType == sourceType {
			// 标记为已耗尽：将 LocalUsed 调整到使 EstimatedRemain() <= 0
			u.LocalUsed = u.RemoteRemain
		}
	}
	return t.store.Save(ctx, accountID, usages)
}

func (t *defaultUsageTracker) GetTrackedUsages(ctx context.Context, accountID string) ([]*TrackedUsage, error) {
	return t.store.GetAll(ctx, accountID)
}

func (t *defaultUsageTracker) MinRemainRatio(ctx context.Context, accountID string) (float64, error) {
	usages, err := t.store.GetAll(ctx, accountID)
	if err != nil {
		return 0, err
	}
	if len(usages) == 0 {
		return 1.0, nil
	}

	minRatio := 1.0
	for _, u := range usages {
		r := u.RemainRatio()
		if r < minRatio {
			minRatio = r
		}
	}
	return minRatio, nil
}

func (t *defaultUsageTracker) InitRules(ctx context.Context, accountID string, rules []*usagerule.UsageRule) error {
	usages := make([]*TrackedUsage, 0, len(rules))
	for _, rule := range rules {
		if rule == nil || !rule.IsValid() {
			continue
		}
		start, end := rule.CalculateWindowTime()
		usages = append(usages, &TrackedUsage{
			Rule:         rule,
			LocalUsed:    0,
			RemoteUsed:   0,
			RemoteRemain: rule.Total,
			WindowStart:  start,
			WindowEnd:    end,
			LastSyncAt:   time.Now(),
		})
	}
	return t.store.Save(ctx, accountID, usages)
}

func (t *defaultUsageTracker) Remove(ctx context.Context, accountID string) error {
	return t.store.Remove(ctx, accountID)
}
