package health

import (
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
)

// CooldownManager 冷却管理接口
// 管理因限流等原因需要冷却的账号
type CooldownManager interface {
	// StartCooldown 将账号设置为冷却状态
	// until 为冷却截止时间，nil 时使用默认冷却时长
	StartCooldown(acct *account.Account, until *time.Time)

	// IsCooldownExpired 判断冷却是否已到期
	IsCooldownExpired(acct *account.Account) bool
}

// CooldownConfig 冷却管理器配置
type CooldownConfig struct {
	// DefaultDuration 默认冷却时长，当限流响应未提供冷却截止时间时使用（默认 30s）
	DefaultDuration time.Duration
}

// DefaultCooldownConfig 返回默认的冷却管理器配置
func DefaultCooldownConfig() CooldownConfig {
	return CooldownConfig{
		DefaultDuration: 30 * time.Second,
	}
}
