package occupancystore

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"

	"github.com/nomand-zc/lumin-acpool/storage"
	storeMysql "github.com/nomand-zc/lumin-acpool/storage/mysql"
)

//go:embed account_occupancy.sql
var accountOccupancyTableSQL string

// 编译期接口合规性检查。
var _ storage.OccupancyStore = (*Store)(nil)

// Store 是基于 MySQL 的 OccupancyStore 实现。
//
// 数据结构设计：
//   - account_occupancy 表，account_id 为主键，count 存储当前并发占用计数
//
// 使用 INSERT ... ON DUPLICATE KEY UPDATE 的 upsert 模式实现原子递增，
// 配合 LAST_INSERT_ID() 技巧原子获取递增后的值。
// 适用于已有 MySQL 基础设施但不希望引入 Redis 的场景。
type Store struct {
	client storeMysql.Client
}

// NewStore 创建一个新的 MySQL 占用计数存储实例。
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
	result, err := s.client.Exec(ctx, queryIncr, accountID)
	if err != nil {
		return 0, fmt.Errorf("occupancystore: failed to incr: %w", err)
	}

	// 使用 LAST_INSERT_ID() 技巧获取递增后的值。
	// 当记录是新插入时（RowsAffected=1），count 初始值为 1。
	// 当记录是更新时（RowsAffected=2，ON DUPLICATE KEY UPDATE 的约定），
	// LastInsertId 返回 LAST_INSERT_ID(count + 1) 设置的值。
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 1 {
		// 新插入，count = 1
		return 1, nil
	}
	lastID, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("occupancystore: failed to get last insert id: %w", err)
	}
	return lastID, nil
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
