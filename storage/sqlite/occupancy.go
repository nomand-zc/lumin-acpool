package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

const (
	queryOccupancyIncr = `INSERT INTO account_occupancy (account_id, count)
		VALUES (?, 1)
		ON CONFLICT(account_id) DO UPDATE SET count = count + 1
		RETURNING count`
	queryOccupancyDecr = `UPDATE account_occupancy SET count = MAX(count - 1, 0) WHERE account_id=?`
	queryOccupancyGet  = `SELECT count FROM account_occupancy WHERE account_id=?`
)

func (s *Store) IncrOccupancy(ctx context.Context, accountID string) (int64, error) {
	var count int64
	err := s.client.QueryRow(ctx, []any{&count}, queryOccupancyIncr, accountID)
	if err != nil {
		return 0, fmt.Errorf("sqlite store: failed to incr occupancy: %w", err)
	}
	return count, nil
}

func (s *Store) DecrOccupancy(ctx context.Context, accountID string) error {
	_, err := s.client.Exec(ctx, queryOccupancyDecr, accountID)
	if err != nil {
		return fmt.Errorf("sqlite store: failed to decr occupancy: %w", err)
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
		return 0, fmt.Errorf("sqlite store: failed to get occupancy: %w", err)
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
		return nil, fmt.Errorf("sqlite store: failed to batch get occupancies: %w", err)
	}
	return result, nil
}
