package store

import "fmt"

func (s *Store) ValidateSnapshot() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.Version != 1 {
		return fmt.Errorf("unsupported snapshot version")
	}
	return nil
}
