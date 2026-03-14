package accountstore

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-acpool/storage/filtercond"
	storeMysql "github.com/nomand-zc/lumin-acpool/storage/mysql"
)

//go:embed accounts.sql
var accountsTableSQL string

// Compile-time interface compliance check.
var _ storage.AccountStorage = (*Store)(nil)

// Store 是基于 MySQL 的 AccountStorage 实现。
type Store struct {
	client    storeMysql.Client
	converter *storeMysql.MysqlConverter
}

// NewStore 创建一个新的 MySQL 账号存储实例。
// 通过 Options 传递 InstanceName 或 DSN 来创建 Client，并在 SkipInitDB 为 false 时自动创建 accounts 表。
func NewStore(opts ...Option) (*Store, error) {
	o := DefaultOptions()
	for _, opt := range opts {
		opt(o)
	}

	client, err := buildClient(o)
	if err != nil {
		return nil, fmt.Errorf("accountstore: %w", err)
	}

	store := &Store{
		client:    client,
		converter: storeMysql.NewConditionConverter(accountFieldMapping, nil),
	}

	if !o.SkipInitDB {
		if err := store.initDB(); err != nil {
			return nil, fmt.Errorf("accountstore: %w", err)
		}
	}

	return store, nil
}

// initDB 执行建表 DDL，初始化 accounts 表。
func (s *Store) initDB() error {
	_, err := s.client.Exec(context.Background(), accountsTableSQL)
	if err != nil {
		return fmt.Errorf("failed to init accounts table: %w", err)
	}
	return nil
}

func (s *Store) GetAccount(ctx context.Context, id string) (*account.Account, error) {
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

	dest := []any{
		&acct.ID, &acct.ProviderType, &acct.ProviderName,
		&credentialJSON, &statusInt, &acct.Priority,
		&tagsJSON, &metadataJSON, &usageRulesJSON,
		&cooldownUntil, &circuitOpenUntil,
		&acct.CreatedAt, &acct.UpdatedAt, &acct.Version,
	}

	err := s.client.QueryRow(ctx, dest, queryGetAccount, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, storage.ErrNotFound
		}
		return nil, fmt.Errorf("accountstore: failed to get account: %w", err)
	}

	result, err := buildAccountInfo(&acct, credentialJSON, statusInt, tagsJSON, metadataJSON, usageRulesJSON, cooldownUntil, circuitOpenUntil)
	if err != nil {
		return nil, fmt.Errorf("accountstore: failed to build account info: %w", err)
	}
	return result, nil
}

func (s *Store) SearchAccounts(ctx context.Context, filter *storage.SearchFilter) ([]*account.Account, error) {
	var extraCond *filtercond.Filter
	if filter != nil {
		extraCond = filter.ExtraCond
	}
	condResult, err := s.converter.Convert(extraCond)
	if err != nil {
		return nil, fmt.Errorf("accountstore: failed to convert filter: %w", err)
	}

	query := fmt.Sprintf(`SELECT `+accountSelectColumns+` FROM accounts WHERE %s`, s.buildWhereClause(filter, condResult))
	args := s.buildWhereArgs(filter, condResult)

	var result []*account.Account
	err = s.client.Query(ctx, func(rows *sql.Rows) error {
		acct, scanErr := scanAccountFields(rows)
		if scanErr != nil {
			return fmt.Errorf("accountstore: failed to scan account: %w", scanErr)
		}
		result = append(result, acct)
		return nil
	}, query, args...)
	if err != nil {
		return nil, fmt.Errorf("accountstore: failed to search accounts: %w", err)
	}
	return result, nil
}

// buildWhereClause 根据 SearchFilter 一级字段和 ExtraCond 构建 WHERE 子句。
func (s *Store) buildWhereClause(filter *storage.SearchFilter, condResult *storeMysql.CondConvertResult) string {
	parts := []string{}
	if filter != nil {
		if filter.ProviderType != "" {
			parts = append(parts, "provider_type=?")
		}
		if filter.ProviderName != "" {
			parts = append(parts, "provider_name=?")
		}
		if filter.Status != 0 {
			parts = append(parts, "status=?")
		}
	}
	parts = append(parts, condResult.Cond)
	return strings.Join(parts, " AND ")
}

// buildWhereArgs 根据 SearchFilter 一级字段和 ExtraCond 构建查询参数。
func (s *Store) buildWhereArgs(filter *storage.SearchFilter, condResult *storeMysql.CondConvertResult) []any {
	var args []any
	if filter != nil {
		if filter.ProviderType != "" {
			args = append(args, filter.ProviderType)
		}
		if filter.ProviderName != "" {
			args = append(args, filter.ProviderName)
		}
		if filter.Status != 0 {
			args = append(args, filter.Status)
		}
	}
	args = append(args, condResult.Args...)
	return args
}

func (s *Store) AddAccount(ctx context.Context, acct *account.Account) error {
	now := time.Now()
	createdAt := acct.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}

	credentialBytes, err := json.Marshal(acct.Credential.ToMap())
	if err != nil {
		return fmt.Errorf("accountstore: failed to marshal credential: %w", err)
	}
	credentialJSON := string(credentialBytes)
	tagsJSON, err := storeMysql.MarshalJSON(acct.Tags)
	if err != nil {
		return fmt.Errorf("accountstore: failed to marshal tags: %w", err)
	}
	metadataJSON, err := storeMysql.MarshalJSON(acct.Metadata)
	if err != nil {
		return fmt.Errorf("accountstore: failed to marshal metadata: %w", err)
	}
	usageRulesJSON, err := storeMysql.MarshalJSON(acct.UsageRules)
	if err != nil {
		return fmt.Errorf("accountstore: failed to marshal usage_rules: %w", err)
	}

	_, err = s.client.Exec(ctx, queryInsertAccount,
		acct.ID, acct.ProviderType, acct.ProviderName,
		credentialJSON, int(acct.Status), acct.Priority,
		tagsJSON, metadataJSON, usageRulesJSON,
		acct.CooldownUntil, acct.CircuitOpenUntil,
		createdAt, now, 1,
	)
	if err != nil {
		if storeMysql.IsDuplicateEntry(err) {
			return storage.ErrAlreadyExists
		}
		return fmt.Errorf("accountstore: failed to add account: %w", err)
	}
	return nil
}

func (s *Store) UpdateAccount(ctx context.Context, acct *account.Account) error {
	credentialBytes, err := json.Marshal(acct.Credential.ToMap())
	if err != nil {
		return fmt.Errorf("accountstore: failed to marshal credential: %w", err)
	}
	credentialJSON := string(credentialBytes)
	tagsJSON, err := storeMysql.MarshalJSON(acct.Tags)
	if err != nil {
		return fmt.Errorf("accountstore: failed to marshal tags: %w", err)
	}
	metadataJSON, err := storeMysql.MarshalJSON(acct.Metadata)
	if err != nil {
		return fmt.Errorf("accountstore: failed to marshal metadata: %w", err)
	}
	usageRulesJSON, err := storeMysql.MarshalJSON(acct.UsageRules)
	if err != nil {
		return fmt.Errorf("accountstore: failed to marshal usage_rules: %w", err)
	}

	result, err := s.client.Exec(ctx, queryUpdateAccount,
		acct.ProviderType, acct.ProviderName,
		credentialJSON, int(acct.Status), acct.Priority,
		tagsJSON, metadataJSON, usageRulesJSON,
		acct.CooldownUntil, acct.CircuitOpenUntil,
		time.Now(), acct.ID, acct.Version,
	)
	if err != nil {
		return fmt.Errorf("accountstore: failed to update account: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("accountstore: failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return storage.ErrVersionConflict
	}
	return nil
}

func (s *Store) RemoveAccount(ctx context.Context, id string) error {
	result, err := s.client.Exec(ctx, queryDeleteAccount, id)
	if err != nil {
		return fmt.Errorf("accountstore: failed to remove account: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("accountstore: failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return storage.ErrNotFound
	}
	return nil
}

func (s *Store) RemoveAccounts(ctx context.Context, filter *storage.SearchFilter) error {
	var extraCond *filtercond.Filter
	if filter != nil {
		extraCond = filter.ExtraCond
	}
	condResult, err := s.converter.Convert(extraCond)
	if err != nil {
		return fmt.Errorf("accountstore: failed to convert filter: %w", err)
	}

	query := fmt.Sprintf(`DELETE FROM accounts WHERE %s`, s.buildWhereClause(filter, condResult))
	args := s.buildWhereArgs(filter, condResult)

	_, err = s.client.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("accountstore: failed to remove accounts by filter: %w", err)
	}
	return nil
}

func (s *Store) CountAccounts(ctx context.Context, filter *storage.SearchFilter) (int, error) {
	var extraCond *filtercond.Filter
	if filter != nil {
		extraCond = filter.ExtraCond
	}
	condResult, err := s.converter.Convert(extraCond)
	if err != nil {
		return 0, fmt.Errorf("accountstore: failed to convert filter: %w", err)
	}

	query := fmt.Sprintf(`SELECT COUNT(*) FROM accounts WHERE %s`, s.buildWhereClause(filter, condResult))
	args := s.buildWhereArgs(filter, condResult)
	var count int
	err = s.client.QueryRow(ctx, []any{&count}, query, args...)
	if err != nil {
		return 0, fmt.Errorf("accountstore: failed to count accounts: %w", err)
	}
	return count, nil
}
