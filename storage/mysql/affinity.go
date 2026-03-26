package mysql

import (
	"context"
	"fmt"
)

const (
	// queryGetAffinity 根据亲和键查询绑定目标 ID。
	queryGetAffinity = `SELECT target_id FROM affinities WHERE affinity_key=?`

	// queryUpsertAffinity 插入或更新亲和键到目标 ID 的绑定关系。
	queryUpsertAffinity = `INSERT INTO affinities (affinity_key, target_id) VALUES (?, ?)
		ON DUPLICATE KEY UPDATE target_id=?, updated_at=NOW(3)`
)

// GetAffinity 获取亲和键对应的绑定目标 ID。
func (s *Store) GetAffinity(affinityKey string) (string, bool) {
	var targetID string
	err := s.client.QueryRow(context.Background(), []any{&targetID},
		queryGetAffinity, affinityKey)
	if err != nil {
		return "", false
	}
	return targetID, true
}

// SetAffinity 设置亲和键到目标 ID 的绑定关系。
// 使用 INSERT ... ON DUPLICATE KEY UPDATE 实现 upsert 语义。
func (s *Store) SetAffinity(affinityKey string, targetID string) {
	_, err := s.client.Exec(context.Background(), queryUpsertAffinity, affinityKey, targetID, targetID)
	if err != nil {
		// AffinityStore 接口不返回 error，记录错误后静默处理
		fmt.Printf("mysql store: failed to set affinity: %v\n", err)
	}
}
