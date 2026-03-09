package health

import (
	"context"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/provider"
	"github.com/nomand-zc/lumin-client/credentials"
)

// CheckSeverity 检查项严重程度
type CheckSeverity int

const (
	// SeverityInfo 信息级别，不影响账号状态（如统计更新）
	SeverityInfo CheckSeverity = iota + 1
	// SeverityWarning 警告级别，可能需要关注但不立即影响可用性
	SeverityWarning
	// SeverityCritical 关键级别，直接影响账号是否可用
	SeverityCritical
)

// String 返回严重程度的可读字符串
func (s CheckSeverity) String() string {
	switch s {
	case SeverityInfo:
		return "info"
	case SeverityWarning:
		return "warning"
	case SeverityCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// CheckStatus 单项检查的结果状态
type CheckStatus int

const (
	// CheckPassed 检查通过
	CheckPassed CheckStatus = iota + 1
	// CheckWarning 检查通过但有告警
	CheckWarning
	// CheckFailed 检查未通过
	CheckFailed
	// CheckSkipped 检查被跳过（如依赖的前置检查失败）
	CheckSkipped
	// CheckError 检查过程本身出错（如网络超时）
	CheckError
)

// String 返回检查状态的可读字符串
func (s CheckStatus) String() string {
	switch s {
	case CheckPassed:
		return "passed"
	case CheckWarning:
		return "warning"
	case CheckFailed:
		return "failed"
	case CheckSkipped:
		return "skipped"
	case CheckError:
		return "error"
	default:
		return "unknown"
	}
}

// CheckResult 单个检查项的执行结果
type CheckResult struct {
	// CheckName 检查项名称
	CheckName string
	// Status 结果状态
	Status CheckStatus
	// Severity 严重程度
	Severity CheckSeverity
	// Message 结果描述信息
	Message string
	// SuggestedStatus 建议将账号切换到的状态（可选）
	// 为 nil 时表示此检查项不建议变更账号状态
	SuggestedStatus *account.Status
	// Data 检查项产出的附加数据
	// 例如 UsageQuotaCheck 可将最新的 UsageStats 放在这里，供上层回写
	Data any
	// Duration 此检查项的执行耗时
	Duration time.Duration
	// Timestamp 检查完成时间
	Timestamp time.Time
}

// HealthReport 一次完整健康巡检的汇总报告
type HealthReport struct {
	// AccountID 被检查的账号 ID
	AccountID string
	// ProviderKey 账号所属供应商
	ProviderKey provider.ProviderKey
	// Results 各检查项的结果（按执行顺序排列）
	Results []*CheckResult
	// TotalDuration 完整巡检的总耗时
	TotalDuration time.Duration
	// Timestamp 巡检完成时间
	Timestamp time.Time
}

// HasCriticalFailure 报告中是否有关键级别的检查失败
func (r *HealthReport) HasCriticalFailure() bool {
	for _, result := range r.Results {
		if result.Status == CheckFailed && result.Severity == SeverityCritical {
			return true
		}
	}
	return false
}

// FailedChecks 获取所有失败的检查项
func (r *HealthReport) FailedChecks() []*CheckResult {
	var failed []*CheckResult
	for _, result := range r.Results {
		if result.Status == CheckFailed {
			failed = append(failed, result)
		}
	}
	return failed
}

// WarningChecks 获取所有告警的检查项
func (r *HealthReport) WarningChecks() []*CheckResult {
	var warnings []*CheckResult
	for _, result := range r.Results {
		if result.Status == CheckWarning {
			warnings = append(warnings, result)
		}
	}
	return warnings
}

// PassedChecks 获取所有通过的检查项
func (r *HealthReport) PassedChecks() []*CheckResult {
	var passed []*CheckResult
	for _, result := range r.Results {
		if result.Status == CheckPassed {
			passed = append(passed, result)
		}
	}
	return passed
}

// CheckTarget 检查目标，封装被检查对象的所有信息
// 使用接口而非具体类型，保持 HealthCheck 接口的独立性
type CheckTarget interface {
	// Credential 账号凭证
	Credential() credentials.Credential
	// ProviderInstance 获取供应商运行时实例（包含元数据和底层 SDK 实例）
	ProviderInstance() *provider.ProviderInstance
	// Account 获取完整的账号对象（供需要访问统计信息等额外字段的检查项使用）
	Account() *account.Account
}

// HealthCheck 健康检查项通用接口
// 这是整个健康检查体系的核心契约。
// 内置检查项和用户自定义检查项都通过实现此接口来扩展。
type HealthCheck interface {
	// Name 检查项的唯一标识名称
	Name() string

	// Severity 该检查项的严重程度
	Severity() CheckSeverity

	// Check 执行检查
	// ctx: 上下文（含超时控制）
	// target: 被检查的对象，携带账号信息和底层 Provider 实例
	//
	// 约定：即使检查过程出错，也应返回 CheckResult（Status=CheckError），而非依赖 error。
	// 这确保 HealthReport 始终能收集到所有检查项的结果。
	Check(ctx context.Context, target CheckTarget) *CheckResult

	// DependsOn 返回此检查项依赖的前置检查项名称
	// 如果依赖的检查项 Status 为 Failed，当前检查项将被标记为 Skipped。
	// 返回 nil 表示无依赖。
	DependsOn() []string
}
