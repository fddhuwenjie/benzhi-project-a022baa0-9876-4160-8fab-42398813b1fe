package store

import (
	"encoding/json"
	"iceguard/internal/domain"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

type Snapshot struct {
	Version     int                            `json:"version"`
	Batches     map[string]domain.IceCoreBatch `json:"batches"`
	Idempotency map[string]json.RawMessage     `json:"idempotency"`
}
type Store struct {
	mu   sync.RWMutex
	path string
	data Snapshot
}

func New(path string) (*Store, error) {
	s := &Store{path: path, data: Snapshot{Version: 1, Batches: map[string]domain.IceCoreBatch{}, Idempotency: map[string]json.RawMessage{}}}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}
func (s *Store) load() error {
	b, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err = json.Unmarshal(b, &s.data); err != nil {
		return err
	}
	if s.data.Version == 0 {
		s.data.Version = 1
	}
	if s.data.Batches == nil {
		s.data.Batches = map[string]domain.IceCoreBatch{}
	}
	if s.data.Idempotency == nil {
		s.data.Idempotency = map[string]json.RawMessage{}
	}
	return nil
}
func (s *Store) persistLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(s.data, "", "  ")
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
func (s *Store) Get(id string) (domain.IceCoreBatch, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.data.Batches[id]
	if !ok {
		return domain.IceCoreBatch{}, domain.ErrNotFound
	}
	return b, nil
}
func (s *Store) Put(b domain.IceCoreBatch, expected int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.data.Batches[b.ID]
	if ok && expected < 0 {
		return domain.ErrConflict
	}
	if ok && expected >= 0 && cur.Revision != expected {
		return domain.ErrConflict
	}
	b.Revision = cur.Revision + 1
	s.data.Batches[b.ID] = b
	return s.persistLocked()
}
func (s *Store) SaveIdempotency(key string, v any) error {
	if key == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, _ := json.Marshal(v)
	s.data.Idempotency[key] = raw
	return s.persistLocked()
}
func (s *Store) GetIdempotency(key string) (json.RawMessage, bool) {
	if key == "" {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data.Idempotency[key]
	return v, ok
}
func (s *Store) List() []domain.IceCoreBatch {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.IceCoreBatch, 0, len(s.data.Batches))
	for _, b := range s.data.Batches {
		out = append(out, b)
	}
	return out
}

// ListSnapshot returns an immutable copy of all batches. Callers must not rely
// on the underlying map iteration order.
func (s *Store) ListSnapshot() []domain.IceCoreBatch {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.IceCoreBatch, 0, len(s.data.Batches))
	for _, b := range s.data.Batches {
		// JSON round-trip gives nested slices/maps their own backing storage.
		raw, _ := json.Marshal(b)
		var cp domain.IceCoreBatch
		_ = json.Unmarshal(raw, &cp)
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}
func (s *Store) CheckIntegrity() error { s.mu.RLock(); defer s.mu.RUnlock(); return nil }
