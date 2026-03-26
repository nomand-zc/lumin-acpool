package mysql

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/nomand-zc/lumin-acpool/storage"
)

//go:embed accounts.sql
var accountsTableSQL string

//go:embed providers.sql
var providersTableSQL string

//go:embed affinities.sql
var affinitiesTableSQL string

//go:embed account_occupancy.sql
var accountOccupancyTableSQL string

//go:embed account_stats.sql
var accountStatsTableSQL string

//go:embed tracked_usages.sql
var trackedUsagesTableSQL string

// 编译期接口合规性检查。
var (
	_ storage.AccountStorage  = (*Store)(nil)
	_ storage.ProviderStorage = (*Store)(nil)
	_ storage.StatsStore      = (*Store)(nil)
	_ storage.UsageStore      = (*Store)(nil)
	_ storage.OccupancyStore  = (*Store)(nil)
	_ storage.AffinityStore   = (*Store)(nil)
)

// Store 是基于 MySQL 的统一存储实现，实现所有 store 接口。
// 共享同一个 Client 连接，支持跨表事务操作。
type Store struct {
	client            Client
	accountConverter  *MysqlConverter
	providerConverter *MysqlConverter
}

// NewStore 创建一个新的 MySQL 统一存储实例。
// 通过 Options 传递 InstanceName 或 DSN 来创建 Client，
// 并在 SkipInitDB 为 false 时自动创建所有表。
func NewStore(opts ...Option) (*Store, error) {
	o := DefaultOptions()
	for _, opt := range opts {
		opt(o)
	}

	client, err := buildClient(o)
	if err != nil {
		return nil, fmt.Errorf("mysql store: %w", err)
	}

	// 定义 provider JSON 字段映射
	providerJSONFields := map[string]bool{
		storage.ProviderFieldSupportedModel: true,
	}

	store := &Store{
		client:            client,
		accountConverter:  NewConditionConverter(accountFieldMapping, nil),
		providerConverter: NewConditionConverter(providerFieldMapping, providerJSONFields),
	}

	if !o.SkipInitDB {
		if err := store.initDB(); err != nil {
			return nil, fmt.Errorf("mysql store: %w", err)
		}
	}

	return store, nil
}

// Close 关闭数据库连接。
func (s *Store) Close() error {
	return s.client.Close()
}

// initDB 执行所有建表 DDL。
func (s *Store) initDB() error {
	ctx := context.Background()

	tables := []struct {
		name string
		sql  string
	}{
		{"accounts", accountsTableSQL},
		{"providers", providersTableSQL},
		{"affinities", affinitiesTableSQL},
		{"account_occupancy", accountOccupancyTableSQL},
		{"account_stats", accountStatsTableSQL},
		{"tracked_usages", trackedUsagesTableSQL},
	}

	for _, t := range tables {
		if _, err := s.client.Exec(ctx, t.sql); err != nil {
			return fmt.Errorf("failed to init %s table: %w", t.name, err)
		}
	}

	return nil
}
