package store

import "encoding/json"

func (s *Store) IdempotencyCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.data.Idempotency)
}
func (s *Store) DecodeIdempotency(key string, v any) bool {
	raw, ok := s.GetIdempotency(key)
	if !ok {
		return false
	}
	return json.Unmarshal(raw, v) == nil
}
