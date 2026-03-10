package affinitystore

const (
	// queryGetAffinity 根据亲和键查询绑定目标 ID。
	queryGetAffinity = `SELECT target_id FROM affinities WHERE affinity_key=?`

	// queryUpsertAffinity 插入或更新亲和键到目标 ID 的绑定关系。
	queryUpsertAffinity = `INSERT INTO affinities (affinity_key, target_id) VALUES (?, ?) 
		ON DUPLICATE KEY UPDATE target_id=?, updated_at=NOW(3)`
)
