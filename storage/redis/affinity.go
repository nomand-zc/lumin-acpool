package redis

import (
	"context"
	"fmt"
)

const (
	affinityKeyPrefix = "affinity:"
)

func (s *Store) affinityRedisKey(key string) string {
	return s.keyPrefix + affinityKeyPrefix + key
}

func (s *Store) GetAffinity(affinityKey string) (string, bool) {
	key := s.affinityRedisKey(affinityKey)
	val, err := s.client.Get(context.Background(), key)
	if err != nil {
		return "", false
	}
	return val, true
}

func (s *Store) SetAffinity(affinityKey string, targetID string) {
	key := s.affinityRedisKey(affinityKey)
	err := s.client.Set(context.Background(), key, targetID, 0)
	if err != nil {
		fmt.Printf("redis store: failed to set affinity: %v\n", err)
	}
}
