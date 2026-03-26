package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

const (
	// queryOccupancyIncr 原子递增占用计数（upsert 语义）。
	// 使用 LAST_INSERT_ID() 技巧，使递增后的值可通过 Result.LastInsertId() 原子获取。
	queryOccupancyIncr = `INSERT INTO account_occupancy (account_id, count)
		VALUES (?, 1)
		ON DUPLICATE KEY UPDATE count = LAST_INSERT_ID(count + 1)`

	// queryOccupancyDecr 原子递减占用计数，保证不低于 0。
	queryOccupancyDecr = `UPDATE account_occupancy SET count = GREATEST(count - 1, 0) WHERE account_id=?`

	// queryOccupancyGet 查询当前占用计数。
	queryOccupancyGet = `SELECT count FROM account_occupancy WHERE account_id=?`
)

func (s *Store) IncrOccupancy(ctx context.Context, accountID string) (int64, error) {
	result, err := s.client.Exec(ctx, queryOccupancyIncr, accountID)
	if err != nil {
		return 0, fmt.Errorf("mysql store: failed to incr occupancy: %w", err)
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
		return 0, fmt.Errorf("mysql store: failed to get last insert id: %w", err)
	}
	return lastID, nil
}

func (s *Store) DecrOccupancy(ctx context.Context, accountID string) error {
	_, err := s.client.Exec(ctx, queryOccupancyDecr, accountID)
	if err != nil {
		return fmt.Errorf("mysql store: failed to decr occupancy: %w", err)
	}
	return nil
}

func (s *Store) GetOccupancy(ctx context.Context, accountID string) (int64, error) {
	var count int64
	err := s.client.QueryRow(ctx, []any{&count}, queryOccupancyGet, accountID)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, fmt.Errorf("mysql store: failed to get occupancy: %w", err)
	}
	return count, nil
}

func (s *Store) GetOccupancies(ctx context.Context, accountIDs []string) (map[string]int64, error) {
	if len(accountIDs) == 0 {
		return make(map[string]int64), nil
	}

	// 构建 IN 子句的占位符
	placeholders := make([]string, len(accountIDs))
	args := make([]any, len(accountIDs))
	for i, id := range accountIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := `SELECT account_id, count FROM account_occupancy WHERE account_id IN (` + strings.Join(placeholders, ",") + `)`

	result := make(map[string]int64, len(accountIDs))
	err := s.client.Query(ctx, func(rows *sql.Rows) error {
		var accountID string
		var count int64
		if err := rows.Scan(&accountID, &count); err != nil {
			return err
		}
		result[accountID] = count
		return nil
	}, query, args...)
	if err != nil {
		return nil, fmt.Errorf("mysql store: failed to batch get occupancies: %w", err)
	}
	return result, nil
}
