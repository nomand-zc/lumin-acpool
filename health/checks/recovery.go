package checks

import (
	"context"
	"fmt"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/health"
)

const RecoveryCheckName = "recovery"

// RecoveryCheck 冷却/熔断到期自动恢复检查。
// 检测处于 CoolingDown 或 CircuitOpen 状态的账号是否已到期，
// 如果到期则建议恢复为 Available 状态。
//
// 该检查无依赖、无网络开销，仅根据账号本地时间字段做判断。
// 应以较短的检查间隔注册（如 5~10s），确保账号能及时恢复。
type RecoveryCheck struct{}

// NewRecoveryCheck 创建一个冷却/熔断恢复检查实例。
func NewRecoveryCheck() *RecoveryCheck {
	return &RecoveryCheck{}
}

func (c *RecoveryCheck) Name() string {
	return RecoveryCheckName
}

func (c *RecoveryCheck) Severity() health.CheckSeverity {
	return health.SeverityCritical
}

func (c *RecoveryCheck) DependsOn() []string {
	return nil // 无依赖，优先执行
}

func (c *RecoveryCheck) Check(_ context.Context, target health.CheckTarget) *health.CheckResult {
	start := time.Now()
	acct := target.Account()

	switch acct.Status {
	case account.StatusCoolingDown:
		if acct.IsCooldownExpired() {
			status := account.StatusAvailable
			return &health.CheckResult{
				CheckName:       RecoveryCheckName,
				Status:          health.CheckPassed,
				Severity:        health.SeverityCritical,
				Message:         fmt.Sprintf("cooldown expired (until: %v), recovering to available", acct.CooldownUntil),
				SuggestedStatus: &status,
				Duration:        time.Since(start),
				Timestamp:       time.Now(),
			}
		}
		return &health.CheckResult{
			CheckName: RecoveryCheckName,
			Status:    health.CheckWarning,
			Severity:  health.SeverityCritical,
			Message:   fmt.Sprintf("cooldown still active (until: %v)", acct.CooldownUntil),
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		}

	case account.StatusCircuitOpen:
		if acct.IsCircuitOpenExpired() {
			status := account.StatusAvailable
			return &health.CheckResult{
				CheckName:       RecoveryCheckName,
				Status:          health.CheckPassed,
				Severity:        health.SeverityCritical,
				Message:         fmt.Sprintf("circuit breaker timeout expired (until: %v), recovering to available", acct.CircuitOpenUntil),
				SuggestedStatus: &status,
				Duration:        time.Since(start),
				Timestamp:       time.Now(),
			}
		}
		return &health.CheckResult{
			CheckName: RecoveryCheckName,
			Status:    health.CheckWarning,
			Severity:  health.SeverityCritical,
			Message:   fmt.Sprintf("circuit breaker still open (until: %v)", acct.CircuitOpenUntil),
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		}

	default:
		// 非冷却/熔断状态的账号，直接跳过
		return &health.CheckResult{
			CheckName: RecoveryCheckName,
			Status:    health.CheckPassed,
			Severity:  health.SeverityCritical,
			Message:   fmt.Sprintf("account status is %s, no recovery needed", acct.Status),
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		}
	}
}
