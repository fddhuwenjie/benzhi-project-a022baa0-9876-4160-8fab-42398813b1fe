package transaction_alias_leak_test

import (
	"iceguard/internal/domain"
	"iceguard/internal/store"
	"path/filepath"
	"testing"
)

func TestUncommittedTransactionCannotMutateStore(t *testing.T) {
	s, err := store.New(filepath.Join(t.TempDir(), "snapshot.json"))
	if err != nil {
		t.Fatal(err)
	}
	b := domain.IceCoreBatch{
		ID:           "batch-tx",
		Observations: []domain.ThawObservation{{ID: "obs-1", AppearanceNote: "clear"}},
	}
	if err := s.Put(b, -1); err != nil {
		t.Fatal(err)
	}
	tx, err := s.Begin(b.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	tx.Batch().Observations[0].AppearanceNote = "mutated without commit"
	got, err := s.Get(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Observations[0].AppearanceNote != "clear" {
		t.Fatalf("uncommitted nested mutation leaked into store: %q", got.Observations[0].AppearanceNote)
	}
}
