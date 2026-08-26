package store

import (
	"iceguard/internal/domain"
	"path/filepath"
	"testing"
)

func TestPersistence(t *testing.T) {
	s, e := New(filepath.Join(t.TempDir(), "x.json"))
	if e != nil {
		t.Fatal(e)
	}
	b := domain.IceCoreBatch{ID: "x", CoreCode: "c", DrillSite: "d", DepthIntervalM: "1", InitialTemperatureC: -20}
	if e = s.Put(b, -1); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Get("x"); e != nil {
		t.Fatal(e)
	}
}
