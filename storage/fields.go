package storage

// Account 可过滤字段常量
const (
	// AccountFieldID 账号 ID
	AccountFieldID = "id"
	// AccountFieldProviderType 供应商类型
	AccountFieldProviderType = "provider_type"
	// AccountFieldProviderName 供应商实例名称
	AccountFieldProviderName = "provider_name"
	// AccountFieldStatus 账号状态
	AccountFieldStatus = "status"
	// AccountFieldPriority 优先级
	AccountFieldPriority = "priority"
	// AccountFieldTotalCalls 总调用次数
	AccountFieldTotalCalls = "total_calls"
	// AccountFieldSuccessCalls 成功调用次数
	AccountFieldSuccessCalls = "success_calls"
	// AccountFieldFailedCalls 失败调用次数
	AccountFieldFailedCalls = "failed_calls"
	// AccountFieldConsecutiveFailures 连续失败次数
	AccountFieldConsecutiveFailures = "consecutive_failures"
	// AccountFieldLastUsedAt 上次使用时间
	AccountFieldLastUsedAt = "last_used_at"
	// AccountFieldLastErrorAt 上次调用失败时间
	AccountFieldLastErrorAt = "last_error_at"
	// AccountFieldLastErrorMsg 上次调用失败错误信息
	AccountFieldLastErrorMsg = "last_error_msg"
	// AccountFieldCooldownUntil 冷却截止时间
	AccountFieldCooldownUntil = "cooldown_until"
	// AccountFieldCircuitOpenUntil 熔断截止时间
	AccountFieldCircuitOpenUntil = "circuit_open_until"
	// AccountFieldCreatedAt 创建时间
	AccountFieldCreatedAt = "created_at"
	// AccountFieldUpdatedAt 最后更新时间
	AccountFieldUpdatedAt = "updated_at"
)

// ProviderInfo 可过滤字段常量
const (
	// ProviderFieldType 供应商类型
	ProviderFieldType = "provider_type"
	// ProviderFieldName 供应商实例名称
	ProviderFieldName = "provider_name"
	// ProviderFieldStatus 供应商状态
	ProviderFieldStatus = "provider_status"
	// ProviderFieldPriority 优先级
	ProviderFieldPriority = "priority"
	// ProviderFieldWeight 权重
	ProviderFieldWeight = "weight"
	// ProviderFieldSupportedModel 支持的模型
	ProviderFieldSupportedModel = "supported_model"
	// ProviderFieldAccountCount 账号总数
	ProviderFieldAccountCount = "account_count"
	// ProviderFieldAvailableAccountCount 可用账号数
	ProviderFieldAvailableAccountCount = "available_account_count"
	// ProviderFieldCreatedAt 创建时间
	ProviderFieldCreatedAt = "created_at"
	// ProviderFieldUpdatedAt 最后更新时间
	ProviderFieldUpdatedAt = "updated_at"
)
