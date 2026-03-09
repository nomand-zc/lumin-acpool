package scheduler

import (
	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/provider"
)

// ScheduleRequest 调度请求
type ScheduleRequest struct {
	// Model 请求的模型名称（必填）
	Model string

	// ProviderKey 供应商定位（可选，指针类型）
	//   - nil: 全自动选择
	//   - 仅填 Type: 限定供应商类型
	//   - Type + Name 都填: 精确指定供应商
	ProviderKey *provider.ProviderKey

	// Tags 标签过滤（可选）
	Tags map[string]string

	// MaxRetries 本次请求的最大重试次数（覆盖全局配置，0 = 不重试）
	MaxRetries int

	// EnableFailover 是否启用故障转移
	// 当一个供应商下无可用账号时，自动尝试下一个候选供应商
	EnableFailover bool
}

// ScheduleResult 调度结果
type ScheduleResult struct {
	// Account 被选中的账号（深拷贝）
	Account *account.Account

	// ProviderKey 被选中的供应商标识
	ProviderKey provider.ProviderKey

	// Attempts 总尝试次数（含重试）
	Attempts int
}
