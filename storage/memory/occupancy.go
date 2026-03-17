package memory

import "context"

func (s *Store) IncrOccupancy(_ context.Context, accountID string) (int64, error) {
	s.occupancyMu.Lock()
	defer s.occupancyMu.Unlock()

	s.occupancyStore[accountID]++
	return s.occupancyStore[accountID], nil
}

func (s *Store) DecrOccupancy(_ context.Context, accountID string) error {
	s.occupancyMu.Lock()
	defer s.occupancyMu.Unlock()

	if s.occupancyStore[accountID] > 0 {
		s.occupancyStore[accountID]--
	}
	// 清理零值键，避免内存泄漏
	if s.occupancyStore[accountID] == 0 {
		delete(s.occupancyStore, accountID)
	}
	return nil
}

func (s *Store) GetOccupancy(_ context.Context, accountID string) (int64, error) {
	s.occupancyMu.Lock()
	defer s.occupancyMu.Unlock()

	return s.occupancyStore[accountID], nil
}

func (s *Store) GetOccupancies(_ context.Context, accountIDs []string) (map[string]int64, error) {
	s.occupancyMu.Lock()
	defer s.occupancyMu.Unlock()

	result := make(map[string]int64, len(accountIDs))
	for _, id := range accountIDs {
		if v, ok := s.occupancyStore[id]; ok {
			result[id] = v
		}
	}
	return result, nil
}
