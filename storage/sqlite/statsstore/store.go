package statsstore

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	storeSqlite "github.com/nomand-zc/lumin-acpool/storage/sqlite"
)

//go:embed account_stats.sql
var accountStatsTableSQL string

// Compile-time interface compliance check.
var _ storage.StatsStore = (*Store)(nil)

// Store 是基于 SQLite 的 StatsStore 实现。
type Store struct {
	client storeSqlite.Client
}

// NewStore 创建一个新的 SQLite 统计存储实例。
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
		lastUsedAt  sql.NullString
		lastErrorAt sql.NullString
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

	// 解析时间字段（SQLite 存储为 TEXT）
	if lastUsedAt.Valid && lastUsedAt.String != "" {
		if t, err := parseTime(lastUsedAt.String); err == nil {
			stats.LastUsedAt = &t
		}
	}
	if lastErrorAt.Valid && lastErrorAt.String != "" {
		if t, err := parseTime(lastErrorAt.String); err == nil {
			stats.LastErrorAt = &t
		}
	}
	if lastErrMsg.Valid {
		stats.LastErrorMsg = lastErrMsg.String
	}

	return &stats, nil
}

func (s *Store) IncrSuccess(ctx context.Context, accountID string) error {
	now := time.Now().Format("2006-01-02 15:04:05.000")
	_, err := s.client.Exec(ctx, queryIncrSuccess, accountID, now, now)
	if err != nil {
		return fmt.Errorf("statsstore: failed to incr success: %w", err)
	}
	return nil
}

func (s *Store) IncrFailure(ctx context.Context, accountID string, errMsg string) (int, error) {
	now := time.Now().Format("2006-01-02 15:04:05.000")

	// SQLite 不支持 MySQL 的 LAST_INSERT_ID() 技巧，
	// 因此在 upsert 后再查询最新的 consecutive_failures 值。
	// SQLite 是嵌入式数据库，单进程内串行写入，不存在并发写入竞态。
	_, err := s.client.Exec(ctx, queryIncrFailure, accountID, now, errMsg, now, errMsg)
	if err != nil {
		return 0, fmt.Errorf("statsstore: failed to incr failure: %w", err)
	}

	// 查询最新的连续失败次数
	var failures int
	err = s.client.QueryRow(ctx, []any{&failures}, queryGetConsecutiveFailuresAfterIncr, accountID)
	if err != nil {
		return 0, fmt.Errorf("statsstore: failed to get consecutive failures after incr: %w", err)
	}
	return failures, nil
}

func (s *Store) UpdateLastUsed(ctx context.Context, accountID string, t time.Time) error {
	timeStr := t.Format("2006-01-02 15:04:05.000")
	_, err := s.client.Exec(ctx, queryUpdateLastUsed, accountID, timeStr, timeStr)
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

// parseTime 解析 SQLite 中存储的时间字符串。
func parseTime(s string) (time.Time, error) {
	formats := []string{
		"2006-01-02 15:04:05.000",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05Z",
		time.RFC3339,
		time.RFC3339Nano,
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported time format: %s", s)
}
