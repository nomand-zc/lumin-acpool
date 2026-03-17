package mysql

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
		FROM tracked_usages WHERE account_id=? AND (window_end IS NULL OR window_end >= NOW()) 
		ORDER BY rule_index ASC`

	// queryDeleteUsages 根据 account_id 删除用量追踪数据。
	queryDeleteUsages = `DELETE FROM tracked_usages WHERE account_id=?`

	// queryInsertUsage 插入单条用量追踪数据。
	queryInsertUsage = `INSERT INTO tracked_usages 
		(account_id, rule_index, source_type, time_granularity, window_size, rule_total, 
		local_used, remote_used, remote_remain, window_start, window_end, last_sync_at) 
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	// queryIncrLocalUsed 增加本地用量。
	queryIncrLocalUsed = `UPDATE tracked_usages SET local_used = local_used + ? 
		WHERE account_id=? AND rule_index=?`

	// queryCalibrateRule 原子校准指定规则的远端数据并重置本地计数。
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
			windowStart     sql.NullTime
			windowEnd       sql.NullTime
			lastSyncAt      time.Time
		)

		scanErr := rows.Scan(
			&ruleIndex, &sourceType, &timeGranularity, &windowSize, &ruleTotal,
			&localUsed, &remoteUsed, &remoteRemain,
			&windowStart, &windowEnd, &lastSyncAt,
		)
		if scanErr != nil {
			return fmt.Errorf("mysql store: failed to scan usage: %w", scanErr)
		}

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
		if windowStart.Valid {
			usage.WindowStart = &windowStart.Time
		}
		if windowEnd.Valid {
			usage.WindowEnd = &windowEnd.Time
		}

		result = append(result, usage)
		return nil
	}, queryGetCurrentUsages, accountID)
	if err != nil {
		return nil, fmt.Errorf("mysql store: failed to get usages: %w", err)
	}
	return result, nil
}

func (s *Store) SaveUsages(ctx context.Context, accountID string, usages []*account.TrackedUsage) error {
	return s.client.Transaction(ctx, func(tx *sql.Tx) error {
		// 先删除该账号的所有追踪数据
		_, err := tx.ExecContext(ctx, queryDeleteUsages, accountID)
		if err != nil {
			return fmt.Errorf("mysql store: failed to delete old usages: %w", err)
		}

		// 批量插入新数据
		if len(usages) > 0 {
			stmt, err := tx.PrepareContext(ctx, queryInsertUsage)
			if err != nil {
				return fmt.Errorf("mysql store: failed to prepare statement: %w", err)
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

				_, err = stmt.ExecContext(ctx,
					accountID, i,
					sourceType, timeGranularity, windowSize, ruleTotal,
					u.LocalUsed, u.RemoteUsed, u.RemoteRemain,
					u.WindowStart, u.WindowEnd,
					u.LastSyncAt,
				)
				if err != nil {
					return fmt.Errorf("mysql store: failed to insert usage at index %d: %w", i, err)
				}
			}
		}

		return nil
	})
}

func (s *Store) IncrLocalUsed(ctx context.Context, accountID string, ruleIndex int, amount float64) error {
	result, err := s.client.Exec(ctx, queryIncrLocalUsed, amount, accountID, ruleIndex)
	if err != nil {
		return fmt.Errorf("mysql store: failed to incr local used: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("mysql store: failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		// 未初始化，静默忽略（与内存实现保持一致）
		return nil
	}
	return nil
}

func (s *Store) RemoveUsages(ctx context.Context, accountID string) error {
	_, err := s.client.Exec(ctx, queryDeleteUsages, accountID)
	if err != nil {
		return fmt.Errorf("mysql store: failed to remove usages: %w", err)
	}
	return nil
}

func (s *Store) CalibrateRule(ctx context.Context, accountID string, ruleIndex int, usage *account.TrackedUsage) error {
	_, err := s.client.Exec(ctx, queryCalibrateRule,
		usage.RemoteUsed, usage.RemoteRemain,
		usage.WindowStart, usage.WindowEnd, time.Now(),
		accountID, ruleIndex,
	)
	if err != nil {
		return fmt.Errorf("mysql store: failed to calibrate rule: %w", err)
	}
	return nil
}
