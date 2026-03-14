package redis

import (
	"encoding/json"
	"fmt"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
)

const (
	provKeyDataPrefix = "provider:"
	provKeyIndex      = "providers:index"
)

// Provider Hash 字段名常量。
const (
	provFieldProviderType          = "provider_type"
	provFieldProviderName          = "provider_name"
	provFieldStatus                = "status"
	provFieldPriority              = "priority"
	provFieldWeight                = "weight"
	provFieldTags                  = "tags"
	provFieldSupportedModels       = "supported_models"
	provFieldUsageRules            = "usage_rules"
	provFieldMetadata              = "metadata"
	provFieldAccountCount          = "account_count"
	provFieldAvailableAccountCount = "available_account_count"
	provFieldCreatedAt             = "created_at"
	provFieldUpdatedAt             = "updated_at"
)

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

func provRedisKey(prefix, providerType, providerName string) string {
	return prefix + provKeyDataPrefix + providerType + "/" + providerName
}

func provIndexRedisKey(prefix string) string {
	return prefix + provKeyIndex
}

func provIndexMember(providerType, providerName string) string {
	return providerType + "/" + providerName
}

func marshalProviderToHash(info *account.ProviderInfo) (map[string]any, error) {
	tagsJSON, err := MarshalJSON(info.Tags)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tags: %w", err)
	}
	modelsJSON, err := MarshalJSON(info.SupportedModels)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal supported_models: %w", err)
	}
	usageRulesJSON, err := MarshalJSON(info.UsageRules)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal usage_rules: %w", err)
	}
	metadataJSON, err := MarshalJSON(info.Metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}

	fields := map[string]any{
		provFieldProviderType:          info.ProviderType,
		provFieldProviderName:          info.ProviderName,
		provFieldStatus:                int(info.Status),
		provFieldPriority:              info.Priority,
		provFieldWeight:                info.Weight,
		provFieldTags:                  tagsJSON,
		provFieldSupportedModels:       modelsJSON,
		provFieldUsageRules:            usageRulesJSON,
		provFieldMetadata:              metadataJSON,
		provFieldAccountCount:          info.AccountCount,
		provFieldAvailableAccountCount: info.AvailableAccountCount,
		provFieldCreatedAt:             FormatTime(info.CreatedAt),
		provFieldUpdatedAt:             FormatTime(info.UpdatedAt),
	}

	return fields, nil
}

func unmarshalProviderFromHash(data map[string]string) (*account.ProviderInfo, error) {
	if len(data) == 0 {
		return nil, nil
	}

	info := &account.ProviderInfo{
		ProviderType:          data[provFieldProviderType],
		ProviderName:          data[provFieldProviderName],
		Status:                account.ProviderStatus(ParseInt(data[provFieldStatus])),
		Priority:              ParseInt(data[provFieldPriority]),
		Weight:                ParseInt(data[provFieldWeight]),
		AccountCount:          ParseInt(data[provFieldAccountCount]),
		AvailableAccountCount: ParseInt(data[provFieldAvailableAccountCount]),
	}

	if s := data[provFieldCreatedAt]; s != "" {
		if t, err := ParseTime(s); err == nil {
			info.CreatedAt = t
		}
	}
	if s := data[provFieldUpdatedAt]; s != "" {
		if t, err := ParseTime(s); err == nil {
			info.UpdatedAt = t
		}
	}

	if s := data[provFieldTags]; s != "" {
		if err := json.Unmarshal([]byte(s), &info.Tags); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
		}
	}
	if s := data[provFieldSupportedModels]; s != "" {
		if err := json.Unmarshal([]byte(s), &info.SupportedModels); err != nil {
			return nil, fmt.Errorf("failed to unmarshal supported_models: %w", err)
		}
	}
	if s := data[provFieldUsageRules]; s != "" {
		if err := json.Unmarshal([]byte(s), &info.UsageRules); err != nil {
			return nil, fmt.Errorf("failed to unmarshal usage_rules: %w", err)
		}
	}
	if s := data[provFieldMetadata]; s != "" {
		if err := json.Unmarshal([]byte(s), &info.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return info, nil
}
