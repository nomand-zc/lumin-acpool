package sqlite

import (
	"context"
	"database/sql"
	"fmt"
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
