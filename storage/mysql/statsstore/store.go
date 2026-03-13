package statsstore

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	storeMysql "github.com/nomand-zc/lumin-acpool/storage/mysql"
)

//go:embed account_stats.sql
var accountStatsTableSQL string

// Compile-time interface compliance check.
var _ storage.StatsStore = (*Store)(nil)

// Store 是基于 MySQL 的 StatsStore 实现。
type Store struct {
	client storeMysql.Client
}

// NewStore 创建一个新的 MySQL 统计存储实例。
// 通过 Options 传递 InstanceName 或 DSN 来创建 Client，并在 SkipInitDB 为 false 时自动创建 account_stats 表。
func NewStore(opts ...Option) (*Store, error) {
	o := DefaultOptions()
	for _, opt := range opts {
		opt(o)
	}

	client, err := buildClient(o)
	if err != nil {
		return nil, fmt.Errorf("statsstore: %w", err)
	}

	store := &Store{client: client}

	if !o.SkipInitDB {
		if err := store.initDB(); err != nil {
			return nil, fmt.Errorf("statsstore: %w", err)
		}
	}

	return store, nil
}

// initDB 执行建表 DDL，初始化 account_stats 表。
func (s *Store) initDB() error {
	_, err := s.client.Exec(context.Background(), accountStatsTableSQL)
	if err != nil {
		return fmt.Errorf("failed to init account_stats table: %w", err)
	}
	return nil
}

func (s *Store) Get(ctx context.Context, accountID string) (*account.AccountStats, error) {
	var (
		stats       account.AccountStats
		lastUsedAt  sql.NullTime
		lastErrorAt sql.NullTime
		lastErrMsg  sql.NullString
	)

	dest := []any{
		&stats.AccountID, &stats.TotalCalls, &stats.SuccessCalls, &stats.FailedCalls,
		&stats.ConsecutiveFailures, &lastUsedAt, &lastErrorAt, &lastErrMsg,
	}

	err := s.client.QueryRow(ctx, dest, queryGetStats, accountID)
	if err != nil {
		if err == sql.ErrNoRows {
			// 不存在统计记录，返回零值
			return &account.AccountStats{AccountID: accountID}, nil
		}
		return nil, fmt.Errorf("statsstore: failed to get stats: %w", err)
	}

	if lastUsedAt.Valid {
		stats.LastUsedAt = &lastUsedAt.Time
	}
	if lastErrorAt.Valid {
		stats.LastErrorAt = &lastErrorAt.Time
	}
	if lastErrMsg.Valid {
		stats.LastErrorMsg = lastErrMsg.String
	}

	return &stats, nil
}

func (s *Store) IncrSuccess(ctx context.Context, accountID string) error {
	now := time.Now()
	_, err := s.client.Exec(ctx, queryIncrSuccess, accountID, now, now)
	if err != nil {
		return fmt.Errorf("statsstore: failed to incr success: %w", err)
	}
	return nil
}

func (s *Store) IncrFailure(ctx context.Context, accountID string, errMsg string) (int, error) {
	now := time.Now()
	result, err := s.client.Exec(ctx, queryIncrFailure, accountID, now, errMsg, now, errMsg)
	if err != nil {
		return 0, fmt.Errorf("statsstore: failed to incr failure: %w", err)
	}

	// 使用 LAST_INSERT_ID() 技巧获取递增后的值。
	// 当记录是新插入时（RowsAffected=1），consecutive_failures 初始值为 1。
	// 当记录是更新时（RowsAffected=2，ON DUPLICATE KEY UPDATE 的约定），
	// LastInsertId 返回 LAST_INSERT_ID(consecutive_failures + 1) 设置的值。
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 1 {
		// 新插入，consecutive_failures = 1
		return 1, nil
	}
	lastID, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("statsstore: failed to get last insert id: %w", err)
	}
	return int(lastID), nil
}

func (s *Store) UpdateLastUsed(ctx context.Context, accountID string, t time.Time) error {
	_, err := s.client.Exec(ctx, queryUpdateLastUsed, accountID, t, t)
	if err != nil {
		return fmt.Errorf("statsstore: failed to update last used: %w", err)
	}
	return nil
}

func (s *Store) GetConsecutiveFailures(ctx context.Context, accountID string) (int, error) {
	var failures int
	err := s.client.QueryRow(ctx, []any{&failures}, queryGetConsecutiveFailures, accountID)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, fmt.Errorf("statsstore: failed to get consecutive failures: %w", err)
	}
	return failures, nil
}

func (s *Store) ResetConsecutiveFailures(ctx context.Context, accountID string) error {
	_, err := s.client.Exec(ctx, queryResetConsecutiveFailures, accountID)
	if err != nil {
		return fmt.Errorf("statsstore: failed to reset consecutive failures: %w", err)
	}
	return nil
}

func (s *Store) Remove(ctx context.Context, accountID string) error {
	_, err := s.client.Exec(ctx, queryDeleteStats, accountID)
	if err != nil {
		return fmt.Errorf("statsstore: failed to remove stats: %w", err)
	}
	return nil
}
