package scheduler

import "context"

// Scheduler 调度器接口
// 编排完整的"筛选供应商 → 选择供应商 → 筛选账号 → 选择账号"流程
// 并管理调用结果上报（驱动熔断/冷却状态机流转）
type Scheduler interface {

	// Schedule 执行一次调度，返回选中的账号和供应商信息
	//
	// 调度模式（由 ScheduleRequest.ProviderKey 决定）：
	//   模式 1 - 精确指定供应商 (ProviderKey.Type + Name 都非空)
	//     → 直接从该供应商下的可用账号中选号
	//   模式 2 - 按类型筛选 (仅 ProviderKey.Type 非空)
	//     → 从该类型下所有支持 Model 的活跃供应商中，用 GroupSelector 选供应商，再选号
	//   模式 3 - 全自动 (ProviderKey 为 nil)
	//     → 从所有支持 Model 的活跃供应商中，用 GroupSelector 选供应商，再选号
	//
	// 故障转移：
	//   当 EnableFailover=true 且当前供应商下无可用账号时，
	//   自动排除该供应商，从剩余候选中重新选择，直到成功或候选耗尽。
	//
	// 重试：
	//   当 MaxRetries>0 时，选号失败后排除已尝试的账号 ID 重新选号，
	//   直到成功或重试次数耗尽。
	Schedule(ctx context.Context, req *ScheduleRequest) (*ScheduleResult, error)

	// ReportSuccess 上报调用成功
	//
	// 行为：
	//   1. 更新账号统计：TotalCalls++, SuccessCalls++, ConsecutiveFailures=0
	//   2. 通知 CircuitBreaker.RecordSuccess（如已配置）
	//   3. 持久化到 AccountStorage
	ReportSuccess(ctx context.Context, accountID string) error

	// ReportFailure 上报调用失败
	//
	// 行为：
	//   1. 更新账号统计：TotalCalls++, FailedCalls++, ConsecutiveFailures++
	//   2. 通知 CircuitBreaker.RecordFailure（如已配置）
	//      - 如果触发熔断 → 状态切换为 CircuitOpen，设置 CircuitOpenUntil
	//   3. 判断是否为限流错误 → 通知 CooldownManager.StartCooldown（如已配置）
	//      - 状态切换为 CoolingDown，设置 CooldownUntil
	//   4. 持久化到 AccountStorage
	//
	// callErr: 实际调用的错误，用于判断错误类型（如限流 vs 服务端错误）
	ReportFailure(ctx context.Context, accountID string, callErr error) error
}
