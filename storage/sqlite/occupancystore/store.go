package occupancystore

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"

	"github.com/nomand-zc/lumin-acpool/storage"
	storeSqlite "github.com/nomand-zc/lumin-acpool/storage/sqlite"
)

//go:embed account_occupancy.sql
var accountOccupancyTableSQL string

// 编译期接口合规性检查。
var _ storage.OccupancyStore = (*Store)(nil)

// Store 是基于 SQLite 的 OccupancyStore 实现。
//
// 数据结构设计：
//   - account_occupancy 表，account_id 为主键，count 存储当前并发占用计数
//
// 使用 INSERT ... ON CONFLICT ... DO UPDATE 的 upsert 模式实现原子递增，
// 配合 RETURNING 子句原子获取递增后的值。
// SQLite 是嵌入式数据库，适用于单机部署场景。
type Store struct {
	client storeSqlite.Client
}

// NewStore 创建一个新的 SQLite 占用计数存储实例。
// 通过 Options 传递 InstanceName 或 DSN 来创建 Client，并在 SkipInitDB 为 false 时自动创建 account_occupancy 表。
func NewStore(opts ...Option) (*Store, error) {
	o := DefaultOptions()
	for _, opt := range opts {
		opt(o)
	}

	client, err := buildClient(o)
	if err != nil {
		return nil, fmt.Errorf("occupancystore: %w", err)
	}

	store := &Store{client: client}

	if !o.SkipInitDB {
		if err := store.initDB(); err != nil {
			return nil, fmt.Errorf("occupancystore: %w", err)
		}
	}

	return store, nil
}

// initDB 执行建表 DDL，初始化 account_occupancy 表。
func (s *Store) initDB() error {
	_, err := s.client.Exec(context.Background(), accountOccupancyTableSQL)
	if err != nil {
		return fmt.Errorf("failed to init account_occupancy table: %w", err)
	}
	return nil
}

func (s *Store) IncrOccupancy(ctx context.Context, accountID string) (int64, error) {
	// 使用 RETURNING 子句原子获取递增后的值。
	var count int64
	err := s.client.QueryRow(ctx, []any{&count}, queryIncr, accountID)
	if err != nil {
		return 0, fmt.Errorf("occupancystore: failed to incr: %w", err)
	}
	return count, nil
}

func (s *Store) DecrOccupancy(ctx context.Context, accountID string) error {
	_, err := s.client.Exec(ctx, queryDecr, accountID)
	if err != nil {
		return fmt.Errorf("occupancystore: failed to decr: %w", err)
	}
	return nil
}

func (s *Store) GetOccupancy(ctx context.Context, accountID string) (int64, error) {
	var count int64
	err := s.client.QueryRow(ctx, []any{&count}, queryGet, accountID)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, fmt.Errorf("occupancystore: failed to get: %w", err)
	}
	return count, nil
}
