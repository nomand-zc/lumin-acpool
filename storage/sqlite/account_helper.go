package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-client/credentials"
)

const (
	accountSelectColumns = `id, provider_type, provider_name, credential, status, priority, 
		tags, metadata, usage_rules, cooldown_until, circuit_open_until, created_at, updated_at, version`

	queryGetAccount    = `SELECT ` + accountSelectColumns + ` FROM accounts WHERE id = ?`
	queryInsertAccount = `INSERT INTO accounts (` + accountSelectColumns + `) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	queryUpdateAccount = `UPDATE accounts SET provider_type=?, provider_name=?, credential=?, status=?, priority=?, 
		tags=?, metadata=?, usage_rules=?, cooldown_until=?, circuit_open_until=?, updated_at=?, 
		version=version+1 
		WHERE id=? AND version=?`
	queryDeleteAccount = `DELETE FROM accounts WHERE id=?`

	// queryIncrProviderAccountCount 增加供应商的账号计数。
	// 参数: available_incr(0或1), updated_at, provider_type, provider_name
	queryIncrProviderAccountCount = `UPDATE providers SET account_count = account_count + 1, 
		available_account_count = available_account_count + ?, updated_at = ? 
		WHERE provider_type = ? AND provider_name = ?`

	// queryDecrProviderAccountCount 减少供应商的账号计数。
	// 参数: available_decr(0或1), updated_at, provider_type, provider_name
	queryDecrProviderAccountCount = `UPDATE providers SET account_count = MAX(account_count - 1, 0), 
		available_account_count = MAX(available_account_count - ?, 0), updated_at = ? 
		WHERE provider_type = ? AND provider_name = ?`
)

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

func scanAccountFields(s Scanner) (*account.Account, error) {
	var (
		acct             account.Account
		credentialJSON   []byte
		statusInt        int
		tagsJSON         sql.NullString
		metadataJSON     sql.NullString
		usageRulesJSON   sql.NullString
		cooldownUntil    sql.NullString
		circuitOpenUntil sql.NullString
		createdAtStr     string
		updatedAtStr     string
	)

	err := s.Scan(
		&acct.ID, &acct.ProviderType, &acct.ProviderName,
		&credentialJSON, &statusInt, &acct.Priority,
		&tagsJSON, &metadataJSON, &usageRulesJSON,
		&cooldownUntil, &circuitOpenUntil,
		&createdAtStr, &updatedAtStr, &acct.Version,
	)
	if err != nil {
		return nil, err
	}

	if t, parseErr := parseTime(createdAtStr); parseErr == nil {
		acct.CreatedAt = t
	}
	if t, parseErr := parseTime(updatedAtStr); parseErr == nil {
		acct.UpdatedAt = t
	}

	return buildAccountInfo(&acct, credentialJSON, statusInt, tagsJSON, metadataJSON, usageRulesJSON, cooldownUntil, circuitOpenUntil)
}

func buildAccountInfo(acct *account.Account, credentialJSON []byte, statusInt int, tagsJSON, metadataJSON, usageRulesJSON, cooldownUntil, circuitOpenUntil sql.NullString) (*account.Account, error) {
	acct.Status = account.Status(statusInt)

	if len(credentialJSON) > 0 {
		cred, err := unmarshalCredential(acct.ProviderType, credentialJSON)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal credential: %w", err)
		}
		acct.Credential = cred
	}

	if tagsJSON.Valid && tagsJSON.String != "" {
		if err := json.Unmarshal([]byte(tagsJSON.String), &acct.Tags); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
		}
	}
	if metadataJSON.Valid && metadataJSON.String != "" {
		if err := json.Unmarshal([]byte(metadataJSON.String), &acct.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}
	if usageRulesJSON.Valid && usageRulesJSON.String != "" {
		if err := json.Unmarshal([]byte(usageRulesJSON.String), &acct.UsageRules); err != nil {
			return nil, fmt.Errorf("failed to unmarshal usage_rules: %w", err)
		}
	}

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
