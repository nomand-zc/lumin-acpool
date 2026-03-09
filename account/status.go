package account

// Status 账号状态枚举
type Status int

const (
	// StatusAvailable 可用：账号正常，可参与选号
	StatusAvailable Status = iota + 1
	// StatusCoolingDown 冷却中：触发限流，等待冷却结束后自动恢复
	StatusCoolingDown
	// StatusCircuitOpen 熔断开启：连续失败过多，暂时不参与选号
	StatusCircuitOpen
	// StatusExpired 凭证过期：Token 已过期，需刷新后恢复
	StatusExpired
	// StatusInvalidated 永久失效：凭证无法恢复（如 refresh token 无效）
	StatusInvalidated
	// StatusBanned 被封禁：被平台封禁，需人工介入
	StatusBanned
	// StatusDisabled 手动禁用：管理员手动禁用
	StatusDisabled
)

// IsSelectable 判断账号是否可参与选号
func (s Status) IsSelectable() bool {
	return s == StatusAvailable
}

// IsRecoverable 判断账号是否有可能自动恢复
func (s Status) IsRecoverable() bool {
	switch s {
	case StatusCoolingDown, StatusCircuitOpen, StatusExpired:
		return true
	default:
		return false
	}
}

// String 返回状态的可读字符串表示
func (s Status) String() string {
	switch s {
	case StatusAvailable:
		return "available"
	case StatusCoolingDown:
		return "cooling_down"
	case StatusCircuitOpen:
		return "circuit_open"
	case StatusExpired:
		return "expired"
	case StatusInvalidated:
		return "invalidated"
	case StatusBanned:
		return "banned"
	case StatusDisabled:
		return "disabled"
	default:
		return "unknown"
	}
}
