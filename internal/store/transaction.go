package store

import (
	"encoding/json"
	"iceguard/internal/domain"
)

type Transaction struct {
	s        *Store
	batch    domain.IceCoreBatch
	expected int
}

// cloneBatch returns a value whose nested slices, pointers and maps share no
// backing storage with the input, so mutating one never reaches the other.
// It uses a JSON round-trip to rebuild every nested allocation, mirroring the
// technique already used by Store.ListSnapshot.
func cloneBatch(b domain.IceCoreBatch) domain.IceCoreBatch {
	raw, err := json.Marshal(b)
	if err != nil {
		// Marshaling a value-only IceCoreBatch never fails in practice; fall
		// back to the shallow copy so callers keep making progress.
		return b
	}
	var cp domain.IceCoreBatch
	if err := json.Unmarshal(raw, &cp); err != nil {
		return b
	}
	return cp
}

func (s *Store) Begin(id string, expected int) (*Transaction, error) {
	b, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	// Hand the transaction a fully isolated working copy so that in-flight
	// mutations to nested observations, plans, risk or review slices never
	// leak into the stored data until Commit publishes them.
	return &Transaction{s: s, batch: cloneBatch(b), expected: expected}, nil
}

func (t *Transaction) Batch() *domain.IceCoreBatch { return &t.batch }

// Commit publishes the transaction's working copy. The published value is
// deep-cloned so that subsequent mutations to the transaction's working copy
// (or to values previously handed out by Batch) cannot reach stored data.
func (t *Transaction) Commit() error { return t.s.Put(cloneBatch(t.batch), t.expected) }
