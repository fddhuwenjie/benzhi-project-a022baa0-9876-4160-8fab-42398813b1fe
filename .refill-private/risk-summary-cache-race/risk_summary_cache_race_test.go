package risk_summary_cache_race_test

import (
	"fmt"
	"iceguard/internal/audit"
	"iceguard/internal/domain"
	"iceguard/internal/httpapi"
	"iceguard/internal/store"
	"iceguard/internal/workflow"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
)

func TestConcurrentRiskSummaryCacheAccess(t *testing.T) {
	dir := t.TempDir()
	st, err := store.New(filepath.Join(dir, "snapshot.json"))
	if err != nil {
		t.Fatal(err)
	}
	wf := workflow.New(st, audit.New(filepath.Join(dir, "audit.jsonl")))
	batch, err := wf.CreateBatch(domain.IceCoreBatch{
		CoreCode:            "RACE-CORE",
		DrillSite:           "RACE-SITE",
		DepthIntervalM:      "10-20",
		InitialTemperatureC: -30,
	}, "race-create")
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(httpapi.New(wf).Handler())
	defer server.Close()

	const workers = 16
	ready := make(chan struct{}, workers)
	start := make(chan struct{})
	results := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ready <- struct{}{}
			<-start
			resp, requestErr := http.Get(server.URL + "/v1/batches/" + batch.ID + "/risk-summary")
			if requestErr != nil {
				results <- requestErr
				return
			}
			defer resp.Body.Close()
			_, readErr := io.ReadAll(resp.Body)
			if readErr != nil {
				results <- readErr
				return
			}
			if resp.StatusCode != http.StatusOK {
				results <- fmt.Errorf("unexpected status: %d", resp.StatusCode)
				return
			}
			results <- nil
		}()
	}
	for i := 0; i < workers; i++ {
		<-ready
	}
	close(start)
	wg.Wait()
	close(results)
	for result := range results {
		if result != nil {
			t.Fatal(result)
		}
	}
}
