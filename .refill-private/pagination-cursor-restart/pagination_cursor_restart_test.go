package pagination_cursor_restart_test

import (
	"encoding/json"
	"fmt"
	"iceguard/internal/audit"
	"iceguard/internal/domain"
	"iceguard/internal/httpapi"
	"iceguard/internal/store"
	"iceguard/internal/workflow"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"
)

func TestPaginationCursorSurvivesServiceRestart(t *testing.T) {
	dir := t.TempDir()
	snapshotPath := filepath.Join(dir, "store.json")
	auditPath := filepath.Join(dir, "audit.jsonl")
	st, err := store.New(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	created := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	for i, id := range []string{"batch-a", "batch-b", "batch-c"} {
		batch := domain.IceCoreBatch{
			ID:                  id,
			CoreCode:            fmt.Sprintf("CORE-%d", i),
			DrillSite:           "restart-site",
			DepthIntervalM:      fmt.Sprintf("%d-%d", i+1, i+2),
			InitialTemperatureC: -25,
			Status:              domain.StatusDraft,
			CreatedAt:           created.Add(time.Duration(i) * time.Minute),
		}
		if err := st.Put(batch, -1); err != nil {
			t.Fatal(err)
		}
	}

	firstServer := httptest.NewServer(httpapi.New(workflow.New(st, audit.New(auditPath))).Handler())
	firstPage := getPage(t, firstServer.URL+"/v1/batches?limit=1")
	firstServer.Close()
	if len(firstPage.Batches) != 1 || firstPage.NextCursor == "" {
		t.Fatalf("first page missing continuation cursor: %#v", firstPage)
	}

	reopened, err := store.New(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	restartedServer := httptest.NewServer(httpapi.New(workflow.New(reopened, audit.New(auditPath))).Handler())
	defer restartedServer.Close()
	secondPage := getPage(t, restartedServer.URL+"/v1/batches?limit=1&cursor="+url.QueryEscape(firstPage.NextCursor))
	if len(secondPage.Batches) != 1 || secondPage.Batches[0].ID != "batch-b" {
		t.Fatalf("cursor did not continue after restart: %#v", secondPage)
	}
}

func getPage(t *testing.T, endpoint string) workflow.BatchListResult {
	t.Helper()
	resp, err := http.Get(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var body map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&body)
		t.Fatalf("pagination request returned HTTP %d: %#v", resp.StatusCode, body)
	}
	var page workflow.BatchListResult
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	return page
}
