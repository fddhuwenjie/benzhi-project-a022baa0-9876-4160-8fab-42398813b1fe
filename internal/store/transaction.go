package store

import "iceguard/internal/domain"

type Transaction struct {
	s        *Store
	batch    domain.IceCoreBatch
	expected int
}

func (s *Store) Begin(id string, expected int) (*Transaction, error) {
	b, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	return &Transaction{s: s, batch: b, expected: expected}, nil
}
func (t *Transaction) Batch() *domain.IceCoreBatch { return &t.batch }
func (t *Transaction) Commit() error               { return t.s.Put(t.batch, t.expected) }
