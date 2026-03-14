package mysql

import (
	"database/sql"
	"encoding/json"
	"fmt"

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

// scanProviderFields 从扫描结果中构建 ProviderInfo 对象。
func scanProviderFields(s Scanner) (*account.ProviderInfo, error) {
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

	return buildProviderInfo(&info, statusInt, tagsJSON, modelsJSON, usageRulesJSON, metadataJSON)
}

// buildProviderInfo 根据 QueryRow 扫描的原始字段值，构建完整的 ProviderInfo 对象。
func buildProviderInfo(info *account.ProviderInfo, statusInt int, tagsJSON, modelsJSON, usageRulesJSON, metadataJSON sql.NullString) (*account.ProviderInfo, error) {
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

	return info, nil
}

// 以下用于消除未使用的导入警告。
var _ = (*usagerule.UsageRule)(nil)
