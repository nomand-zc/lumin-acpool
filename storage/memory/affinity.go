package memory

// GetAffinity 获取亲和键对应的绑定目标 ID。
func (s *Store) GetAffinity(affinityKey string) (string, bool) {
	s.affinityMu.RLock()
	targetID, exists := s.affinityBindings[affinityKey]
	s.affinityMu.RUnlock()
	return targetID, exists
}

// SetAffinity 设置亲和键到目标 ID 的绑定关系。
func (s *Store) SetAffinity(affinityKey string, targetID string) {
	s.affinityMu.Lock()
	defer s.affinityMu.Unlock()
	// 检查容量，超过上限时清空重建
	if len(s.affinityBindings) >= s.affinityMaxEntries {
		s.affinityBindings = make(map[string]string, s.affinityMaxEntries/2)
	}
	s.affinityBindings[affinityKey] = targetID
}
