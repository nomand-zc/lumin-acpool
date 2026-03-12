package providerstore

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"strings"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-acpool/storage/filtercond"
	storeMysql "github.com/nomand-zc/lumin-acpool/storage/mysql"
)

//go:embed providers.sql
var providersTableSQL string

// Compile-time interface compliance check.
var _ storage.ProviderStorage = (*Store)(nil)

// Store 是基于 MySQL 的 ProviderStorage 实现。
type Store struct {
	client    storeMysql.Client
	converter *storeMysql.MysqlConverter
}

// NewStore 创建一个新的 MySQL 供应商存储实例。
// 通过 Options 传递 InstanceName 或 DSN 来创建 Client，并在 SkipInitDB 为 false 时自动创建 providers 表。
func NewStore(opts ...Option) (*Store, error) {
	o := DefaultOptions()
	for _, opt := range opts {
		opt(o)
	}

	client, err := buildClient(o)
	if err != nil {
		return nil, fmt.Errorf("providerstore: %w", err)
	}

	store := &Store{
		client:    client,
		converter: storeMysql.NewConditionConverter(providerFieldMapping),
	}

	if !o.SkipInitDB {
		if err := store.initDB(); err != nil {
			return nil, fmt.Errorf("providerstore: %w", err)
		}
	}

	return store, nil
}

// initDB 执行建表 DDL，初始化 providers 表。
func (s *Store) initDB() error {
	_, err := s.client.Exec(context.Background(), providersTableSQL)
	if err != nil {
		return fmt.Errorf("failed to init providers table: %w", err)
	}
	return nil
}

func (s *Store) Get(ctx context.Context, key account.ProviderKey) (*account.ProviderInfo, error) {
	var info *account.ProviderInfo
	err := s.client.Query(ctx, func(rows *sql.Rows) error {
		if !rows.Next() {
			return nil
		}
		var scanErr error
		info, scanErr = scanProviderFromRows(rows)
		return scanErr
	}, queryGetProvider, key.Type, key.Name)
	if err != nil {
		return nil, fmt.Errorf("providerstore: failed to get provider: %w", err)
	}
	if info == nil {
		return nil, storage.ErrNotFound
	}
	return info, nil
}

func (s *Store) Search(ctx context.Context, filter *storage.SearchFilter) ([]*account.ProviderInfo, error) {
	var extraCond *filtercond.Filter
	if filter != nil {
		extraCond = filter.ExtraCond
	}
	condResult, err := s.converter.Convert(extraCond)
	if err != nil {
		return nil, fmt.Errorf("providerstore: failed to convert filter: %w", err)
	}

	query := fmt.Sprintf(`SELECT `+providerSelectColumns+` FROM providers WHERE %s`, buildProviderWhereClause(filter, condResult))
	args := buildProviderWhereArgs(filter, condResult)

	var result []*account.ProviderInfo
	err = s.client.Query(ctx, func(rows *sql.Rows) error {
		for rows.Next() {
			info, scanErr := scanProviderFromRows(rows)
			if scanErr != nil {
				return fmt.Errorf("providerstore: failed to scan provider: %w", scanErr)
			}
			result = append(result, info)
		}
		return nil
	}, query, args...)
	if err != nil {
		return nil, fmt.Errorf("providerstore: failed to search providers: %w", err)
	}
	return result, nil
}

// buildProviderWhereClause 根据 SearchFilter 一级字段和 ExtraCond 构建 WHERE 子句。
func buildProviderWhereClause(filter *storage.SearchFilter, condResult *storeMysql.CondConvertResult) string {
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

// buildProviderWhereArgs 根据 SearchFilter 一级字段和 ExtraCond 构建查询参数。
func buildProviderWhereArgs(filter *storage.SearchFilter, condResult *storeMysql.CondConvertResult) []any {
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

func (s *Store) Add(ctx context.Context, info *account.ProviderInfo) error {
	now := time.Now()
	createdAt := info.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}

	tagsJSON, err := storeMysql.MarshalJSON(info.Tags)
	if err != nil {
		return fmt.Errorf("providerstore: failed to marshal tags: %w", err)
	}
	modelsJSON, err := storeMysql.MarshalJSON(info.SupportedModels)
	if err != nil {
		return fmt.Errorf("providerstore: failed to marshal supported_models: %w", err)
	}
	usageRulesJSON, err := storeMysql.MarshalJSON(info.UsageRules)
	if err != nil {
		return fmt.Errorf("providerstore: failed to marshal usage_rules: %w", err)
	}
	metadataJSON, err := storeMysql.MarshalJSON(info.Metadata)
	if err != nil {
		return fmt.Errorf("providerstore: failed to marshal metadata: %w", err)
	}

	_, err = s.client.Exec(ctx, queryInsertProvider,
		info.ProviderType, info.ProviderName,
		int(info.Status), info.Priority, info.Weight,
		tagsJSON, modelsJSON, usageRulesJSON, metadataJSON,
		info.AccountCount, info.AvailableAccountCount,
		createdAt, now,
	)
	if err != nil {
		if storeMysql.IsDuplicateEntry(err) {
			return storage.ErrAlreadyExists
		}
		return fmt.Errorf("providerstore: failed to add provider: %w", err)
	}
	return nil
}

func (s *Store) Update(ctx context.Context, info *account.ProviderInfo) error {
	tagsJSON, err := storeMysql.MarshalJSON(info.Tags)
	if err != nil {
		return fmt.Errorf("providerstore: failed to marshal tags: %w", err)
	}
	modelsJSON, err := storeMysql.MarshalJSON(info.SupportedModels)
	if err != nil {
		return fmt.Errorf("providerstore: failed to marshal supported_models: %w", err)
	}
	usageRulesJSON, err := storeMysql.MarshalJSON(info.UsageRules)
	if err != nil {
		return fmt.Errorf("providerstore: failed to marshal usage_rules: %w", err)
	}
	metadataJSON, err := storeMysql.MarshalJSON(info.Metadata)
	if err != nil {
		return fmt.Errorf("providerstore: failed to marshal metadata: %w", err)
	}

	result, err := s.client.Exec(ctx, queryUpdateProvider,
		int(info.Status), info.Priority, info.Weight,
		tagsJSON, modelsJSON, usageRulesJSON, metadataJSON,
		info.AccountCount, info.AvailableAccountCount,
		time.Now(), info.ProviderType, info.ProviderName,
	)
	if err != nil {
		return fmt.Errorf("providerstore: failed to update provider: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("providerstore: failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return storage.ErrNotFound
	}
	return nil
}

func (s *Store) Remove(ctx context.Context, key account.ProviderKey) error {
	result, err := s.client.Exec(ctx, queryDeleteProvider, key.Type, key.Name)
	if err != nil {
		return fmt.Errorf("providerstore: failed to remove provider: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("providerstore: failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return storage.ErrNotFound
	}
	return nil
}
