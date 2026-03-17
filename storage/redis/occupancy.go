package redis

import (
	"context"
	_ "embed"
	"fmt"
)

const (
	occupancyKeyPrefix = "occupancy:"
)

func occupancyRedisKey(prefix, accountID string) string {
	return prefix + occupancyKeyPrefix + accountID
}

var (
	//go:embed scripts/occupancy_incr.lua
	occupancyLuaIncr string

	//go:embed scripts/occupancy_decr.lua
	occupancyLuaDecr string

	//go:embed scripts/occupancy_get.lua
	occupancyLuaGet string

	//go:embed scripts/occupancy_batch_get.lua
	occupancyLuaBatchGet string
)

func (s *Store) IncrOccupancy(ctx context.Context, accountID string) (int64, error) {
	key := occupancyRedisKey(s.keyPrefix, accountID)
	result, err := s.client.Eval(ctx, occupancyLuaIncr, []string{key})
	if err != nil {
		return 0, fmt.Errorf("redis store: failed to incr occupancy: %w", err)
	}
	newVal, ok := result.(int64)
	if !ok {
		return 0, fmt.Errorf("redis store: unexpected incr result type: %T", result)
	}
	return newVal, nil
}

func (s *Store) DecrOccupancy(ctx context.Context, accountID string) error {
	key := occupancyRedisKey(s.keyPrefix, accountID)
	_, err := s.client.Eval(ctx, occupancyLuaDecr, []string{key})
	if err != nil {
		return fmt.Errorf("redis store: failed to decr occupancy: %w", err)
	}
	return nil
}

func (s *Store) GetOccupancy(ctx context.Context, accountID string) (int64, error) {
	key := occupancyRedisKey(s.keyPrefix, accountID)
	result, err := s.client.Eval(ctx, occupancyLuaGet, []string{key})
	if err != nil {
		return 0, fmt.Errorf("redis store: failed to get occupancy: %w", err)
	}
	val, ok := result.(int64)
	if !ok {
		return 0, fmt.Errorf("redis store: unexpected get result type: %T", result)
	}
	return val, nil
}

func (s *Store) GetOccupancies(ctx context.Context, accountIDs []string) (map[string]int64, error) {
	if len(accountIDs) == 0 {
		return make(map[string]int64), nil
	}

	keys := make([]string, len(accountIDs))
	for i, id := range accountIDs {
		keys[i] = occupancyRedisKey(s.keyPrefix, id)
	}

	result, err := s.client.Eval(ctx, occupancyLuaBatchGet, keys)
	if err != nil {
		return nil, fmt.Errorf("redis store: failed to batch get occupancies: %w", err)
	}

	vals, ok := result.([]interface{})
	if !ok {
		return nil, fmt.Errorf("redis store: unexpected batch get result type: %T", result)
	}

	occupancies := make(map[string]int64, len(accountIDs))
	for i, v := range vals {
		if i >= len(accountIDs) {
			break
		}
		if val, ok := v.(int64); ok && val > 0 {
			occupancies[accountIDs[i]] = val
		}
	}
	return occupancies, nil
}
