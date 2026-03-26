package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-client/usagerule"
)

const (
	// queryGetCurrentUsages 根据 account_id 查询当前窗口内的用量追踪数据。
	queryGetCurrentUsages = `SELECT rule_index, source_type, time_granularity, window_size, rule_total,
		local_used, remote_used, remote_remain, window_start, window_end, last_sync_at
		FROM tracked_usages WHERE account_id=? AND (window_end IS NULL OR window_end >= datetime('now'))
		ORDER BY rule_index ASC`

	queryDeleteUsages = `DELETE FROM tracked_usages WHERE account_id=?`

	queryInsertUsage = `INSERT INTO tracked_usages
		(account_id, rule_index, source_type, time_granularity, window_size, rule_total,
		local_used, remote_used, remote_remain, window_start, window_end, last_sync_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	queryIncrLocalUsed = `UPDATE tracked_usages SET local_used = local_used + ?
		WHERE account_id=? AND rule_index=?`

	queryCalibrateRule = `UPDATE tracked_usages SET
		remote_used=?, remote_remain=?, local_used=0,
		window_start=?, window_end=?, last_sync_at=?
		WHERE account_id=? AND rule_index=?`
)

func (s *Store) GetCurrentUsages(ctx context.Context, accountID string) ([]*account.TrackedUsage, error) {
	var result []*account.TrackedUsage
	err := s.client.Query(ctx, func(rows *sql.Rows) error {
		var (
			ruleIndex       int
			sourceType      int
			timeGranularity string
			windowSize      int
			ruleTotal       float64
			localUsed       float64
			remoteUsed      float64
			remoteRemain    float64
			windowStart     sql.NullString
			windowEnd       sql.NullString
			lastSyncAtStr   string
		)

		scanErr := rows.Scan(
			&ruleIndex, &sourceType, &timeGranularity, &windowSize, &ruleTotal,
			&localUsed, &remoteUsed, &remoteRemain,
			&windowStart, &windowEnd, &lastSyncAtStr,
		)
		if scanErr != nil {
			return fmt.Errorf("sqlite store: failed to scan usage: %w", scanErr)
		}

		lastSyncAt, _ := parseTime(lastSyncAtStr)

		usage := &account.TrackedUsage{
			Rule: &usagerule.UsageRule{
				SourceType:      usagerule.SourceType(sourceType),
				TimeGranularity: usagerule.TimeGranularity(timeGranularity),
				WindowSize:      windowSize,
				Total:           ruleTotal,
			},
			LocalUsed:    localUsed,
			RemoteUsed:   remoteUsed,
			RemoteRemain: remoteRemain,
			LastSyncAt:   lastSyncAt,
		}
		if windowStart.Valid && windowStart.String != "" {
			if t, err := parseTime(windowStart.String); err == nil {
				usage.WindowStart = &t
			}
		}
		if windowEnd.Valid && windowEnd.String != "" {
			if t, err := parseTime(windowEnd.String); err == nil {
				usage.WindowEnd = &t
			}
		}

		result = append(result, usage)
		return nil
	}, queryGetCurrentUsages, accountID)
	if err != nil {
		return nil, fmt.Errorf("sqlite store: failed to get usages: %w", err)
	}
	return result, nil
}

func (s *Store) SaveUsages(ctx context.Context, accountID string, usages []*account.TrackedUsage) error {
	return s.client.Transaction(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, queryDeleteUsages, accountID)
		if err != nil {
			return fmt.Errorf("sqlite store: failed to delete old usages: %w", err)
		}

		if len(usages) > 0 {
			stmt, err := tx.PrepareContext(ctx, queryInsertUsage)
			if err != nil {
				return fmt.Errorf("sqlite store: failed to prepare statement: %w", err)
			}
			defer stmt.Close()

			for i, u := range usages {
				var (
					sourceType      int
					timeGranularity string
					windowSize      int
					ruleTotal       float64
				)
				if u.Rule != nil {
					sourceType = int(u.Rule.SourceType)
					timeGranularity = string(u.Rule.TimeGranularity)
					windowSize = u.Rule.WindowSize
					ruleTotal = u.Rule.Total
				}

				var windowStart, windowEnd *string
				if u.WindowStart != nil {
					ws := u.WindowStart.Format("2006-01-02 15:04:05.000")
					windowStart = &ws
				}
				if u.WindowEnd != nil {
					we := u.WindowEnd.Format("2006-01-02 15:04:05.000")
					windowEnd = &we
				}

				_, err = stmt.ExecContext(ctx,
					accountID, i,
					sourceType, timeGranularity, windowSize, ruleTotal,
					u.LocalUsed, u.RemoteUsed, u.RemoteRemain,
					windowStart, windowEnd,
					u.LastSyncAt.Format("2006-01-02 15:04:05.000"),
				)
				if err != nil {
					return fmt.Errorf("sqlite store: failed to insert usage at index %d: %w", i, err)
				}
			}
		}

		return nil
	})
}

func (s *Store) IncrLocalUsed(ctx context.Context, accountID string, ruleIndex int, amount float64) error {
	result, err := s.client.Exec(ctx, queryIncrLocalUsed, amount, accountID, ruleIndex)
	if err != nil {
		return fmt.Errorf("sqlite store: failed to incr local used: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite store: failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return nil
	}
	return nil
}

func (s *Store) RemoveUsages(ctx context.Context, accountID string) error {
	_, err := s.client.Exec(ctx, queryDeleteUsages, accountID)
	if err != nil {
		return fmt.Errorf("sqlite store: failed to remove usages: %w", err)
	}
	return nil
}

func (s *Store) CalibrateRule(ctx context.Context, accountID string, ruleIndex int, usage *account.TrackedUsage) error {
	var windowStart, windowEnd *string
	if usage.WindowStart != nil {
		ws := usage.WindowStart.Format("2006-01-02 15:04:05.000")
		windowStart = &ws
	}
	if usage.WindowEnd != nil {
		we := usage.WindowEnd.Format("2006-01-02 15:04:05.000")
		windowEnd = &we
	}

	_, err := s.client.Exec(ctx, queryCalibrateRule,
		usage.RemoteUsed, usage.RemoteRemain,
		windowStart, windowEnd,
		time.Now().Format("2006-01-02 15:04:05.000"),
		accountID, ruleIndex,
	)
	if err != nil {
		return fmt.Errorf("sqlite store: failed to calibrate rule: %w", err)
	}
	return nil
}
