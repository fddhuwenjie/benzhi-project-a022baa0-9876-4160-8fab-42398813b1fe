package filteredlistcachestale_test

import (
	"encoding/json"
	"iceguard/internal/audit"
	"iceguard/internal/domain"
	"iceguard/internal/httpapi"
	"iceguard/internal/store"
	"iceguard/internal/workflow"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestFilteredBatchListRefreshesAfterStateTransition(t *testing.T) {
	dir := t.TempDir()
	st, err := store.New(filepath.Join(dir, "snapshot.json"))
	if err != nil {
		t.Fatal(err)
	}
	wf := workflow.New(st, audit.New(filepath.Join(dir, "audit.jsonl")))
	b, err := wf.CreateBatch(domain.IceCoreBatch{
		CoreCode: "CACHE-EDGE", DrillSite: "DOME-A", DepthIntervalM: "10-20", InitialTemperatureC: -25,
	}, "create-cache-edge")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	b, err = wf.RegisterTransport(b.ID, domain.TransportDeviation{
		AllowedMinC: -40, AllowedMaxC: 0, DeviationNote: "等待质量复核",
		Intervals: []domain.TemperatureInterval{{Start: start, End: start.Add(10 * time.Minute), MinTemperatureC: 5, MaxTemperatureC: 6}},
	}, b.Revision, "transport-cache-edge")
	if err != nil {
		t.Fatal(err)
	}

	handler := httpapi.New(wf).Handler()
	query := func() workflow.BatchListResult {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/v1/batches?has_unmet_preconditions=true", nil)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("query status %d: %s", res.Code, res.Body.String())
		}
		var out workflow.BatchListResult
		if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	if got := query(); len(got.Batches) != 1 || got.Batches[0].ID != b.ID {
		t.Fatalf("initial filtered result = %#v", got.Batches)
	}

	disposition, approved, dual := "继续解冻", true, true
	b, err = wf.ReviewTransportDecision(b.ID, &disposition, &approved, &dual, "quality-owner", "补齐风险处置证据", b.Revision, "review-cache-edge")
	if err != nil {
		t.Fatal(err)
	}
	for _, precondition := range b.Risk.Preconditions {
		if precondition.Required && !precondition.Satisfied {
			t.Fatalf("transition left unmet precondition: %#v", precondition)
		}
	}

	if got := query(); len(got.Batches) != 0 {
		t.Fatalf("stale filtered response retained batch %s after revision %d", got.Batches[0].ID, b.Revision)
	}
}
