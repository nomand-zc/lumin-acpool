package accountstore

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-client/credentials"
)

const (
	// accountSelectColumns 是 accounts 表的通用查询列。
	accountSelectColumns = `id, provider_type, provider_name, credential, status, priority, 
		tags, metadata, usage_rules, cooldown_until, circuit_open_until, created_at, updated_at`

	// queryGetAccount 根据 ID 查询单个账号。
	queryGetAccount = `SELECT ` + accountSelectColumns + ` FROM accounts WHERE id = ?`

	// queryInsertAccount 插入新账号。
	queryInsertAccount = `INSERT INTO accounts (` + accountSelectColumns + `) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	// queryUpdateAccount 更新账号信息。
	queryUpdateAccount = `UPDATE accounts SET provider_type=?, provider_name=?, credential=?, status=?, priority=?, 
		tags=?, metadata=?, usage_rules=?, cooldown_until=?, circuit_open_until=?, updated_at=? 
		WHERE id=?`

	// queryDeleteAccount 根据 ID 删除账号。
	queryDeleteAccount = `DELETE FROM accounts WHERE id=?`
)

// accountFieldMapping 定义逻辑字段名到数据库列名的映射。
var accountFieldMapping = map[string]string{
	storage.AccountFieldID:               "id",
	storage.AccountFieldProviderType:     "provider_type",
	storage.AccountFieldProviderName:     "provider_name",
	storage.AccountFieldStatus:           "status",
	storage.AccountFieldPriority:         "priority",
	storage.AccountFieldCooldownUntil:    "cooldown_until",
	storage.AccountFieldCircuitOpenUntil: "circuit_open_until",
	storage.AccountFieldCreatedAt:        "created_at",
	storage.AccountFieldUpdatedAt:        "updated_at",
}

// scanner 是 sql.Row 和 sql.Rows 的通用 Scan 接口。
type scanner interface {
	Scan(dest ...any) error
}

// scanAccountFields 从扫描结果中构建 Account 对象。
func scanAccountFields(s scanner) (*account.Account, error) {
	var (
		acct             account.Account
		credentialJSON   []byte
		statusInt        int
		tagsJSON         sql.NullString
		metadataJSON     sql.NullString
		usageRulesJSON   sql.NullString
		cooldownUntil    sql.NullTime
		circuitOpenUntil sql.NullTime
	)

	err := s.Scan(
		&acct.ID, &acct.ProviderType, &acct.ProviderName,
		&credentialJSON, &statusInt, &acct.Priority,
		&tagsJSON, &metadataJSON, &usageRulesJSON,
		&cooldownUntil, &circuitOpenUntil,
		&acct.CreatedAt, &acct.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	acct.Status = account.Status(statusInt)

	// 解析 credential
	if len(credentialJSON) > 0 {
		cred, err := unmarshalCredential(acct.ProviderType, credentialJSON)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal credential: %w", err)
		}
		acct.Credential = cred
	}

	// 解析 tags
	if tagsJSON.Valid && tagsJSON.String != "" {
		if err := json.Unmarshal([]byte(tagsJSON.String), &acct.Tags); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
		}
	}

	// 解析 metadata
	if metadataJSON.Valid && metadataJSON.String != "" {
		if err := json.Unmarshal([]byte(metadataJSON.String), &acct.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	// 解析 usage_rules
	if usageRulesJSON.Valid && usageRulesJSON.String != "" {
		if err := json.Unmarshal([]byte(usageRulesJSON.String), &acct.UsageRules); err != nil {
			return nil, fmt.Errorf("failed to unmarshal usage_rules: %w", err)
		}
	}

	if cooldownUntil.Valid {
		acct.CooldownUntil = &cooldownUntil.Time
	}
	if circuitOpenUntil.Valid {
		acct.CircuitOpenUntil = &circuitOpenUntil.Time
	}

	return &acct, nil
}

// unmarshalCredential 根据 providerType 获取对应的凭证工厂方法，从 JSON 反序列化 Credential。
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
