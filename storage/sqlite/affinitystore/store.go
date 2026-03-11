package affinitystore

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/nomand-zc/lumin-acpool/storage"
	storeSqlite "github.com/nomand-zc/lumin-acpool/storage/sqlite"
)

//go:embed affinities.sql
var affinitiesTableSQL string

// Compile-time interface compliance check.
var _ storage.AffinityStore = (*Store)(nil)

// Store 是基于 SQLite 的 AffinityStore 实现。
// 适用于单机部署或需要持久化绑定关系的场景。
type Store struct {
	client storeSqlite.Client
}

// NewStore 创建一个新的 SQLite 亲和存储实例。
// 通过 Options 传递 InstanceName 或 DSN 来创建 Client，并在 SkipInitDB 为 false 时自动创建 affinities 表。
func NewStore(opts ...Option) (*Store, error) {
	o := DefaultOptions()
	for _, opt := range opts {
		opt(o)
	}

	client, err := buildClient(o)
	if err != nil {
		return nil, fmt.Errorf("affinitystore: %w", err)
	}

	store := &Store{client: client}

	if !o.SkipInitDB {
		if err := store.initDB(); err != nil {
			return nil, fmt.Errorf("affinitystore: %w", err)
		}
	}

	return store, nil
}

// initDB 执行建表 DDL，初始化 affinities 表。
func (s *Store) initDB() error {
	_, err := s.client.Exec(context.Background(), affinitiesTableSQL)
	if err != nil {
		return fmt.Errorf("failed to init affinities table: %w", err)
	}
	return nil
}

// Get 获取亲和键对应的绑定目标 ID。
func (s *Store) Get(affinityKey string) (string, bool) {
	var targetID string
	err := s.client.QueryRow(context.Background(), []any{&targetID},
		queryGetAffinity, affinityKey)
	if err != nil {
		return "", false
	}
	return targetID, true
}

// Set 设置亲和键到目标 ID 的绑定关系。
// 使用 INSERT ... ON CONFLICT ... DO UPDATE 实现 upsert 语义。
func (s *Store) Set(affinityKey string, targetID string) {
	_, err := s.client.Exec(context.Background(), queryUpsertAffinity, affinityKey, targetID, targetID)
	if err != nil {
		// AffinityStore 接口不返回 error，记录错误后静默处理
		fmt.Printf("affinitystore: failed to set affinity: %v\n", err)
	}
}
