package provider

import (
	"slices"
	"time"

	"github.com/nomand-zc/lumin-client/providers"
	"github.com/nomand-zc/lumin-client/usagerule"
)

// ProviderKey 供应商两级标识，Type + Name 唯一确定一个供应商分组
type ProviderKey struct {
	// Type 供应商类型，对应 lumin-client 的 Provider.Type()，如 "kiro"
	Type string
	// Name 供应商实例名称，对应 lumin-client 的 Provider.Name()，如 "kiro-team-a"
	Name string
}

// String 返回 "type/name" 格式的字符串表示
func (pk ProviderKey) String() string {
	return pk.Type + "/" + pk.Name
}

// ProviderStatus 供应商状态
type ProviderStatus int

const (
	// ProviderStatusActive 正常启用
	ProviderStatusActive ProviderStatus = 1
	// ProviderStatusDisabled 手动禁用
	ProviderStatusDisabled ProviderStatus = 2
	// ProviderStatusDegraded 降级（部分模型不可用或用量紧张）
	ProviderStatusDegraded ProviderStatus = 3
)

// ProviderInfo 供应商元数据，描述一个 Provider 实例的静态信息和运行时状态
type ProviderInfo struct {
	// Key 供应商唯一标识
	Key ProviderKey
	// Status 当前状态
	Status ProviderStatus
	// Priority 优先级，数值越大优先级越高（默认 0）
	Priority int
	// Weight 权重，用于加权选择（默认 1）
	Weight int
	// Tags 标签集合，用于分类筛选
	Tags map[string]string
	// SupportedModels 该供应商支持的模型列表
	SupportedModels []string
	// UsageRules 该供应商关联的用量规则（从 lumin-client 获取后存储）
	UsageRules []*usagerule.UsageRule
	// Metadata 扩展元数据
	Metadata map[string]any

	// --- 运行时统计 ---

	// AccountCount 该分组下的账号总数
	AccountCount int
	// AvailableAccountCount 可用账号数量
	AvailableAccountCount int

	// --- 时间戳 ---

	// CreatedAt 创建时间
	CreatedAt time.Time
	// UpdatedAt 最后更新时间
	UpdatedAt time.Time
}

// SupportsModel 判断该供应商是否支持指定模型
func (p *ProviderInfo) SupportsModel(model string) bool {
	return slices.Contains(p.SupportedModels, model)
}

// IsActive 判断供应商是否处于活跃状态
func (p *ProviderInfo) IsActive() bool {
	return p.Status == ProviderStatusActive || p.Status == ProviderStatusDegraded
}

// ProviderInstance 供应商运行时实例
// 将元数据（ProviderInfo）和底层 SDK 实例（providers.Provider）绑定在一起
type ProviderInstance struct {
	// Info 供应商元数据
	Info *ProviderInfo
	// Client 底层 lumin-client 的 Provider 实例，用于实际的 API 调用
	Client providers.Provider
}
