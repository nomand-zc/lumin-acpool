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
	storeSqlite "github.com/nomand-zc/lumin-acpool/storage/sqlite"
)

//go:embed providers.sql
var providersTableSQL string

// Compile-time interface compliance check.
var _ storage.ProviderStorage = (*Store)(nil)

// Store 是基于 SQLite 的 ProviderStorage 实现。
type Store struct {
	client    storeSqlite.Client
	converter *storeSqlite.SqliteConverter
}

// NewStore 创建一个新的 SQLite 供应商存储实例。
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

	// 定义JSON字段映射（SQLite中存储为TEXT类型的JSON数组字段）
	jsonFields := map[string]bool{
		storage.ProviderFieldSupportedModel: true,
	}

	store := &Store{
		client:    client,
		converter: storeSqlite.NewConditionConverter(providerFieldMapping, jsonFields),
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

	dest := []any{
		&info.ProviderType, &info.ProviderName,
		&statusInt, &info.Priority, &info.Weight,
		&tagsJSON, &modelsJSON, &usageRulesJSON, &metadataJSON,
		&info.AccountCount, &info.AvailableAccountCount,
		&createdAtStr, &updatedAtStr,
	}

	err := s.client.QueryRow(ctx, dest, queryGetProvider, key.Type, key.Name)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, storage.ErrNotFound
		}
		return nil, fmt.Errorf("providerstore: failed to get provider: %w", err)
	}

	result, err := buildProviderInfo(&info, statusInt, tagsJSON, modelsJSON, usageRulesJSON, metadataJSON, createdAtStr, updatedAtStr)
	if err != nil {
		return nil, fmt.Errorf("providerstore: failed to build provider info: %w", err)
	}
	return result, nil
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
		info, scanErr := scanProviderFields(rows)
		if scanErr != nil {
			return fmt.Errorf("providerstore: failed to scan provider: %w", scanErr)
		}
		result = append(result, info)
		return nil
	}, query, args...)
	if err != nil {
		return nil, fmt.Errorf("providerstore: failed to search providers: %w", err)
	}
	return result, nil
}

// buildProviderWhereClause 根据 SearchFilter 一级字段和 ExtraCond 构建 WHERE 子句。
func buildProviderWhereClause(filter *storage.SearchFilter, condResult *storeSqlite.CondConvertResult) string {
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
		if filter.SupportedModel != "" {
			parts = append(parts, `EXISTS(SELECT 1 FROM json_each(CAST("supported_models" AS TEXT)) WHERE json_each.value = ?)`)
		}
	}
	parts = append(parts, condResult.Cond)
	return strings.Join(parts, " AND ")
}

// buildProviderWhereArgs 根据 SearchFilter 一级字段和 ExtraCond 构建查询参数。
func buildProviderWhereArgs(filter *storage.SearchFilter, condResult *storeSqlite.CondConvertResult) []any {
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
		if filter.SupportedModel != "" {
			args = append(args, filter.SupportedModel)
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

	tagsJSON, err := storeSqlite.MarshalJSON(info.Tags)
	if err != nil {
		return fmt.Errorf("providerstore: failed to marshal tags: %w", err)
	}
	modelsJSON, err := storeSqlite.MarshalJSON(info.SupportedModels)
	if err != nil {
		return fmt.Errorf("providerstore: failed to marshal supported_models: %w", err)
	}
	usageRulesJSON, err := storeSqlite.MarshalJSON(info.UsageRules)
	if err != nil {
		return fmt.Errorf("providerstore: failed to marshal usage_rules: %w", err)
	}
	metadataJSON, err := storeSqlite.MarshalJSON(info.Metadata)
	if err != nil {
		return fmt.Errorf("providerstore: failed to marshal metadata: %w", err)
	}

	_, err = s.client.Exec(ctx, queryInsertProvider,
		info.ProviderType, info.ProviderName,
		int(info.Status), info.Priority, info.Weight,
		tagsJSON, modelsJSON, usageRulesJSON, metadataJSON,
		info.AccountCount, info.AvailableAccountCount,
		createdAt.Format("2006-01-02 15:04:05.000"),
		now.Format("2006-01-02 15:04:05.000"),
	)
	if err != nil {
		if storeSqlite.IsDuplicateEntry(err) {
			return storage.ErrAlreadyExists
		}
		return fmt.Errorf("providerstore: failed to add provider: %w", err)
	}
	return nil
}

func (s *Store) Update(ctx context.Context, info *account.ProviderInfo) error {
	tagsJSON, err := storeSqlite.MarshalJSON(info.Tags)
	if err != nil {
		return fmt.Errorf("providerstore: failed to marshal tags: %w", err)
	}
	modelsJSON, err := storeSqlite.MarshalJSON(info.SupportedModels)
	if err != nil {
		return fmt.Errorf("providerstore: failed to marshal supported_models: %w", err)
	}
	usageRulesJSON, err := storeSqlite.MarshalJSON(info.UsageRules)
	if err != nil {
		return fmt.Errorf("providerstore: failed to marshal usage_rules: %w", err)
	}
	metadataJSON, err := storeSqlite.MarshalJSON(info.Metadata)
	if err != nil {
		return fmt.Errorf("providerstore: failed to marshal metadata: %w", err)
	}

	result, err := s.client.Exec(ctx, queryUpdateProvider,
		int(info.Status), info.Priority, info.Weight,
		tagsJSON, modelsJSON, usageRulesJSON, metadataJSON,
		info.AccountCount, info.AvailableAccountCount,
		time.Now().Format("2006-01-02 15:04:05.000"),
		info.ProviderType, info.ProviderName,
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
