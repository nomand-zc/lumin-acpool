package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
)

const (
	queryGetStats = `SELECT account_id, total_calls, success_calls, failed_calls,
		consecutive_failures, last_used_at, last_error_at, last_error_msg
		FROM account_stats WHERE account_id=?`

	queryIncrSuccess = `INSERT INTO account_stats (account_id, total_calls, success_calls, consecutive_failures, last_used_at)
		VALUES (?, 1, 1, 0, ?)
		ON CONFLICT(account_id) DO UPDATE SET
			total_calls = total_calls + 1,
			success_calls = success_calls + 1,
			consecutive_failures = 0,
			last_used_at = ?`

	queryIncrFailure = `INSERT INTO account_stats (account_id, total_calls, failed_calls, consecutive_failures, last_error_at, last_error_msg)
		VALUES (?, 1, 1, 1, ?, ?)
		ON CONFLICT(account_id) DO UPDATE SET
			total_calls = total_calls + 1,
			failed_calls = failed_calls + 1,
			consecutive_failures = consecutive_failures + 1,
			last_error_at = ?,
			last_error_msg = ?`

	queryGetConsecutiveFailuresAfterIncr = `SELECT consecutive_failures FROM account_stats WHERE account_id=?`

	queryUpdateLastUsed = `INSERT INTO account_stats (account_id, last_used_at)
		VALUES (?, ?)
		ON CONFLICT(account_id) DO UPDATE SET last_used_at = ?`

	queryGetConsecutiveFailures   = `SELECT consecutive_failures FROM account_stats WHERE account_id=?`
	queryResetConsecutiveFailures = `UPDATE account_stats SET consecutive_failures = 0 WHERE account_id=?`
	queryDeleteStats              = `DELETE FROM account_stats WHERE account_id=?`
)

func (s *Store) GetStats(ctx context.Context, accountID string) (*account.AccountStats, error) {
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
			return &account.AccountStats{AccountID: accountID}, nil
		}
		return nil, fmt.Errorf("sqlite store: failed to get stats: %w", err)
	}

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
		return fmt.Errorf("sqlite store: failed to incr success: %w", err)
	}
	return nil
}

func (s *Store) IncrFailure(ctx context.Context, accountID string, errMsg string) (int, error) {
	now := time.Now().Format("2006-01-02 15:04:05.000")

	_, err := s.client.Exec(ctx, queryIncrFailure, accountID, now, errMsg, now, errMsg)
	if err != nil {
		return 0, fmt.Errorf("sqlite store: failed to incr failure: %w", err)
	}

	var failures int
	err = s.client.QueryRow(ctx, []any{&failures}, queryGetConsecutiveFailuresAfterIncr, accountID)
	if err != nil {
		return 0, fmt.Errorf("sqlite store: failed to get consecutive failures after incr: %w", err)
	}
	return failures, nil
}

func (s *Store) UpdateLastUsed(ctx context.Context, accountID string, t time.Time) error {
	timeStr := t.Format("2006-01-02 15:04:05.000")
	_, err := s.client.Exec(ctx, queryUpdateLastUsed, accountID, timeStr, timeStr)
	if err != nil {
		return fmt.Errorf("sqlite store: failed to update last used: %w", err)
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
		return 0, fmt.Errorf("sqlite store: failed to get consecutive failures: %w", err)
	}
	return failures, nil
}

func (s *Store) ResetConsecutiveFailures(ctx context.Context, accountID string) error {
	_, err := s.client.Exec(ctx, queryResetConsecutiveFailures, accountID)
	if err != nil {
		return fmt.Errorf("sqlite store: failed to reset consecutive failures: %w", err)
	}
	return nil
}

func (s *Store) RemoveStats(ctx context.Context, accountID string) error {
	_, err := s.client.Exec(ctx, queryDeleteStats, accountID)
	if err != nil {
		return fmt.Errorf("sqlite store: failed to remove stats: %w", err)
	}
	return nil
}
