package affinitystore

const (
	// queryGetAffinity 根据亲和键查询绑定目标 ID。
	queryGetAffinity = `SELECT target_id FROM affinities WHERE affinity_key=?`

	// queryUpsertAffinity 插入或更新亲和键到目标 ID 的绑定关系。
	// SQLite 使用 INSERT ... ON CONFLICT ... DO UPDATE 代替 MySQL 的 ON DUPLICATE KEY UPDATE。
	queryUpsertAffinity = `INSERT INTO affinities (affinity_key, target_id, updated_at) VALUES (?, ?, strftime('%Y-%m-%d %H:%M:%f', 'now')) 
		ON CONFLICT(affinity_key) DO UPDATE SET target_id=?, updated_at=strftime('%Y-%m-%d %H:%M:%f', 'now')`
)
