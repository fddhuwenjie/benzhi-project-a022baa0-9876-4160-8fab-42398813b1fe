package store_persist_rollback_test

import (
	"errors"
	"iceguard/internal/domain"
	"iceguard/internal/store"
	"os"
	"path/filepath"
	"testing"
)

func TestPutFailureDoesNotPublishBatch(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "blocked", "snapshot.json")
	s, err := store.New(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "blocked"), []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	b := domain.IceCoreBatch{ID: "batch-rollback", CoreCode: "CORE", DrillSite: "SITE", DepthIntervalM: "1-2"}
	if err := s.Put(b, -1); err == nil {
		t.Fatal("expected persistence failure")
	}
	if _, err := s.Get(b.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("failed Put became visible in memory: %v", err)
	}
}
