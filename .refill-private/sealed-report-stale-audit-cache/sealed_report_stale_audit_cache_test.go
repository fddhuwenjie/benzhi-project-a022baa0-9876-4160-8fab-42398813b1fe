package sealed_report_stale_audit_cache_test

import (
	"iceguard/internal/audit"
	"iceguard/internal/domain"
	"iceguard/internal/httpapi"
	"iceguard/internal/store"
	"iceguard/internal/workflow"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestReleaseReportCacheRevalidatesAuditResource(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "store.json")
	auditPath := filepath.Join(dir, "audit.jsonl")
	st, err := store.New(storePath)
	if err != nil {
		t.Fatal(err)
	}
	wf := workflow.New(st, audit.New(auditPath))

	batch, err := wf.CreateBatch(domain.IceCoreBatch{
		CoreCode:            "CACHE-1",
		DrillSite:           "南极测试点",
		DepthIntervalM:      "10-20",
		InitialTemperatureC: -30,
	}, "cache-create")
	if err != nil {
		t.Fatal(err)
	}
	batch, err = wf.RegisterTransport(batch.ID, domain.TransportDeviation{
		Summary:         "运输正常",
		AllowedMinC:     -40,
		AllowedMaxC:     0,
		MinTemperatureC: -32,
		MaxTemperatureC: -25,
	}, batch.Revision, "cache-transport")
	if err != nil {
		t.Fatal(err)
	}
	batch, err = wf.CreatePlan(batch.ID, domain.ThawPlan{
		AuthorID: "author",
		Stages: []domain.ThawStage{{
			Index:              1,
			TargetTemperatureC: -1,
			HoldMinutes:        5,
		}},
	}, batch.Revision, "cache-plan")
	if err != nil {
		t.Fatal(err)
	}
	batch, err = wf.ApprovePlan(batch.ID, "approver", batch.Revision, true, "cache-approve")
	if err != nil {
		t.Fatal(err)
	}
	batch, err = wf.AddObservation(batch.ID, domain.ThawObservation{
		StageIndex:           1,
		ObservedTemperatureC: -1,
		HoldMinutes:          5,
	}, batch.Revision, "cache-observation")
	if err != nil {
		t.Fatal(err)
	}
	batch, err = wf.Review(batch.ID, domain.ReleaseReview{
		ReviewerID: "reviewer",
		Decision:   "pass",
	}, batch.Revision, "cache-review")
	if err != nil {
		t.Fatal(err)
	}

	handler := httpapi.New(wf).Handler()
	path := "/v1/batches/" + batch.ID + "/release-report"
	first := httptest.NewRecorder()
	handler.ServeHTTP(first, httptest.NewRequest(http.MethodGet, path, nil))
	if first.Code != http.StatusOK {
		t.Fatalf("initial report status = %d, body = %s", first.Code, first.Body.String())
	}

	if err = os.WriteFile(auditPath, []byte("corrupted audit resource\n"), 0644); err != nil {
		t.Fatal(err)
	}
	second := httptest.NewRecorder()
	handler.ServeHTTP(second, httptest.NewRequest(http.MethodGet, path, nil))
	if second.Code == http.StatusOK {
		t.Fatalf("cached report remained verified after audit resource corruption: %s", second.Body.String())
	}
}
