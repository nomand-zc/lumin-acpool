package circuitbreaker

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-client/usagerule"
)

// defaultCircuitBreaker 是 CircuitBreaker 接口的默认实现。
// 基于连续失败次数判断是否触发熔断，支持基于账号 UsageRules 动态计算阈值和超时时间。
type defaultCircuitBreaker struct {
	opts options
}

// NewCircuitBreaker 创建一个 CircuitBreaker 实例。
func NewCircuitBreaker(opts ...Option) (CircuitBreaker, error) {
	o := defaultOptions
	for _, opt := range opts {
		opt(&o)
	}
	if o.statsStore == nil {
		return nil, fmt.Errorf("circuitbreaker: StatsStore is required")
	}
	return &defaultCircuitBreaker{opts: o}, nil
}

// RecordSuccess 记录一次成功调用，重置连续失败计数。
func (cb *defaultCircuitBreaker) RecordSuccess(ctx context.Context, acct *account.Account) error {
	if err := cb.opts.statsStore.ResetConsecutiveFailures(ctx, acct.ID); err != nil {
		return fmt.Errorf("circuitbreaker: reset consecutive failures: %w", err)
	}
	acct.CircuitOpenUntil = nil
	return nil
}

// RecordFailure 记录一次失败调用。
// 返回是否触发熔断（true 表示账号应切换为 CircuitOpen 状态）。
func (cb *defaultCircuitBreaker) RecordFailure(ctx context.Context, acct *account.Account) (tripped bool, err error) {
	failures, err := cb.opts.statsStore.GetConsecutiveFailures(ctx, acct.ID)
	if err != nil {
		return false, fmt.Errorf("circuitbreaker: get consecutive failures: %w", err)
	}

	threshold := cb.dynamicThreshold(acct)
	if failures >= threshold {
		// 达到阈值，触发熔断，设置熔断恢复时间
		timeout := cb.dynamicTimeout(acct)
		until := time.Now().Add(timeout)
		acct.CircuitOpenUntil = &until
		return true, nil
	}
	return false, nil
}

// ShouldAllow 检查熔断状态的账号是否可以尝试半开探测，
// 即熔断超时时间窗口是否已过。
func (cb *defaultCircuitBreaker) ShouldAllow(acct *account.Account) bool {
	return acct.IsCircuitOpenExpired()
}

// dynamicThreshold 根据账号的 UsageRules 动态计算熔断阈值。
// 取请求类型规则中最小 Total 的比例，至少为 minThreshold。
func (cb *defaultCircuitBreaker) dynamicThreshold(acct *account.Account) int {
	if len(acct.UsageRules) == 0 {
		return cb.opts.defaultThreshold
	}

	minTotal := math.MaxFloat64
	for _, rule := range acct.UsageRules {
		if rule != nil && rule.SourceType == usagerule.SourceTypeRequest && rule.Total > 0 && rule.Total < minTotal {
			minTotal = rule.Total
		}
	}

	if minTotal == math.MaxFloat64 {
		return cb.opts.defaultThreshold
	}

	threshold := int(minTotal * cb.opts.thresholdRatio)
	if threshold < cb.opts.minThreshold {
		threshold = cb.opts.minThreshold
	}
	return threshold
}

// dynamicTimeout 根据账号的 UsageRules 动态计算熔断超时时间。
// 取最小规则窗口的剩余时间，至少为 defaultTimeout。
func (cb *defaultCircuitBreaker) dynamicTimeout(acct *account.Account) time.Duration {
	if len(acct.UsageRules) == 0 {
		return cb.opts.defaultTimeout
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

	if minRemain == time.Duration(math.MaxInt64) || minRemain < cb.opts.defaultTimeout {
		return cb.opts.defaultTimeout
	}
	return minRemain
}
