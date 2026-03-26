package sqlite

import (
	"context"
	"fmt"
)

const (
	queryGetAffinity    = `SELECT target_id FROM affinities WHERE affinity_key=?`
	queryUpsertAffinity = `INSERT INTO affinities (affinity_key, target_id, updated_at) VALUES (?, ?, strftime('%Y-%m-%d %H:%M:%f', 'now'))
		ON CONFLICT(affinity_key) DO UPDATE SET target_id=?, updated_at=strftime('%Y-%m-%d %H:%M:%f', 'now')`
)

func (s *Store) GetAffinity(affinityKey string) (string, bool) {
	var targetID string
	err := s.client.QueryRow(context.Background(), []any{&targetID},
		queryGetAffinity, affinityKey)
	if err != nil {
		return "", false
	}
	return targetID, true
}

func (s *Store) SetAffinity(affinityKey string, targetID string) {
	_, err := s.client.Exec(context.Background(), queryUpsertAffinity, affinityKey, targetID, targetID)
	if err != nil {
		fmt.Printf("sqlite store: failed to set affinity: %v\n", err)
	}
}
