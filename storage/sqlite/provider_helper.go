package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-client/usagerule"
)

const (
	providerSelectColumns = `provider_type, provider_name, status, priority, weight,
		tags, supported_models, usage_rules, metadata,
		account_count, available_account_count, created_at, updated_at`

	queryGetProvider    = `SELECT ` + providerSelectColumns + ` FROM providers WHERE provider_type=? AND provider_name=?`
	queryInsertProvider = `INSERT INTO providers (` + providerSelectColumns + `) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	queryUpdateProvider = `UPDATE providers SET status=?, priority=?, weight=?,
		tags=?, supported_models=?, usage_rules=?, metadata=?,
		account_count=?, available_account_count=?, updated_at=?
		WHERE provider_type=? AND provider_name=?`
	queryDeleteProvider = `DELETE FROM providers WHERE provider_type=? AND provider_name=?`
)

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

func scanProviderFields(s Scanner) (*account.ProviderInfo, error) {
	var (
		info           account.ProviderInfo
		statusInt      int
		tagsJSON       sql.NullString
		modelsJSON     sql.NullString
		usageRulesJSON sql.NullString
		metadataJSON   sql.NullString
		createdAtStr   string
		updatedAtStr   string
	)

	err := s.Scan(
		&info.ProviderType, &info.ProviderName,
		&statusInt, &info.Priority, &info.Weight,
		&tagsJSON, &modelsJSON, &usageRulesJSON, &metadataJSON,
		&info.AccountCount, &info.AvailableAccountCount,
		&createdAtStr, &updatedAtStr,
	)
	if err != nil {
		return nil, err
	}

	return buildProviderInfo(&info, statusInt, tagsJSON, modelsJSON, usageRulesJSON, metadataJSON, createdAtStr, updatedAtStr)
}

func buildProviderInfo(info *account.ProviderInfo, statusInt int, tagsJSON, modelsJSON, usageRulesJSON, metadataJSON sql.NullString, createdAtStr, updatedAtStr string) (*account.ProviderInfo, error) {
	info.Status = account.ProviderStatus(statusInt)

	info.CreatedAt, _ = parseTime(createdAtStr)
	info.UpdatedAt, _ = parseTime(updatedAtStr)

	if tagsJSON.Valid && tagsJSON.String != "" {
		if err := json.Unmarshal([]byte(tagsJSON.String), &info.Tags); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
		}
	}
	if modelsJSON.Valid && modelsJSON.String != "" {
		if err := json.Unmarshal([]byte(modelsJSON.String), &info.SupportedModels); err != nil {
			return nil, fmt.Errorf("failed to unmarshal supported_models: %w", err)
		}
	}
	if usageRulesJSON.Valid && usageRulesJSON.String != "" {
		if err := json.Unmarshal([]byte(usageRulesJSON.String), &info.UsageRules); err != nil {
			return nil, fmt.Errorf("failed to unmarshal usage_rules: %w", err)
		}
	}
	if metadataJSON.Valid && metadataJSON.String != "" {
		if err := json.Unmarshal([]byte(metadataJSON.String), &info.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return info, nil
}

var _ = (*usagerule.UsageRule)(nil)
