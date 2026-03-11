package providerstore

import (
	"encoding/json"
	"fmt"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	storeRedis "github.com/nomand-zc/lumin-acpool/storage/redis"
)

const (
	// Redis key 格式。
	// provider:{type}/{name} -> Hash，存储单个供应商的所有字段。
	// providers:index        -> Set，存储所有供应商 key（"type/name" 格式）。

	keyProviderPrefix = "provider:"
	keyProviderIndex  = "providers:index"
)

// Hash 字段名常量。
const (
	fieldProviderType           = "provider_type"
	fieldProviderName           = "provider_name"
	fieldStatus                 = "status"
	fieldPriority               = "priority"
	fieldWeight                 = "weight"
	fieldTags                   = "tags"
	fieldSupportedModels        = "supported_models"
	fieldUsageRules             = "usage_rules"
	fieldMetadata               = "metadata"
	fieldAccountCount           = "account_count"
	fieldAvailableAccountCount  = "available_account_count"
	fieldCreatedAt              = "created_at"
	fieldUpdatedAt              = "updated_at"
)

// providerFieldExtractor 定义逻辑字段名到 ProviderInfo 属性的提取函数。
var providerFieldExtractor = func(obj any, field string) (any, bool) {
	info, ok := obj.(*account.ProviderInfo)
	if !ok {
		return nil, false
	}
	switch field {
	case storage.ProviderFieldType:
		return info.ProviderType, true
	case storage.ProviderFieldName:
		return info.ProviderName, true
	case storage.ProviderFieldStatus:
		return int(info.Status), true
	case storage.ProviderFieldPriority:
		return info.Priority, true
	case storage.ProviderFieldWeight:
		return info.Weight, true
	case storage.ProviderFieldAccountCount:
		return info.AccountCount, true
	case storage.ProviderFieldAvailableAccountCount:
		return info.AvailableAccountCount, true
	case storage.ProviderFieldCreatedAt:
		return info.CreatedAt, true
	case storage.ProviderFieldUpdatedAt:
		return info.UpdatedAt, true
	default:
		return nil, false
	}
}

// providerRedisKey 返回供应商的 Redis Hash key。
func providerRedisKey(prefix, providerType, providerName string) string {
	return prefix + keyProviderPrefix + providerType + "/" + providerName
}

// providerIndexRedisKey 返回供应商索引的 Redis Set key。
func providerIndexRedisKey(prefix string) string {
	return prefix + keyProviderIndex
}

// providerIndexMember 返回供应商在索引 Set 中的成员值。
func providerIndexMember(providerType, providerName string) string {
	return providerType + "/" + providerName
}

// marshalProviderToHash 将 ProviderInfo 序列化为 Redis Hash 字段值映射。
func marshalProviderToHash(info *account.ProviderInfo) (map[string]any, error) {
	tagsJSON, err := storeRedis.MarshalJSON(info.Tags)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tags: %w", err)
	}
	modelsJSON, err := storeRedis.MarshalJSON(info.SupportedModels)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal supported_models: %w", err)
	}
	usageRulesJSON, err := storeRedis.MarshalJSON(info.UsageRules)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal usage_rules: %w", err)
	}
	metadataJSON, err := storeRedis.MarshalJSON(info.Metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}

	fields := map[string]any{
		fieldProviderType:          info.ProviderType,
		fieldProviderName:          info.ProviderName,
		fieldStatus:                int(info.Status),
		fieldPriority:              info.Priority,
		fieldWeight:                info.Weight,
		fieldTags:                  tagsJSON,
		fieldSupportedModels:       modelsJSON,
		fieldUsageRules:            usageRulesJSON,
		fieldMetadata:              metadataJSON,
		fieldAccountCount:          info.AccountCount,
		fieldAvailableAccountCount: info.AvailableAccountCount,
		fieldCreatedAt:             storeRedis.FormatTime(info.CreatedAt),
		fieldUpdatedAt:             storeRedis.FormatTime(info.UpdatedAt),
	}

	return fields, nil
}

// unmarshalProviderFromHash 从 Redis Hash 字段值映射中反序列化 ProviderInfo。
func unmarshalProviderFromHash(data map[string]string) (*account.ProviderInfo, error) {
	if len(data) == 0 {
		return nil, nil
	}

	info := &account.ProviderInfo{
		ProviderType:          data[fieldProviderType],
		ProviderName:          data[fieldProviderName],
		Status:                account.ProviderStatus(storeRedis.ParseInt(data[fieldStatus])),
		Priority:              storeRedis.ParseInt(data[fieldPriority]),
		Weight:                storeRedis.ParseInt(data[fieldWeight]),
		AccountCount:          storeRedis.ParseInt(data[fieldAccountCount]),
		AvailableAccountCount: storeRedis.ParseInt(data[fieldAvailableAccountCount]),
	}

	// 解析时间
	if s := data[fieldCreatedAt]; s != "" {
		if t, err := storeRedis.ParseTime(s); err == nil {
			info.CreatedAt = t
		}
	}
	if s := data[fieldUpdatedAt]; s != "" {
		if t, err := storeRedis.ParseTime(s); err == nil {
			info.UpdatedAt = t
		}
	}

	// 解析 JSON 字段
	if s := data[fieldTags]; s != "" {
		if err := json.Unmarshal([]byte(s), &info.Tags); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
		}
	}
	if s := data[fieldSupportedModels]; s != "" {
		if err := json.Unmarshal([]byte(s), &info.SupportedModels); err != nil {
			return nil, fmt.Errorf("failed to unmarshal supported_models: %w", err)
		}
	}
	if s := data[fieldUsageRules]; s != "" {
		if err := json.Unmarshal([]byte(s), &info.UsageRules); err != nil {
			return nil, fmt.Errorf("failed to unmarshal usage_rules: %w", err)
		}
	}
	if s := data[fieldMetadata]; s != "" {
		if err := json.Unmarshal([]byte(s), &info.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return info, nil
}
