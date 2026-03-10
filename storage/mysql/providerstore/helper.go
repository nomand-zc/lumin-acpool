package providerstore

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-client/usagerule"
)

const (
	// providerSelectColumns 是 providers 表的通用查询列。
	providerSelectColumns = `provider_type, provider_name, status, priority, weight, 
		tags, supported_models, usage_rules, metadata, 
		account_count, available_account_count, created_at, updated_at`

	// queryGetProvider 根据 provider_type 和 provider_name 查询单个供应商。
	queryGetProvider = `SELECT ` + providerSelectColumns + ` FROM providers WHERE provider_type=? AND provider_name=?`

	// queryInsertProvider 插入新供应商。
	queryInsertProvider = `INSERT INTO providers (` + providerSelectColumns + `) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	// queryUpdateProvider 更新供应商信息。
	queryUpdateProvider = `UPDATE providers SET status=?, priority=?, weight=?, 
		tags=?, supported_models=?, usage_rules=?, metadata=?, 
		account_count=?, available_account_count=?, updated_at=? 
		WHERE provider_type=? AND provider_name=?`

	// queryDeleteProvider 根据 provider_type 和 provider_name 删除供应商。
	queryDeleteProvider = `DELETE FROM providers WHERE provider_type=? AND provider_name=?`
)

// providerFieldMapping 定义逻辑字段名到数据库列名的映射。
var providerFieldMapping = map[string]string{
	storage.ProviderFieldType:                  "provider_type",
	storage.ProviderFieldName:                  "provider_name",
	storage.ProviderFieldStatus:                "status",
	storage.ProviderFieldPriority:              "priority",
	storage.ProviderFieldWeight:                "weight",
	storage.ProviderFieldSupportedModel:        "supported_models",
	storage.ProviderFieldAccountCount:          "account_count",
	storage.ProviderFieldAvailableAccountCount: "available_account_count",
	storage.ProviderFieldCreatedAt:             "created_at",
	storage.ProviderFieldUpdatedAt:             "updated_at",
}

// scanner 是 sql.Row 和 sql.Rows 的通用 Scan 接口。
type scanner interface {
	Scan(dest ...any) error
}

// scanProviderFields 从扫描结果中构建 ProviderInfo 对象。
func scanProviderFields(s scanner) (*account.ProviderInfo, error) {
	var (
		info           account.ProviderInfo
		statusInt      int
		tagsJSON       sql.NullString
		modelsJSON     sql.NullString
		usageRulesJSON sql.NullString
		metadataJSON   sql.NullString
	)

	err := s.Scan(
		&info.ProviderType, &info.ProviderName,
		&statusInt, &info.Priority, &info.Weight,
		&tagsJSON, &modelsJSON, &usageRulesJSON, &metadataJSON,
		&info.AccountCount, &info.AvailableAccountCount,
		&info.CreatedAt, &info.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	info.Status = account.ProviderStatus(statusInt)

	// 解析 tags
	if tagsJSON.Valid && tagsJSON.String != "" {
		if err := json.Unmarshal([]byte(tagsJSON.String), &info.Tags); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
		}
	}

	// 解析 supported_models
	if modelsJSON.Valid && modelsJSON.String != "" {
		if err := json.Unmarshal([]byte(modelsJSON.String), &info.SupportedModels); err != nil {
			return nil, fmt.Errorf("failed to unmarshal supported_models: %w", err)
		}
	}

	// 解析 usage_rules
	if usageRulesJSON.Valid && usageRulesJSON.String != "" {
		if err := json.Unmarshal([]byte(usageRulesJSON.String), &info.UsageRules); err != nil {
			return nil, fmt.Errorf("failed to unmarshal usage_rules: %w", err)
		}
	}

	// 解析 metadata
	if metadataJSON.Valid && metadataJSON.String != "" {
		if err := json.Unmarshal([]byte(metadataJSON.String), &info.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &info, nil
}

func scanProvider(row *sql.Row) (*account.ProviderInfo, error) {
	return scanProviderFields(row)
}

func scanProviderFromRows(rows *sql.Rows) (*account.ProviderInfo, error) {
	return scanProviderFields(rows)
}

// marshalJSON 将任意值序列化为 JSON，nil 返回 nil。
func marshalJSON(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}

// isDuplicateEntry 判断是否为主键冲突错误。
func isDuplicateEntry(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "Duplicate entry") || strings.Contains(err.Error(), "1062")
}

// 以下用于消除未使用的导入警告。
var _ = (*usagerule.UsageRule)(nil)
