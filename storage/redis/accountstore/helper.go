package accountstore

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-acpool/storage/filtercond"
	storeRedis "github.com/nomand-zc/lumin-acpool/storage/redis"
	"github.com/nomand-zc/lumin-client/credentials"
)

// ============================
// Redis Key 格式定义
// ============================
//
// 数据存储：
//   account:{id}                                                → Hash，存储单个账号的所有字段
//
// 全局索引：
//   accounts:index                                              → Set，存储所有账号 ID
//
// 三层分级索引（写入时同步维护，查询时按条件自动选取最优索引）：
//   accounts:type:{provider_type}                               → Set  (层级 1: 按 provider_type)
//   accounts:provider:{provider_type}/{provider_name}           → Set  (层级 2: 按 provider_type + provider_name)
//   accounts:group:{provider_type}/{provider_name}/{status}     → Set  (层级 3: 按 provider_type + provider_name + status)

const (
	keyAccountPrefix  = "account:"
	keyAccountIndex   = "accounts:index"
	keyTypePrefix     = "accounts:type:"
	keyProviderPrefix = "accounts:provider:"
	keyGroupPrefix    = "accounts:group:"
)

// Hash 字段名常量。
const (
	fieldID               = "id"
	fieldProviderType     = "provider_type"
	fieldProviderName     = "provider_name"
	fieldCredential       = "credential"
	fieldStatus           = "status"
	fieldPriority         = "priority"
	fieldTags             = "tags"
	fieldMetadata         = "metadata"
	fieldUsageRules       = "usage_rules"
	fieldCooldownUntil    = "cooldown_until"
	fieldCircuitOpenUntil = "circuit_open_until"
	fieldCreatedAt        = "created_at"
	fieldUpdatedAt        = "updated_at"
	fieldVersion          = "version"
)

// accountFieldExtractor 定义逻辑字段名到 account 属性的提取函数，供 FilterEvaluator 使用。
var accountFieldExtractor = func(obj any, field string) (any, bool) {
	acct, ok := obj.(*account.Account)
	if !ok {
		return nil, false
	}
	switch field {
	case storage.AccountFieldID:
		return acct.ID, true
	case storage.AccountFieldProviderType:
		return acct.ProviderType, true
	case storage.AccountFieldProviderName:
		return acct.ProviderName, true
	case storage.AccountFieldStatus:
		return int(acct.Status), true
	case storage.AccountFieldPriority:
		return acct.Priority, true
	case storage.AccountFieldCooldownUntil:
		if acct.CooldownUntil != nil {
			return *acct.CooldownUntil, true
		}
		return nil, true
	case storage.AccountFieldCircuitOpenUntil:
		if acct.CircuitOpenUntil != nil {
			return *acct.CircuitOpenUntil, true
		}
		return nil, true
	case storage.AccountFieldCreatedAt:
		return acct.CreatedAt, true
	case storage.AccountFieldUpdatedAt:
		return acct.UpdatedAt, true
	default:
		return nil, false
	}
}

// ============================
// Key 生成函数
// ============================

// accountKey 返回账号数据的 Redis Hash key。
func accountKey(prefix, id string) string {
	return prefix + keyAccountPrefix + id
}

// accountIndexKey 返回全局索引的 Redis Set key。
func accountIndexKey(prefix string) string {
	return prefix + keyAccountIndex
}

// typeIndexKey 返回 provider_type 层级索引的 Redis Set key（层级 1）。
func typeIndexKey(prefix, providerType string) string {
	return prefix + keyTypePrefix + providerType
}

// providerIndexKey 返回 provider_type + provider_name 层级索引的 Redis Set key（层级 2）。
func providerIndexKey(prefix, providerType, providerName string) string {
	return prefix + keyProviderPrefix + providerType + "/" + providerName
}

// groupIndexKey 返回 provider_type + provider_name + status 组合索引的 Redis Set key（层级 3）。
func groupIndexKey(prefix, providerType, providerName string, status int) string {
	return fmt.Sprintf("%s%s%s/%s/%d", prefix, keyGroupPrefix, providerType, providerName, status)
}

// indexKeys 包含一个账号关联的全部索引 key。
type indexKeys struct {
	global   string // accounts:index
	typeIdx  string // accounts:type:{type}
	provider string // accounts:provider:{type}/{name}
	group    string // accounts:group:{type}/{name}/{status}
}

// allIndexKeys 返回一个账号关联的全部索引 key（写入/删除时维护）。
func allIndexKeys(prefix, providerType, providerName string, status int) indexKeys {
	return indexKeys{
		global:   accountIndexKey(prefix),
		typeIdx:  typeIndexKey(prefix, providerType),
		provider: providerIndexKey(prefix, providerType, providerName),
		group:    groupIndexKey(prefix, providerType, providerName, status),
	}
}

// ============================
// Filter 索引下推
// ============================

// indexCondition 保存从 Filter 中提取出的可用于索引下推的等值条件。
type indexCondition struct {
	providerType string // 精确匹配的 provider_type
	providerName string // 精确匹配的 provider_name
	statusValues []int  // 精确匹配或 IN 匹配的 status 值列表
	residual     *filtercond.Filter // 去除已下推条件后的残余 Filter（用于内存过滤）
}

// extractIndexFromSearchFilter 从 SearchFilter 一级字段直接构建索引条件，
// 无需再从 filtercond.Filter 树中解析。
func extractIndexFromSearchFilter(filter *storage.SearchFilter) *indexCondition {
	if filter == nil {
		return nil
	}

	cond := &indexCondition{
		providerType: filter.ProviderType,
		providerName: filter.ProviderName,
	}
	if filter.Status != 0 {
		cond.statusValues = []int{filter.Status}
	}

	// 如果什么都没提取到，返回 nil
	if cond.providerType == "" && cond.providerName == "" && len(cond.statusValues) == 0 {
		return nil
	}

	// residual 仅用于 ExtraCond，已由调用方处理
	cond.residual = nil
	return cond
}

// extractIndexCondition 从 Filter 中提取可下推到 Redis 索引的等值条件。
// 仅处理顶层 AND 组合或单条件，不递归到 OR 内部。
// 返回提取结果。如果无法提取任何索引条件，返回 nil。
func extractIndexCondition(filter *filtercond.Filter) *indexCondition {
	if filter == nil {
		return nil
	}

	cond := &indexCondition{}
	var residuals []*filtercond.Filter

	// 收集待检查的条件列表
	var conditions []*filtercond.Filter
	if filter.Operator == filtercond.OperatorAnd {
		children, ok := filter.Value.([]*filtercond.Filter)
		if !ok {
			return nil
		}
		conditions = children
	} else {
		conditions = []*filtercond.Filter{filter}
	}

	for _, c := range conditions {
		pushed := false
		switch {
		case c.Field == storage.AccountFieldProviderType && c.Operator == filtercond.OperatorEqual:
			if v, ok := c.Value.(string); ok {
				cond.providerType = v
				pushed = true
			}
		case c.Field == storage.AccountFieldProviderName && c.Operator == filtercond.OperatorEqual:
			if v, ok := c.Value.(string); ok {
				cond.providerName = v
				pushed = true
			}
		case c.Field == storage.AccountFieldStatus && c.Operator == filtercond.OperatorEqual:
			if v := toIntValue(c.Value); v != nil {
				cond.statusValues = []int{*v}
				pushed = true
			}
		case c.Field == storage.AccountFieldStatus && c.Operator == filtercond.OperatorIn:
			if vals := toIntSlice(c.Value); len(vals) > 0 {
				cond.statusValues = vals
				pushed = true
			}
		}
		if !pushed {
			residuals = append(residuals, c)
		}
	}

	// 如果什么都没提取到，返回 nil
	if cond.providerType == "" && cond.providerName == "" && len(cond.statusValues) == 0 {
		return nil
	}

	// 构建残余 Filter
	switch len(residuals) {
	case 0:
		cond.residual = nil
	case 1:
		cond.residual = residuals[0]
	default:
		cond.residual = &filtercond.Filter{
			Operator: filtercond.OperatorAnd,
			Value:    residuals,
		}
	}

	return cond
}

// resolveIndexKeys 根据提取到的索引条件，生成应该查询的 Redis Set key 列表。
//
// 索引选取策略（按精确度从高到低，优先选取最精确的索引）：
//
//	层级 3: provider_type + provider_name + status 都有 → 使用 group 组合索引
//	层级 2: provider_type + provider_name              → 使用 provider 索引
//	层级 1: 仅 provider_type                           → 使用 type 索引
//
// 返回值：
//
//	keys      - 需要查询的 Redis Set key 列表（多个 key 表示取并集）
//	allPushed - 是否所有提取到的索引条件都已被索引覆盖（决定是否还需内存过滤这些字段）
func (ic *indexCondition) resolveIndexKeys(prefix string) (keys []string, allPushed bool) {
	hasType := ic.providerType != ""
	hasName := ic.providerName != ""
	hasStatus := len(ic.statusValues) > 0

	// 层级 3: 三者齐全 → group 组合索引
	if hasType && hasName && hasStatus {
		for _, s := range ic.statusValues {
			keys = append(keys, groupIndexKey(prefix, ic.providerType, ic.providerName, s))
		}
		return keys, true
	}

	// 层级 2: provider_type + provider_name → provider 索引
	if hasType && hasName {
		keys = append(keys, providerIndexKey(prefix, ic.providerType, ic.providerName))
		return keys, !hasStatus // status 未被索引覆盖，需内存过滤
	}

	// 层级 1: 仅 provider_type → type 索引
	if hasType {
		keys = append(keys, typeIndexKey(prefix, ic.providerType))
		return keys, !hasName && !hasStatus
	}

	// 其他情况（如仅有 status 或仅有 provider_name）无法利用索引
	return nil, false
}

// ============================
// 类型转换辅助
// ============================

// toIntValue 尝试将值转为 int 指针。
func toIntValue(v any) *int {
	switch val := v.(type) {
	case int:
		return &val
	case int64:
		i := int(val)
		return &i
	case float64:
		i := int(val)
		return &i
	case account.Status:
		i := int(val)
		return &i
	default:
		return nil
	}
}

// toIntSlice 尝试将 slice 值转为 []int。
func toIntSlice(v any) []int {
	switch vals := v.(type) {
	case []int:
		return vals
	case []any:
		result := make([]int, 0, len(vals))
		for _, item := range vals {
			if iv := toIntValue(item); iv != nil {
				result = append(result, *iv)
			}
		}
		return result
	case []account.Status:
		result := make([]int, 0, len(vals))
		for _, s := range vals {
			result = append(result, int(s))
		}
		return result
	default:
		return nil
	}
}

// formatInt 将 int 转为 string。
func formatInt(v int) string {
	return strconv.Itoa(v)
}

// ============================
// Account 序列化/反序列化
// ============================

// marshalAccountToHash 将 Account 序列化为 Redis Hash 字段值映射。
func marshalAccountToHash(acct *account.Account) (map[string]any, error) {
	credentialJSON, err := json.Marshal(acct.Credential.ToMap())
	if err != nil {
		return nil, fmt.Errorf("failed to marshal credential: %w", err)
	}
	tagsJSON, err := storeRedis.MarshalJSON(acct.Tags)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tags: %w", err)
	}
	metadataJSON, err := storeRedis.MarshalJSON(acct.Metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}
	usageRulesJSON, err := storeRedis.MarshalJSON(acct.UsageRules)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal usage_rules: %w", err)
	}

	fields := map[string]any{
		fieldID:               acct.ID,
		fieldProviderType:     acct.ProviderType,
		fieldProviderName:     acct.ProviderName,
		fieldCredential:       string(credentialJSON),
		fieldStatus:           int(acct.Status),
		fieldPriority:         acct.Priority,
		fieldTags:             tagsJSON,
		fieldMetadata:         metadataJSON,
		fieldUsageRules:       usageRulesJSON,
		fieldCooldownUntil:    storeRedis.FormatTimePtr(acct.CooldownUntil),
		fieldCircuitOpenUntil: storeRedis.FormatTimePtr(acct.CircuitOpenUntil),
		fieldCreatedAt:        storeRedis.FormatTime(acct.CreatedAt),
		fieldUpdatedAt:        storeRedis.FormatTime(acct.UpdatedAt),
		fieldVersion:          acct.Version,
	}

	return fields, nil
}

// unmarshalAccountFromHash 从 Redis Hash 字段值映射中反序列化 Account。
func unmarshalAccountFromHash(data map[string]string) (*account.Account, error) {
	if len(data) == 0 {
		return nil, nil
	}

	acct := &account.Account{
		ID:           data[fieldID],
		ProviderType: data[fieldProviderType],
		ProviderName: data[fieldProviderName],
		Status:       account.Status(storeRedis.ParseInt(data[fieldStatus])),
		Priority:     storeRedis.ParseInt(data[fieldPriority]),
		Version:      storeRedis.ParseInt(data[fieldVersion]),
	}

	// 解析时间
	if s := data[fieldCreatedAt]; s != "" {
		if t, err := storeRedis.ParseTime(s); err == nil {
			acct.CreatedAt = t
		}
	}
	if s := data[fieldUpdatedAt]; s != "" {
		if t, err := storeRedis.ParseTime(s); err == nil {
			acct.UpdatedAt = t
		}
	}
	acct.CooldownUntil = storeRedis.ParseTimePtr(data[fieldCooldownUntil])
	acct.CircuitOpenUntil = storeRedis.ParseTimePtr(data[fieldCircuitOpenUntil])

	// 解析 credential
	if s := data[fieldCredential]; s != "" {
		cred, err := unmarshalCredential(acct.ProviderType, []byte(s))
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal credential: %w", err)
		}
		acct.Credential = cred
	}

	// 解析 JSON 字段
	if s := data[fieldTags]; s != "" {
		if err := json.Unmarshal([]byte(s), &acct.Tags); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
		}
	}
	if s := data[fieldMetadata]; s != "" {
		if err := json.Unmarshal([]byte(s), &acct.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}
	if s := data[fieldUsageRules]; s != "" {
		if err := json.Unmarshal([]byte(s), &acct.UsageRules); err != nil {
			return nil, fmt.Errorf("failed to unmarshal usage_rules: %w", err)
		}
	}

	return acct, nil
}

// unmarshalCredential 根据 providerType 从 JSON 反序列化 Credential。
func unmarshalCredential(providerType string, data []byte) (credentials.Credential, error) {
	factory := credentials.GetFactory(providerType)
	if factory == nil {
		return nil, fmt.Errorf("no credential factory registered for provider type: %s", providerType)
	}
	cred := factory(data)
	if cred == nil {
		return nil, fmt.Errorf("credential factory returned nil for provider type: %s", providerType)
	}
	return cred, nil
}
