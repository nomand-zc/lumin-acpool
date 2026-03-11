package usagestore

const (
	// queryGetAllUsages 根据 account_id 查询所有用量追踪数据。
	queryGetAllUsages = `SELECT rule_index, source_type, time_granularity, window_size, rule_total, 
		local_used, remote_used, remote_remain, window_start, window_end, last_sync_at 
		FROM tracked_usages WHERE account_id=? ORDER BY rule_index ASC`

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
