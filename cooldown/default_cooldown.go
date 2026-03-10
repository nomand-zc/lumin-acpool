package cooldown

import (
	"math"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
)

// defaultCooldownManager 是 CooldownManager 接口的默认实现。
// 支持基于账号 UsageRules 动态计算冷却时长。
type defaultCooldownManager struct {
	opts Options
}

// NewCooldownManager 创建一个 CooldownManager 实例。
func NewCooldownManager(opts ...Option) CooldownManager {
	o := defaultOptions
	for _, opt := range opts {
		opt(&o)
	}
	return &defaultCooldownManager{
		opts: o,
	}
}

// StartCooldown 将账号设置为冷却状态。
// until 为冷却到期时间；nil 表示根据 UsageRules 动态计算，fallback 到默认冷却时长。
func (cm *defaultCooldownManager) StartCooldown(acct *account.Account, until *time.Time) {
	if until != nil {
		acct.CooldownUntil = until
		return
	}

	// 根据 UsageRules 估算冷却时长
	if d := cm.estimateCooldownDuration(acct); d > 0 {
		t := time.Now().Add(d)
		acct.CooldownUntil = &t
		return
	}

	// fallback 到默认值
	t := time.Now().Add(cm.opts.DefaultDuration)
	acct.CooldownUntil = &t
}

// IsCooldownExpired 返回冷却期是否已过期。
func (cm *defaultCooldownManager) IsCooldownExpired(acct *account.Account) bool {
	return acct.IsCooldownExpired()
}

// estimateCooldownDuration 根据 UsageRules 估算合理的冷却时长。
// 取最小窗口的剩余时间，即最快可能恢复的时间点。
func (cm *defaultCooldownManager) estimateCooldownDuration(acct *account.Account) time.Duration {
	if len(acct.UsageRules) == 0 {
		return 0
	}

	now := time.Now()
	minRemain := time.Duration(math.MaxInt64)

	for _, rule := range acct.UsageRules {
		if rule == nil || !rule.IsValid() {
			continue
		}
		_, end := rule.CalculateWindowTime()
		if end != nil && end.After(now) {
			remain := end.Sub(now)
			if remain < minRemain {
				minRemain = remain
			}
		}
	}

	if minRemain == time.Duration(math.MaxInt64) {
		return 0
	}
	return minRemain
}
