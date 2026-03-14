package redis

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-acpool/storage/filtercond"
	"github.com/nomand-zc/lumin-client/credentials"
)

// ============================
// Account Redis Key 格式定义
// ============================

const (
	acctKeyDataPrefix     = "account:"
	acctKeyGlobalIndex    = "accounts:index"
	acctKeyTypePrefix     = "accounts:type:"
	acctKeyProviderPrefix = "accounts:provider:"
	acctKeyGroupPrefix    = "accounts:group:"
)

// Account Hash 字段名常量。
const (
	acctFieldID               = "id"
	acctFieldProviderType     = "provider_type"
	acctFieldProviderName     = "provider_name"
	acctFieldCredential       = "credential"
	acctFieldStatus           = "status"
	acctFieldPriority         = "priority"
	acctFieldTags             = "tags"
	acctFieldMetadata         = "metadata"
	acctFieldUsageRules       = "usage_rules"
	acctFieldCooldownUntil    = "cooldown_until"
	acctFieldCircuitOpenUntil = "circuit_open_until"
	acctFieldCreatedAt        = "created_at"
	acctFieldUpdatedAt        = "updated_at"
	acctFieldVersion          = "version"
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
// Account Key 生成函数
// ============================

func acctDataKey(prefix, id string) string {
	return prefix + acctKeyDataPrefix + id
}

func acctGlobalIndexKey(prefix string) string {
	return prefix + acctKeyGlobalIndex
}

func acctTypeIndexKey(prefix, providerType string) string {
	return prefix + acctKeyTypePrefix + providerType
}

func acctProviderIndexKey(prefix, providerType, providerName string) string {
	return prefix + acctKeyProviderPrefix + providerType + "/" + providerName
}

func acctGroupIndexKey(prefix, providerType, providerName string, status int) string {
	return fmt.Sprintf("%s%s%s/%s/%d", prefix, acctKeyGroupPrefix, providerType, providerName, status)
}

type acctIndexKeys struct {
	global   string
	typeIdx  string
	provider string
	group    string
}

func acctAllIndexKeys(prefix, providerType, providerName string, status int) acctIndexKeys {
	return acctIndexKeys{
		global:   acctGlobalIndexKey(prefix),
		typeIdx:  acctTypeIndexKey(prefix, providerType),
		provider: acctProviderIndexKey(prefix, providerType, providerName),
		group:    acctGroupIndexKey(prefix, providerType, providerName, status),
	}
}

// ============================
// Account Filter 索引下推
// ============================

type indexCondition struct {
	providerType string
	providerName string
	statusValues []int
	residual     *filtercond.Filter
}

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

	if cond.providerType == "" && cond.providerName == "" && len(cond.statusValues) == 0 {
		return nil
	}

	cond.residual = nil
	return cond
}

func (ic *indexCondition) resolveIndexKeys(prefix string) (keys []string, allPushed bool) {
	hasType := ic.providerType != ""
	hasName := ic.providerName != ""
	hasStatus := len(ic.statusValues) > 0

	if hasType && hasName && hasStatus {
		for _, s := range ic.statusValues {
			keys = append(keys, acctGroupIndexKey(prefix, ic.providerType, ic.providerName, s))
		}
		return keys, true
	}

	if hasType && hasName {
		keys = append(keys, acctProviderIndexKey(prefix, ic.providerType, ic.providerName))
		return keys, !hasStatus
	}

	if hasType {
		keys = append(keys, acctTypeIndexKey(prefix, ic.providerType))
		return keys, !hasName && !hasStatus
	}

	return nil, false
}

// ============================
// Account 类型转换辅助
// ============================

func acctToIntValue(v any) *int {
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

func acctFormatInt(v int) string {
	return strconv.Itoa(v)
}

// ============================
// Account 序列化/反序列化
// ============================

func marshalAccountToHash(acct *account.Account) (map[string]any, error) {
	credentialJSON, err := json.Marshal(acct.Credential.ToMap())
	if err != nil {
		return nil, fmt.Errorf("failed to marshal credential: %w", err)
	}
	tagsJSON, err := MarshalJSON(acct.Tags)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tags: %w", err)
	}
	metadataJSON, err := MarshalJSON(acct.Metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}
	usageRulesJSON, err := MarshalJSON(acct.UsageRules)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal usage_rules: %w", err)
	}

	fields := map[string]any{
		acctFieldID:               acct.ID,
		acctFieldProviderType:     acct.ProviderType,
		acctFieldProviderName:     acct.ProviderName,
		acctFieldCredential:       string(credentialJSON),
		acctFieldStatus:           int(acct.Status),
		acctFieldPriority:         acct.Priority,
		acctFieldTags:             tagsJSON,
		acctFieldMetadata:         metadataJSON,
		acctFieldUsageRules:       usageRulesJSON,
		acctFieldCooldownUntil:    FormatTimePtr(acct.CooldownUntil),
		acctFieldCircuitOpenUntil: FormatTimePtr(acct.CircuitOpenUntil),
		acctFieldCreatedAt:        FormatTime(acct.CreatedAt),
		acctFieldUpdatedAt:        FormatTime(acct.UpdatedAt),
		acctFieldVersion:          acct.Version,
	}

	return fields, nil
}

func unmarshalAccountFromHash(data map[string]string) (*account.Account, error) {
	if len(data) == 0 {
		return nil, nil
	}

	acct := &account.Account{
		ID:           data[acctFieldID],
		ProviderType: data[acctFieldProviderType],
		ProviderName: data[acctFieldProviderName],
		Status:       account.Status(ParseInt(data[acctFieldStatus])),
		Priority:     ParseInt(data[acctFieldPriority]),
		Version:      ParseInt(data[acctFieldVersion]),
	}

	if s := data[acctFieldCreatedAt]; s != "" {
		if t, err := ParseTime(s); err == nil {
			acct.CreatedAt = t
		}
	}
	if s := data[acctFieldUpdatedAt]; s != "" {
		if t, err := ParseTime(s); err == nil {
			acct.UpdatedAt = t
		}
	}
	acct.CooldownUntil = ParseTimePtr(data[acctFieldCooldownUntil])
	acct.CircuitOpenUntil = ParseTimePtr(data[acctFieldCircuitOpenUntil])

	if s := data[acctFieldCredential]; s != "" {
		cred, err := acctUnmarshalCredential(acct.ProviderType, []byte(s))
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal credential: %w", err)
		}
		acct.Credential = cred
	}

	if s := data[acctFieldTags]; s != "" {
		if err := json.Unmarshal([]byte(s), &acct.Tags); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
		}
	}
	if s := data[acctFieldMetadata]; s != "" {
		if err := json.Unmarshal([]byte(s), &acct.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}
	if s := data[acctFieldUsageRules]; s != "" {
		if err := json.Unmarshal([]byte(s), &acct.UsageRules); err != nil {
			return nil, fmt.Errorf("failed to unmarshal usage_rules: %w", err)
		}
	}

	return acct, nil
}

func acctUnmarshalCredential(providerType string, data []byte) (credentials.Credential, error) {
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
