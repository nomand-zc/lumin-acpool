package health

import (
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
)

// CircuitBreaker 熔断器接口
// 根据连续失败次数决定是否触发熔断
type CircuitBreaker interface {
	// RecordSuccess 记录一次成功调用，重置连续失败计数
	RecordSuccess(acct *account.Account)

	// RecordFailure 记录一次失败调用
	// 返回是否触发熔断（true 表示需要将账号切换到 CircuitOpen 状态）
	RecordFailure(acct *account.Account) (tripped bool)

	// ShouldAllow 判断熔断中的账号是否可以尝试半开探测
	// 即熔断时间窗口是否已过
	ShouldAllow(acct *account.Account) bool
}

// CircuitBreakerConfig 熔断器配置
type CircuitBreakerConfig struct {
	// Threshold 触发熔断的连续失败次数阈值（默认 5）
	Threshold int
	// Timeout 熔断恢复时间窗口（默认 60s）
	// 熔断触发后，等待 Timeout 后进入半开状态，允许一次探测请求
	Timeout time.Duration
}

// DefaultCircuitBreakerConfig 返回默认的熔断器配置
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		Threshold: 5,
		Timeout:   60 * time.Second,
	}
}
