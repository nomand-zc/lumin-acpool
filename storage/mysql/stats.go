package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
)

const (
	// queryGetStats 根据 account_id 查询统计数据。
	queryGetStats = `SELECT account_id, total_calls, success_calls, failed_calls,
		consecutive_failures, last_used_at, last_error_at, last_error_msg
		FROM account_stats WHERE account_id=?`

	// queryIncrSuccess 增加成功调用计数（upsert 语义）。
	queryIncrSuccess = `INSERT INTO account_stats (account_id, total_calls, success_calls, consecutive_failures, last_used_at)
		VALUES (?, 1, 1, 0, ?)
		ON DUPLICATE KEY UPDATE
			total_calls = total_calls + 1,
			success_calls = success_calls + 1,
			consecutive_failures = 0,
			last_used_at = ?`

	// queryIncrFailure 增加失败调用计数（upsert 语义）。
	// 使用 LAST_INSERT_ID() 技巧，使递增后的 consecutive_failures 可通过 Result.LastInsertId() 原子获取。
	queryIncrFailure = `INSERT INTO account_stats (account_id, total_calls, failed_calls, consecutive_failures, last_error_at, last_error_msg)
		VALUES (?, 1, 1, 1, ?, ?)
		ON DUPLICATE KEY UPDATE
			total_calls = total_calls + 1,
			failed_calls = failed_calls + 1,
			consecutive_failures = LAST_INSERT_ID(consecutive_failures + 1),
			last_error_at = ?,
			last_error_msg = ?`

	// queryUpdateLastUsed 更新最后使用时间（upsert 语义）。
	queryUpdateLastUsed = `INSERT INTO account_stats (account_id, last_used_at)
		VALUES (?, ?)
		ON DUPLICATE KEY UPDATE last_used_at = ?`

	// queryGetConsecutiveFailures 查询连续失败次数。
	queryGetConsecutiveFailures = `SELECT consecutive_failures FROM account_stats WHERE account_id=?`

	// queryResetConsecutiveFailures 重置连续失败次数。
	queryResetConsecutiveFailures = `UPDATE account_stats SET consecutive_failures = 0 WHERE account_id=?`

	// queryDeleteStats 根据 account_id 删除统计记录。
	queryDeleteStats = `DELETE FROM account_stats WHERE account_id=?`
)

func (s *Store) GetStats(ctx context.Context, accountID string) (*account.AccountStats, error) {
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
		return nil, fmt.Errorf("mysql store: failed to get stats: %w", err)
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
		return fmt.Errorf("mysql store: failed to incr success: %w", err)
	}
	return nil
}

func (s *Store) IncrFailure(ctx context.Context, accountID string, errMsg string) (int, error) {
	now := time.Now()
	result, err := s.client.Exec(ctx, queryIncrFailure, accountID, now, errMsg, now, errMsg)
	if err != nil {
		return 0, fmt.Errorf("mysql store: failed to incr failure: %w", err)
	}

	// 使用 LAST_INSERT_ID() 技巧获取递增后的值。
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 1 {
		return 1, nil
	}
	lastID, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("mysql store: failed to get last insert id: %w", err)
	}
	return int(lastID), nil
}

func (s *Store) UpdateLastUsed(ctx context.Context, accountID string, t time.Time) error {
	_, err := s.client.Exec(ctx, queryUpdateLastUsed, accountID, t, t)
	if err != nil {
		return fmt.Errorf("mysql store: failed to update last used: %w", err)
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
		return 0, fmt.Errorf("mysql store: failed to get consecutive failures: %w", err)
	}
	return failures, nil
}

func (s *Store) ResetConsecutiveFailures(ctx context.Context, accountID string) error {
	_, err := s.client.Exec(ctx, queryResetConsecutiveFailures, accountID)
	if err != nil {
		return fmt.Errorf("mysql store: failed to reset consecutive failures: %w", err)
	}
	return nil
}

func (s *Store) RemoveStats(ctx context.Context, accountID string) error {
	_, err := s.client.Exec(ctx, queryDeleteStats, accountID)
	if err != nil {
		return fmt.Errorf("mysql store: failed to remove stats: %w", err)
	}
	return nil
}
