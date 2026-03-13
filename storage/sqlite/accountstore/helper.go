package accountstore

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	storeSqlite "github.com/nomand-zc/lumin-acpool/storage/sqlite"
	"github.com/nomand-zc/lumin-client/credentials"
)

const (
	// accountSelectColumns 是 accounts 表的通用查询列。
	accountSelectColumns = `id, provider_type, provider_name, credential, status, priority, 
		tags, metadata, usage_rules, cooldown_until, circuit_open_until, created_at, updated_at, version`

	// queryGetAccount 根据 ID 查询单个账号。
	queryGetAccount = `SELECT ` + accountSelectColumns + ` FROM accounts WHERE id = ?`

	// queryInsertAccount 插入新账号。
	queryInsertAccount = `INSERT INTO accounts (` + accountSelectColumns + `) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	// queryUpdateAccount 更新账号信息（乐观锁：WHERE version=?，自动递增 version）。
	queryUpdateAccount = `UPDATE accounts SET provider_type=?, provider_name=?, credential=?, status=?, priority=?, 
		tags=?, metadata=?, usage_rules=?, cooldown_until=?, circuit_open_until=?, updated_at=?, 
		version=version+1 
		WHERE id=? AND version=?`

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

// scanAccountFields 从扫描结果中构建 Account 对象。
func scanAccountFields(s storeSqlite.Scanner) (*account.Account, error) {
	var (
		acct             account.Account
		credentialJSON   []byte
		statusInt        int
		tagsJSON         sql.NullString
		metadataJSON     sql.NullString
		usageRulesJSON   sql.NullString
		cooldownUntil    sql.NullString
		circuitOpenUntil sql.NullString
	)

	err := s.Scan(
		&acct.ID, &acct.ProviderType, &acct.ProviderName,
		&credentialJSON, &statusInt, &acct.Priority,
		&tagsJSON, &metadataJSON, &usageRulesJSON,
		&cooldownUntil, &circuitOpenUntil,
		&acct.CreatedAt, &acct.UpdatedAt, &acct.Version,
	)
	if err != nil {
		return nil, err
	}

	return buildAccountInfo(&acct, credentialJSON, statusInt, tagsJSON, metadataJSON, usageRulesJSON, cooldownUntil, circuitOpenUntil)
}

// buildAccountInfo 根据 QueryRow 扫描的原始字段值，构建完整的 Account 对象。
func buildAccountInfo(acct *account.Account, credentialJSON []byte, statusInt int, tagsJSON, metadataJSON, usageRulesJSON, cooldownUntil, circuitOpenUntil sql.NullString) (*account.Account, error) {
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

	// 解析时间字段（SQLite 存储为 TEXT）
	if cooldownUntil.Valid && cooldownUntil.String != "" {
		t, err := parseTime(cooldownUntil.String)
		if err == nil {
			acct.CooldownUntil = &t
		}
	}
	if circuitOpenUntil.Valid && circuitOpenUntil.String != "" {
		t, err := parseTime(circuitOpenUntil.String)
		if err == nil {
			acct.CircuitOpenUntil = &t
		}
	}

	return acct, nil
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

// parseTime 解析 SQLite 中存储的时间字符串。
// 支持多种格式，兼容不同的时间精度。
func parseTime(s string) (time.Time, error) {
	formats := []string{
		"2006-01-02 15:04:05.000",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05Z",
		time.RFC3339,
		time.RFC3339Nano,
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported time format: %s", s)
}
