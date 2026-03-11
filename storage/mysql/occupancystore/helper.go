package occupancystore

const (
	// queryIncr 原子递增占用计数（upsert 语义）。
	// 使用 LAST_INSERT_ID() 技巧，使递增后的值可通过 Result.LastInsertId() 原子获取。
	queryIncr = `INSERT INTO account_occupancy (account_id, count) 
		VALUES (?, 1) 
		ON DUPLICATE KEY UPDATE count = LAST_INSERT_ID(count + 1)`

	// queryDecr 原子递减占用计数，保证不低于 0。
	queryDecr = `UPDATE account_occupancy SET count = GREATEST(count - 1, 0) WHERE account_id=?`

	// queryGet 查询当前占用计数。
	queryGet = `SELECT count FROM account_occupancy WHERE account_id=?`

	// queryDelete 删除指定账号的占用记录。
	queryDelete = `DELETE FROM account_occupancy WHERE account_id=?`

	// queryCleanZero 清理计数为 0 的记录，避免表膨胀。
	queryCleanZero = `DELETE FROM account_occupancy WHERE count = 0`
)
