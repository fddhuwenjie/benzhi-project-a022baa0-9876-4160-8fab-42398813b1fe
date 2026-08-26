package store

import (
	"os"
	"time"
)

func (s *Store) Backup(path string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}
func (s *Store) LastModified() time.Time {
	if st, err := os.Stat(s.path); err == nil {
		return st.ModTime()
	}
	return time.Time{}
}
