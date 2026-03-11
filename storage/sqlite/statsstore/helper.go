package statsstore

const (
	// queryGetStats 根据 account_id 查询统计数据。
	queryGetStats = `SELECT account_id, total_calls, success_calls, failed_calls, 
		consecutive_failures, last_used_at, last_error_at, last_error_msg 
		FROM account_stats WHERE account_id=?`

	// queryIncrSuccess 增加成功调用计数（upsert 语义）。
	// SQLite 使用 INSERT ... ON CONFLICT ... DO UPDATE 代替 MySQL 的 ON DUPLICATE KEY UPDATE。
	queryIncrSuccess = `INSERT INTO account_stats (account_id, total_calls, success_calls, consecutive_failures, last_used_at) 
		VALUES (?, 1, 1, 0, ?) 
		ON CONFLICT(account_id) DO UPDATE SET 
			total_calls = total_calls + 1, 
			success_calls = success_calls + 1, 
			consecutive_failures = 0, 
			last_used_at = ?`

	// queryIncrFailure 增加失败调用计数（upsert 语义）。
	queryIncrFailure = `INSERT INTO account_stats (account_id, total_calls, failed_calls, consecutive_failures, last_error_at, last_error_msg) 
		VALUES (?, 1, 1, 1, ?, ?) 
		ON CONFLICT(account_id) DO UPDATE SET 
			total_calls = total_calls + 1, 
			failed_calls = failed_calls + 1, 
			consecutive_failures = consecutive_failures + 1, 
			last_error_at = ?, 
			last_error_msg = ?`

	// queryGetConsecutiveFailuresAfterIncr 获取递增后的连续失败次数（用于 IncrFailure 后获取最新值）。
	queryGetConsecutiveFailuresAfterIncr = `SELECT consecutive_failures FROM account_stats WHERE account_id=?`

	// queryUpdateLastUsed 更新最后使用时间（upsert 语义）。
	queryUpdateLastUsed = `INSERT INTO account_stats (account_id, last_used_at) 
		VALUES (?, ?) 
		ON CONFLICT(account_id) DO UPDATE SET last_used_at = ?`

	// queryGetConsecutiveFailures 查询连续失败次数。
	queryGetConsecutiveFailures = `SELECT consecutive_failures FROM account_stats WHERE account_id=?`

	// queryResetConsecutiveFailures 重置连续失败次数。
	queryResetConsecutiveFailures = `UPDATE account_stats SET consecutive_failures = 0 WHERE account_id=?`

	// queryDeleteStats 根据 account_id 删除统计记录。
	queryDeleteStats = `DELETE FROM account_stats WHERE account_id=?`
)
