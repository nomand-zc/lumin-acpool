package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
)

const (
	statsKeyPrefix = "stats:"
)

// Stats Hash 字段名常量。
const (
	statsFieldAccountID           = "account_id"
	statsFieldTotalCalls          = "total_calls"
	statsFieldSuccessCalls        = "success_calls"
	statsFieldFailedCalls         = "failed_calls"
	statsFieldConsecutiveFailures = "consecutive_failures"
	statsFieldLastUsedAt          = "last_used_at"
	statsFieldLastErrorAt         = "last_error_at"
	statsFieldLastErrorMsg        = "last_error_msg"
)

func statsRedisKey(prefix, accountID string) string {
	return prefix + statsKeyPrefix + accountID
}

var statsLuaIncrSuccess = `
	local key = KEYS[1]
	local now = ARGV[1]
	local accountID = ARGV[2]
	
	redis.call("HSET", key, "account_id", accountID)
	redis.call("HINCRBY", key, "total_calls", 1)
	redis.call("HINCRBY", key, "success_calls", 1)
	redis.call("HSET", key, "consecutive_failures", "0")
	redis.call("HSET", key, "last_used_at", now)
	
	return 1
`

var statsLuaIncrFailure = `
	local key = KEYS[1]
	local now = ARGV[1]
	local errMsg = ARGV[2]
	local accountID = ARGV[3]
	
	redis.call("HSET", key, "account_id", accountID)
	redis.call("HINCRBY", key, "total_calls", 1)
	redis.call("HINCRBY", key, "failed_calls", 1)
	local failures = redis.call("HINCRBY", key, "consecutive_failures", 1)
	redis.call("HSET", key, "last_error_at", now)
	redis.call("HSET", key, "last_error_msg", errMsg)
	
	return failures
`

func (s *Store) GetStats(ctx context.Context, accountID string) (*account.AccountStats, error) {
	key := statsRedisKey(s.keyPrefix, accountID)
	data, err := s.client.HGetAll(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("redis store: failed to get stats: %w", err)
	}

	if len(data) == 0 {
		return &account.AccountStats{AccountID: accountID}, nil
	}

	stats := &account.AccountStats{
		AccountID:           data[statsFieldAccountID],
		TotalCalls:          ParseInt64(data[statsFieldTotalCalls]),
		SuccessCalls:        ParseInt64(data[statsFieldSuccessCalls]),
		FailedCalls:         ParseInt64(data[statsFieldFailedCalls]),
		ConsecutiveFailures: ParseInt(data[statsFieldConsecutiveFailures]),
		LastUsedAt:          ParseTimePtr(data[statsFieldLastUsedAt]),
		LastErrorAt:         ParseTimePtr(data[statsFieldLastErrorAt]),
		LastErrorMsg:        data[statsFieldLastErrorMsg],
	}

	if stats.AccountID == "" {
		stats.AccountID = accountID
	}

	return stats, nil
}

func (s *Store) IncrSuccess(ctx context.Context, accountID string) error {
	key := statsRedisKey(s.keyPrefix, accountID)
	now := FormatTime(time.Now())

	_, err := s.client.Eval(ctx, statsLuaIncrSuccess, []string{key}, now, accountID)
	if err != nil {
		return fmt.Errorf("redis store: failed to incr success: %w", err)
	}
	return nil
}

func (s *Store) IncrFailure(ctx context.Context, accountID string, errMsg string) (int, error) {
	key := statsRedisKey(s.keyPrefix, accountID)
	now := FormatTime(time.Now())

	result, err := s.client.Eval(ctx, statsLuaIncrFailure, []string{key}, now, errMsg, accountID)
	if err != nil {
		return 0, fmt.Errorf("redis store: failed to incr failure: %w", err)
	}

	failures, ok := result.(int64)
	if !ok {
		return 0, fmt.Errorf("redis store: unexpected result type: %T", result)
	}
	return int(failures), nil
}

func (s *Store) UpdateLastUsed(ctx context.Context, accountID string, t time.Time) error {
	key := statsRedisKey(s.keyPrefix, accountID)
	timeStr := FormatTime(t)

	err := s.client.HSet(ctx, key, statsFieldAccountID, accountID, statsFieldLastUsedAt, timeStr)
	if err != nil {
		return fmt.Errorf("redis store: failed to update last used: %w", err)
	}
	return nil
}

func (s *Store) GetConsecutiveFailures(ctx context.Context, accountID string) (int, error) {
	key := statsRedisKey(s.keyPrefix, accountID)
	val, err := s.client.HGet(ctx, key, statsFieldConsecutiveFailures)
	if err != nil {
		if IsNotFound(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("redis store: failed to get consecutive failures: %w", err)
	}
	return ParseInt(val), nil
}

func (s *Store) ResetConsecutiveFailures(ctx context.Context, accountID string) error {
	key := statsRedisKey(s.keyPrefix, accountID)
	err := s.client.HSet(ctx, key, statsFieldConsecutiveFailures, "0")
	if err != nil {
		return fmt.Errorf("redis store: failed to reset consecutive failures: %w", err)
	}
	return nil
}

func (s *Store) RemoveStats(ctx context.Context, accountID string) error {
	key := statsRedisKey(s.keyPrefix, accountID)
	err := s.client.Del(ctx, key)
	if err != nil {
		return fmt.Errorf("redis store: failed to remove stats: %w", err)
	}
	return nil
}
