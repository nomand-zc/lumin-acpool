package providerstore

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
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

func (s *Store) Search(ctx context.Context, filter *filtercond.Filter) ([]*account.ProviderInfo, error) {
	condResult, err := s.converter.Convert(filter)
	if err != nil {
		return nil, fmt.Errorf("providerstore: failed to convert filter: %w", err)
	}

	query := fmt.Sprintf(`SELECT `+providerSelectColumns+` FROM providers WHERE %s`, condResult.Cond)

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
	}, query, condResult.Args...)
	if err != nil {
		return nil, fmt.Errorf("providerstore: failed to search providers: %w", err)
	}
	return result, nil
}

func (s *Store) Add(ctx context.Context, info *account.ProviderInfo) error {
	now := time.Now()
	createdAt := info.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}

	tagsJSON, err := marshalJSON(info.Tags)
	if err != nil {
		return fmt.Errorf("providerstore: failed to marshal tags: %w", err)
	}
	modelsJSON, err := marshalJSON(info.SupportedModels)
	if err != nil {
		return fmt.Errorf("providerstore: failed to marshal supported_models: %w", err)
	}
	usageRulesJSON, err := marshalJSON(info.UsageRules)
	if err != nil {
		return fmt.Errorf("providerstore: failed to marshal usage_rules: %w", err)
	}
	metadataJSON, err := marshalJSON(info.Metadata)
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
		if isDuplicateEntry(err) {
			return storage.ErrAlreadyExists
		}
		return fmt.Errorf("providerstore: failed to add provider: %w", err)
	}
	return nil
}

func (s *Store) Update(ctx context.Context, info *account.ProviderInfo) error {
	tagsJSON, err := marshalJSON(info.Tags)
	if err != nil {
		return fmt.Errorf("providerstore: failed to marshal tags: %w", err)
	}
	modelsJSON, err := marshalJSON(info.SupportedModels)
	if err != nil {
		return fmt.Errorf("providerstore: failed to marshal supported_models: %w", err)
	}
	usageRulesJSON, err := marshalJSON(info.UsageRules)
	if err != nil {
		return fmt.Errorf("providerstore: failed to marshal usage_rules: %w", err)
	}
	metadataJSON, err := marshalJSON(info.Metadata)
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
