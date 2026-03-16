package storage

// UpdateField 标识 UpdateAccount 时需要更新的字段集合（位掩码）。
// 调用方通过按位或组合多个字段，存储层据此动态构建更新语句，仅更新指定字段。
type UpdateField uint16

const (
	// UpdateFieldCredential 凭证信息（Token 刷新后需要持久化新凭证）。
	UpdateFieldCredential UpdateField = 1 << iota

	// UpdateFieldStatus 状态及关联的冷却/熔断时间。
	// 包含 Status、CooldownUntil、CircuitOpenUntil 三个字段，
	// 因为这三者在所有场景中总是一起变更。
	// 当状态发生变更时，存储层应在事务内同步更新 Provider 的 AvailableAccountCount。
	UpdateFieldStatus

	// UpdateFieldPriority 优先级。
	UpdateFieldPriority

	// UpdateFieldTags 标签集合。
	UpdateFieldTags

	// UpdateFieldMetadata 扩展元数据。
	UpdateFieldMetadata

	// UpdateFieldUsageRules 用量规则。
	UpdateFieldUsageRules
)

// Has 判断是否包含指定字段。
func (f UpdateField) Has(field UpdateField) bool {
	return f&field != 0
}

// String 返回字段掩码的可读字符串表示（用于日志/调试）。
func (f UpdateField) String() string {
	if f == 0 {
		return "none"
	}

	names := make([]string, 0, 6)
	if f.Has(UpdateFieldCredential) {
		names = append(names, "Credential")
	}
	if f.Has(UpdateFieldStatus) {
		names = append(names, "Status")
	}
	if f.Has(UpdateFieldPriority) {
		names = append(names, "Priority")
	}
	if f.Has(UpdateFieldTags) {
		names = append(names, "Tags")
	}
	if f.Has(UpdateFieldMetadata) {
		names = append(names, "Metadata")
	}
	if f.Has(UpdateFieldUsageRules) {
		names = append(names, "UsageRules")
	}

	result := ""
	for i, name := range names {
		if i > 0 {
			result += "|"
		}
		result += name
	}
	return result
}
