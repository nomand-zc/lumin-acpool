package usagetracker

import (
	"context"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-acpool/storage/memory/usagestore"
	"github.com/nomand-zc/lumin-client/usagerule"
)

// Compile-time interface compliance check.
var _ UsageTracker = (*defaultUsageTracker)(nil)

// defaultUsageTracker 是 UsageTracker 接口的默认实现。
// 通过 storage.UsageStore 接口管理追踪数据，支持内存和 Redis 等多种后端。
type defaultUsageTracker struct {
	opts  Options
	store storage.UsageStore
}

// NewUsageTracker 创建一个 UsageTracker 实例。
// 若未通过 WithUsageStore 注入存储后端，默认使用 MemoryUsageStore。
func NewUsageTracker(opts ...Option) UsageTracker {
	o := defaultOpts
	for _, opt := range opts {
		opt(&o)
	}
	store := o.Store
	if store == nil {
		store = usagestore.NewMemoryUsageStore()
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

	// 检测配额是否达到安全阈值，触发回调
	if len(t.opts.OnQuotaExhausted) > 0 {
		// 重新获取最新数据（IncrLocalUsed 后数据已变更）
		usages, err = t.store.GetAll(ctx, accountID)
		if err != nil {
			return nil // 检测失败不影响主流程
		}
		now := time.Now()
		for _, u := range usages {
			if u.Rule == nil || u.Rule.Total <= 0 || u.Rule.SourceType != sourceType {
				continue
			}
			// 窗口已过期则跳过
			if u.WindowEnd != nil && now.After(*u.WindowEnd) {
				continue
			}
			usedRatio := u.EstimatedUsed() / u.Rule.Total
			if usedRatio >= t.opts.SafetyRatio {
				for _, cb := range t.opts.OnQuotaExhausted {
					cb(ctx, accountID, u.Rule)
				}
				return nil // 触发一次即可，由回调方决定如何处理
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
		if usedRatio >= t.opts.SafetyRatio {
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
		// 根据远端 stats 初始化（首次校准，无并发写入，可安全使用 Save）
		newUsages := make([]*account.TrackedUsage, 0, len(stats))
		for _, s := range stats {
			if s == nil {
				continue
			}
			newUsages = append(newUsages, &account.TrackedUsage{
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

	// 校准已有规则：逐条原子更新，避免全量 Save 覆盖并发 IncrLocalUsed 的增量
	hasNewRules := false
	for _, s := range stats {
		if s == nil || s.Rule == nil {
			continue
		}
		matched := false
		for i, u := range usages {
			if u.Rule != nil && u.Rule.SourceType == s.Rule.SourceType &&
				u.Rule.TimeGranularity == s.Rule.TimeGranularity &&
				u.Rule.WindowSize == s.Rule.WindowSize {
				// 匹配到已有规则，使用原子校准（重置 local_used=0 并更新远端数据）
				calibrated := &account.TrackedUsage{
					RemoteUsed:   s.Used,
					RemoteRemain: s.Remain,
					WindowStart:  s.StartTime,
					WindowEnd:    s.EndTime,
				}
				if err := t.store.CalibrateRule(ctx, accountID, i, calibrated); err != nil {
					return err
				}
				// 同步更新内存中的 usages 以保持一致性
				u.RemoteUsed = s.Used
				u.RemoteRemain = s.Remain
				u.LocalUsed = 0
				u.WindowStart = s.StartTime
				u.WindowEnd = s.EndTime
				u.LastSyncAt = time.Now()
				matched = true
				break
			}
		}
		if !matched {
			// 新增规则，追加到 usages
			usages = append(usages, &account.TrackedUsage{
				Rule:         s.Rule,
				LocalUsed:    0,
				RemoteUsed:   s.Used,
				RemoteRemain: s.Remain,
				WindowStart:  s.StartTime,
				WindowEnd:    s.EndTime,
				LastSyncAt:   time.Now(),
			})
			hasNewRules = true
		}
	}

	// 如果有新增规则，需要全量 Save（因为需要插入新行）
	if hasNewRules {
		return t.store.Save(ctx, accountID, usages)
	}

	return nil
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

func (t *defaultUsageTracker) GetTrackedUsages(ctx context.Context, accountID string) ([]*account.TrackedUsage, error) {
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
	usages := make([]*account.TrackedUsage, 0, len(rules))
	for _, rule := range rules {
		if rule == nil || !rule.IsValid() {
			continue
		}
		start, end := rule.CalculateWindowTime()
		usages = append(usages, &account.TrackedUsage{
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
