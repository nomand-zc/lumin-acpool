package usagestore

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	storeSqlite "github.com/nomand-zc/lumin-acpool/storage/sqlite"
	"github.com/nomand-zc/lumin-client/usagerule"
)

//go:embed tracked_usages.sql
var trackedUsagesTableSQL string

// Compile-time interface compliance check.
var _ storage.UsageStore = (*Store)(nil)

// Store 是基于 SQLite 的 UsageStore 实现。
type Store struct {
	client storeSqlite.Client
}

// NewStore 创建一个新的 SQLite 用量存储实例。
// 通过 Options 传递 InstanceName 或 DSN 来创建 Client，并在 SkipInitDB 为 false 时自动创建 tracked_usages 表。
func NewStore(opts ...Option) (*Store, error) {
	o := DefaultOptions()
	for _, opt := range opts {
		opt(o)
	}

	client, err := buildClient(o)
	if err != nil {
		return nil, fmt.Errorf("usagestore: %w", err)
	}

	store := &Store{client: client}

	if !o.SkipInitDB {
		if err := store.initDB(); err != nil {
			return nil, fmt.Errorf("usagestore: %w", err)
		}
	}

	return store, nil
}

// initDB 执行建表 DDL，初始化 tracked_usages 表。
func (s *Store) initDB() error {
	_, err := s.client.Exec(context.Background(), trackedUsagesTableSQL)
	if err != nil {
		return fmt.Errorf("failed to init tracked_usages table: %w", err)
	}
	return nil
}

func (s *Store) GetAllUsages(ctx context.Context, accountID string) ([]*account.TrackedUsage, error) {
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
			return fmt.Errorf("usagestore: failed to scan usage: %w", scanErr)
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
	}, queryGetAllUsages, accountID)
	if err != nil {
		return nil, fmt.Errorf("usagestore: failed to get usages: %w", err)
	}
	return result, nil
}

func (s *Store) SaveUsages(ctx context.Context, accountID string, usages []*account.TrackedUsage) error {
	return s.client.Transaction(ctx, func(tx *sql.Tx) error {
		// 先删除该账号的所有追踪数据
		_, err := tx.ExecContext(ctx, queryDeleteUsages, accountID)
		if err != nil {
			return fmt.Errorf("usagestore: failed to delete old usages: %w", err)
		}

		// 批量插入新数据
		if len(usages) > 0 {
			stmt, err := tx.PrepareContext(ctx, queryInsertUsage)
			if err != nil {
				return fmt.Errorf("usagestore: failed to prepare statement: %w", err)
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

				// 时间转为字符串
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
					return fmt.Errorf("usagestore: failed to insert usage at index %d: %w", i, err)
				}
			}
		}

		return nil
	})
}

func (s *Store) IncrLocalUsed(ctx context.Context, accountID string, ruleIndex int, amount float64) error {
	result, err := s.client.Exec(ctx, queryIncrLocalUsed, amount, accountID, ruleIndex)
	if err != nil {
		return fmt.Errorf("usagestore: failed to incr local used: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("usagestore: failed to get rows affected: %w", err)
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
		return fmt.Errorf("usagestore: failed to remove usages: %w", err)
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
		return fmt.Errorf("usagestore: failed to calibrate rule: %w", err)
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
