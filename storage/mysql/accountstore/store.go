package accountstore

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
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
		converter: storeMysql.NewConditionConverter(accountFieldMapping),
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

func (s *Store) Get(ctx context.Context, id string) (*account.Account, error) {
	var acct *account.Account
	err := s.client.Query(ctx, func(rows *sql.Rows) error {
		if !rows.Next() {
			return nil
		}
		var scanErr error
		acct, scanErr = scanAccountFields(rows)
		return scanErr
	}, queryGetAccount, id)
	if err != nil {
		return nil, fmt.Errorf("accountstore: failed to get account: %w", err)
	}
	if acct == nil {
		return nil, storage.ErrNotFound
	}
	return acct, nil
}

func (s *Store) Search(ctx context.Context, filter *filtercond.Filter) ([]*account.Account, error) {
	condResult, err := s.converter.Convert(filter)
	if err != nil {
		return nil, fmt.Errorf("accountstore: failed to convert filter: %w", err)
	}

	query := fmt.Sprintf(`SELECT `+accountSelectColumns+` FROM accounts WHERE %s`, condResult.Cond)

	var result []*account.Account
	err = s.client.Query(ctx, func(rows *sql.Rows) error {
		for rows.Next() {
			acct, scanErr := scanAccountFields(rows)
			if scanErr != nil {
				return fmt.Errorf("accountstore: failed to scan account: %w", scanErr)
			}
			result = append(result, acct)
		}
		return nil
	}, query, condResult.Args...)
	if err != nil {
		return nil, fmt.Errorf("accountstore: failed to search accounts: %w", err)
	}
	return result, nil
}

func (s *Store) Add(ctx context.Context, acct *account.Account) error {
	now := time.Now()
	createdAt := acct.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}

	credentialJSON, err := json.Marshal(acct.Credential.ToMap())
	if err != nil {
		return fmt.Errorf("accountstore: failed to marshal credential: %w", err)
	}
	tagsJSON, err := marshalJSON(acct.Tags)
	if err != nil {
		return fmt.Errorf("accountstore: failed to marshal tags: %w", err)
	}
	metadataJSON, err := marshalJSON(acct.Metadata)
	if err != nil {
		return fmt.Errorf("accountstore: failed to marshal metadata: %w", err)
	}
	usageRulesJSON, err := marshalJSON(acct.UsageRules)
	if err != nil {
		return fmt.Errorf("accountstore: failed to marshal usage_rules: %w", err)
	}

	_, err = s.client.Exec(ctx, queryInsertAccount,
		acct.ID, acct.ProviderType, acct.ProviderName,
		credentialJSON, int(acct.Status), acct.Priority,
		tagsJSON, metadataJSON, usageRulesJSON,
		acct.CooldownUntil, acct.CircuitOpenUntil,
		createdAt, now,
	)
	if err != nil {
		if isDuplicateEntry(err) {
			return storage.ErrAlreadyExists
		}
		return fmt.Errorf("accountstore: failed to add account: %w", err)
	}
	return nil
}

func (s *Store) Update(ctx context.Context, acct *account.Account) error {
	credentialJSON, err := json.Marshal(acct.Credential.ToMap())
	if err != nil {
		return fmt.Errorf("accountstore: failed to marshal credential: %w", err)
	}
	tagsJSON, err := marshalJSON(acct.Tags)
	if err != nil {
		return fmt.Errorf("accountstore: failed to marshal tags: %w", err)
	}
	metadataJSON, err := marshalJSON(acct.Metadata)
	if err != nil {
		return fmt.Errorf("accountstore: failed to marshal metadata: %w", err)
	}
	usageRulesJSON, err := marshalJSON(acct.UsageRules)
	if err != nil {
		return fmt.Errorf("accountstore: failed to marshal usage_rules: %w", err)
	}

	result, err := s.client.Exec(ctx, queryUpdateAccount,
		acct.ProviderType, acct.ProviderName,
		credentialJSON, int(acct.Status), acct.Priority,
		tagsJSON, metadataJSON, usageRulesJSON,
		acct.CooldownUntil, acct.CircuitOpenUntil,
		time.Now(), acct.ID,
	)
	if err != nil {
		return fmt.Errorf("accountstore: failed to update account: %w", err)
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

func (s *Store) Remove(ctx context.Context, id string) error {
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

func (s *Store) RemoveFilter(ctx context.Context, filter *filtercond.Filter) error {
	condResult, err := s.converter.Convert(filter)
	if err != nil {
		return fmt.Errorf("accountstore: failed to convert filter: %w", err)
	}

	query := fmt.Sprintf(`DELETE FROM accounts WHERE %s`, condResult.Cond)

	_, err = s.client.Exec(ctx, query, condResult.Args...)
	if err != nil {
		return fmt.Errorf("accountstore: failed to remove accounts by filter: %w", err)
	}
	return nil
}

func (s *Store) Count(ctx context.Context, filter *filtercond.Filter) (int, error) {
	condResult, err := s.converter.Convert(filter)
	if err != nil {
		return 0, fmt.Errorf("accountstore: failed to convert filter: %w", err)
	}

	query := fmt.Sprintf(`SELECT COUNT(*) FROM accounts WHERE %s`, condResult.Cond)
	var count int
	err = s.client.QueryRow(ctx, []any{&count}, query, condResult.Args...)
	if err != nil {
		return 0, fmt.Errorf("accountstore: failed to count accounts: %w", err)
	}
	return count, nil
}

func (s *Store) CountByProvider(ctx context.Context, key account.ProviderKey, filter *filtercond.Filter) (int, error) {
	condResult, err := s.converter.Convert(filter)
	if err != nil {
		return 0, fmt.Errorf("accountstore: failed to convert filter: %w", err)
	}

	query := fmt.Sprintf(`SELECT COUNT(*) FROM accounts WHERE provider_type=? AND provider_name=? AND (%s)`,
		condResult.Cond)
	args := append([]any{key.Type, key.Name}, condResult.Args...)

	var count int
	err = s.client.QueryRow(ctx, []any{&count}, query, args...)
	if err != nil {
		return 0, fmt.Errorf("accountstore: failed to count accounts by provider: %w", err)
	}
	return count, nil
}
