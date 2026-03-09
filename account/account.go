package account

import (
	"time"

	"github.com/nomand-zc/lumin-client/credentials"
	"github.com/nomand-zc/lumin-client/usagerule"
)

// Account 账号聚合根，代表一个被管理的 AI 平台账号
type Account struct {
	// ID 账号唯一标识
	ID string
	// ProviderType 供应商类型，对应 lumin-client 的 Provider.Type()，如 "kiro"
	ProviderType string
	// ProviderName 供应商实例名称，对应 lumin-client 的 Provider.Name()，如 "kiro-team-a"
	ProviderName string
	// Credential 底层凭证，对应 lumin-client 的 credentials.Credential
	Credential credentials.Credential
	// Status 当前状态
	Status Status
	// Priority 优先级，数值越大优先级越高（默认 0）
	Priority int
	// Tags 标签集合，用于灵活分类和筛选
	Tags map[string]string
	// Metadata 扩展元数据，存放自定义业务字段
	Metadata map[string]any

	// --- 用量信息 ---

	// UsageStats 当前凭证的用量统计快照
	// 由健康巡检定期从 lumin-client 刷新
	UsageStats []*usagerule.UsageStats

	// --- 运行时统计 ---

	// TotalCalls 总调用次数
	TotalCalls int64
	// SuccessCalls 成功调用次数
	SuccessCalls int64
	// FailedCalls 失败调用次数
	FailedCalls int64
	// ConsecutiveFailures 当前连续失败次数（成功后重置为 0）
	ConsecutiveFailures int
	// LastUsedAt 上次被选中使用的时间
	LastUsedAt *time.Time
	// LastErrorAt 上次调用失败的时间
	LastErrorAt *time.Time
	// LastErrorMsg 上次调用失败的错误信息
	LastErrorMsg string

	// --- 冷却/熔断 ---

	// CooldownUntil 冷却截止时间，仅 StatusCoolingDown 时有效
	CooldownUntil *time.Time
	// CircuitOpenUntil 熔断截止时间，仅 StatusCircuitOpen 时有效
	CircuitOpenUntil *time.Time

	// --- 时间戳 ---

	// CreatedAt 创建时间
	CreatedAt time.Time
	// UpdatedAt 最后更新时间
	UpdatedAt time.Time
}

// SuccessRate 计算成功率，无调用时返回 1.0
func (a *Account) SuccessRate() float64 {
	if a.TotalCalls == 0 {
		return 1.0
	}
	return float64(a.SuccessCalls) / float64(a.TotalCalls)
}

// IsUsageLimited 判断是否有任何用量规则已触发限制
func (a *Account) IsUsageLimited() bool {
	for _, s := range a.UsageStats {
		if s != nil && s.IsTriggered() {
			return true
		}
	}
	return false
}

// UsageRemainRatio 返回最小剩余用量比例（0.0 ~ 1.0）
// 用于选号策略中评估账号的"宽裕程度"
func (a *Account) UsageRemainRatio() float64 {
	minRatio := 1.0
	for _, s := range a.UsageStats {
		if s == nil || s.Rule == nil || s.Rule.Total <= 0 {
			continue
		}
		ratio := s.Remain / s.Rule.Total
		if ratio < minRatio {
			minRatio = ratio
		}
	}
	return minRatio
}

// IsCooldownExpired 判断冷却是否已到期
func (a *Account) IsCooldownExpired() bool {
	if a.CooldownUntil == nil {
		return true
	}
	return time.Now().After(*a.CooldownUntil)
}

// IsCircuitOpenExpired 判断熔断是否已到期（可进入半开状态）
func (a *Account) IsCircuitOpenExpired() bool {
	if a.CircuitOpenUntil == nil {
		return true
	}
	return time.Now().After(*a.CircuitOpenUntil)
}
