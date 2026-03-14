package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-acpool/storage/filtercond"
)

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
		return nil, fmt.Errorf("mysql store: failed to get account: %w", err)
	}

	result, err := buildAccountInfo(&acct, credentialJSON, statusInt, tagsJSON, metadataJSON, usageRulesJSON, cooldownUntil, circuitOpenUntil)
	if err != nil {
		return nil, fmt.Errorf("mysql store: failed to build account info: %w", err)
	}
	return result, nil
}

func (s *Store) SearchAccounts(ctx context.Context, filter *storage.SearchFilter) ([]*account.Account, error) {
	var extraCond *filtercond.Filter
	if filter != nil {
		extraCond = filter.ExtraCond
	}
	condResult, err := s.accountConverter.Convert(extraCond)
	if err != nil {
		return nil, fmt.Errorf("mysql store: failed to convert filter: %w", err)
	}

	query := fmt.Sprintf(`SELECT `+accountSelectColumns+` FROM accounts WHERE %s`, buildAccountWhereClause(filter, condResult))
	args := buildAccountWhereArgs(filter, condResult)

	var result []*account.Account
	err = s.client.Query(ctx, func(rows *sql.Rows) error {
		acct, scanErr := scanAccountFields(rows)
		if scanErr != nil {
			return fmt.Errorf("mysql store: failed to scan account: %w", scanErr)
		}
		result = append(result, acct)
		return nil
	}, query, args...)
	if err != nil {
		return nil, fmt.Errorf("mysql store: failed to search accounts: %w", err)
	}
	return result, nil
}

// buildAccountWhereClause 根据 SearchFilter 一级字段和 ExtraCond 构建 WHERE 子句。
func buildAccountWhereClause(filter *storage.SearchFilter, condResult *CondConvertResult) string {
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

// buildAccountWhereArgs 根据 SearchFilter 一级字段和 ExtraCond 构建查询参数。
func buildAccountWhereArgs(filter *storage.SearchFilter, condResult *CondConvertResult) []any {
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
		return fmt.Errorf("mysql store: failed to marshal credential: %w", err)
	}
	credentialJSON := string(credentialBytes)
	tagsJSON, err := MarshalJSON(acct.Tags)
	if err != nil {
		return fmt.Errorf("mysql store: failed to marshal tags: %w", err)
	}
	metadataJSON, err := MarshalJSON(acct.Metadata)
	if err != nil {
		return fmt.Errorf("mysql store: failed to marshal metadata: %w", err)
	}
	usageRulesJSON, err := MarshalJSON(acct.UsageRules)
	if err != nil {
		return fmt.Errorf("mysql store: failed to marshal usage_rules: %w", err)
	}

	availableIncr := 0
	if acct.Status == account.StatusAvailable {
		availableIncr = 1
	}

	return s.client.Transaction(ctx, func(tx *sql.Tx) error {
		_, txErr := tx.ExecContext(ctx, queryInsertAccount,
			acct.ID, acct.ProviderType, acct.ProviderName,
			credentialJSON, int(acct.Status), acct.Priority,
			tagsJSON, metadataJSON, usageRulesJSON,
			acct.CooldownUntil, acct.CircuitOpenUntil,
			createdAt, now, 1,
		)
		if txErr != nil {
			if IsDuplicateEntry(txErr) {
				return storage.ErrAlreadyExists
			}
			return fmt.Errorf("mysql store: failed to add account: %w", txErr)
		}

		_, txErr = tx.ExecContext(ctx, queryIncrProviderAccountCount,
			availableIncr, acct.ProviderType, acct.ProviderName)
		if txErr != nil {
			return fmt.Errorf("mysql store: failed to incr provider account count: %w", txErr)
		}

		return nil
	})
}

func (s *Store) UpdateAccount(ctx context.Context, acct *account.Account) error {
	credentialBytes, err := json.Marshal(acct.Credential.ToMap())
	if err != nil {
		return fmt.Errorf("mysql store: failed to marshal credential: %w", err)
	}
	credentialJSON := string(credentialBytes)
	tagsJSON, err := MarshalJSON(acct.Tags)
	if err != nil {
		return fmt.Errorf("mysql store: failed to marshal tags: %w", err)
	}
	metadataJSON, err := MarshalJSON(acct.Metadata)
	if err != nil {
		return fmt.Errorf("mysql store: failed to marshal metadata: %w", err)
	}
	usageRulesJSON, err := MarshalJSON(acct.UsageRules)
	if err != nil {
		return fmt.Errorf("mysql store: failed to marshal usage_rules: %w", err)
	}

	result, err := s.client.Exec(ctx, queryUpdateAccount,
		acct.ProviderType, acct.ProviderName,
		credentialJSON, int(acct.Status), acct.Priority,
		tagsJSON, metadataJSON, usageRulesJSON,
		acct.CooldownUntil, acct.CircuitOpenUntil,
		time.Now(), acct.ID, acct.Version,
	)
	if err != nil {
		return fmt.Errorf("mysql store: failed to update account: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("mysql store: failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return storage.ErrVersionConflict
	}
	return nil
}

func (s *Store) RemoveAccount(ctx context.Context, id string) error {
	return s.client.Transaction(ctx, func(tx *sql.Tx) error {
		// 在事务内查询账号信息，保证读取和删除的原子性
		var providerType, providerName string
		var statusInt int
		row := tx.QueryRowContext(ctx, `SELECT provider_type, provider_name, status FROM accounts WHERE id=?`, id)
		if txErr := row.Scan(&providerType, &providerName, &statusInt); txErr != nil {
			if txErr == sql.ErrNoRows {
				return storage.ErrNotFound
			}
			return fmt.Errorf("mysql store: failed to get account for removal: %w", txErr)
		}

		availableDecr := 0
		if account.Status(statusInt) == account.StatusAvailable {
			availableDecr = 1
		}

		// 删除账号，外键 ON DELETE CASCADE 会自动级联删除 account_stats、tracked_usages、account_occupancy。
		_, txErr := tx.ExecContext(ctx, queryDeleteAccount, id)
		if txErr != nil {
			return fmt.Errorf("mysql store: failed to remove account: %w", txErr)
		}

		_, txErr = tx.ExecContext(ctx, queryDecrProviderAccountCount,
			availableDecr, providerType, providerName)
		if txErr != nil {
			return fmt.Errorf("mysql store: failed to decr provider account count: %w", txErr)
		}

		return nil
	})
}

func (s *Store) RemoveAccounts(ctx context.Context, filter *storage.SearchFilter) error {
	var extraCond *filtercond.Filter
	if filter != nil {
		extraCond = filter.ExtraCond
	}
	condResult, err := s.accountConverter.Convert(extraCond)
	if err != nil {
		return fmt.Errorf("mysql store: failed to convert filter: %w", err)
	}

	query := fmt.Sprintf(`DELETE FROM accounts WHERE %s`, buildAccountWhereClause(filter, condResult))
	args := buildAccountWhereArgs(filter, condResult)

	_, err = s.client.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("mysql store: failed to remove accounts by filter: %w", err)
	}
	return nil
}

func (s *Store) CountAccounts(ctx context.Context, filter *storage.SearchFilter) (int, error) {
	var extraCond *filtercond.Filter
	if filter != nil {
		extraCond = filter.ExtraCond
	}
	condResult, err := s.accountConverter.Convert(extraCond)
	if err != nil {
		return 0, fmt.Errorf("mysql store: failed to convert filter: %w", err)
	}

	query := fmt.Sprintf(`SELECT COUNT(*) FROM accounts WHERE %s`, buildAccountWhereClause(filter, condResult))
	args := buildAccountWhereArgs(filter, condResult)
	var count int
	err = s.client.QueryRow(ctx, []any{&count}, query, args...)
	if err != nil {
		return 0, fmt.Errorf("mysql store: failed to count accounts: %w", err)
	}
	return count, nil
}
