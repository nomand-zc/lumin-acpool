package usagestore

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	storeMysql "github.com/nomand-zc/lumin-acpool/storage/mysql"
	"github.com/nomand-zc/lumin-client/usagerule"
)

//go:embed tracked_usages.sql
var trackedUsagesTableSQL string

// Compile-time interface compliance check.
var _ storage.UsageStore = (*Store)(nil)

// Store 是基于 MySQL 的 UsageStore 实现。
type Store struct {
	client storeMysql.Client
}

// NewStore 创建一个新的 MySQL 用量存储实例。
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

func (s *Store) GetAll(ctx context.Context, accountID string) ([]*account.TrackedUsage, error) {
	var result []*account.TrackedUsage
	err := s.client.Query(ctx, func(rows *sql.Rows) error {
		for rows.Next() {
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
				return fmt.Errorf("usagestore: failed to scan usage: %w", scanErr)
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
		}
		return nil
	}, queryGetAllUsages, accountID)
	if err != nil {
		return nil, fmt.Errorf("usagestore: failed to get usages: %w", err)
	}
	return result, nil
}

func (s *Store) Save(ctx context.Context, accountID string, usages []*account.TrackedUsage) error {
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

				_, err = stmt.ExecContext(ctx,
					accountID, i,
					sourceType, timeGranularity, windowSize, ruleTotal,
				u.LocalUsed, u.RemoteUsed, u.RemoteRemain,
				u.WindowStart, u.WindowEnd,
				u.LastSyncAt,
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

func (s *Store) Remove(ctx context.Context, accountID string) error {
	_, err := s.client.Exec(ctx, queryDeleteUsages, accountID)
	if err != nil {
		return fmt.Errorf("usagestore: failed to remove usages: %w", err)
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
		return fmt.Errorf("usagestore: failed to calibrate rule: %w", err)
	}
	return nil
}
