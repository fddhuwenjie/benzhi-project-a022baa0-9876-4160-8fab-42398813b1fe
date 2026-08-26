package workflow

import (
	"iceguard/internal/audit"
	"iceguard/internal/domain"
	"iceguard/internal/store"
	"path/filepath"
	"testing"
	"time"
)

func featureService(t *testing.T) *Service {
	t.Helper()
	d := t.TempDir()
	st, e := store.New(filepath.Join(d, "s.json"))
	if e != nil {
		t.Fatal(e)
	}
	return New(st, audit.New(filepath.Join(d, "a.jsonl")))
}

func TestBatchConflictAndAdjacentDepth(t *testing.T) {
	wf := featureService(t)
	_, e := wf.CreateBatch(domain.IceCoreBatch{CoreCode: " a ", DrillSite: " S ", DepthIntervalM: "100 m - 110 m", InitialTemperatureC: -20}, "a")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = wf.CreateBatch(domain.IceCoreBatch{CoreCode: "b", DrillSite: "S", DepthIntervalM: "105-115", InitialTemperatureC: -20}, "b"); e == nil {
		t.Fatal("expected overlap")
	}
	b, e := wf.CreateBatch(domain.IceCoreBatch{CoreCode: "c", DrillSite: "S", DepthIntervalM: "110-120", InitialTemperatureC: -20}, "c")
	if e != nil || b.DepthIntervalM != "110-120" {
		t.Fatalf("adjacent: %v %#v", e, b)
	}
}

func TestTransportMetricsAndAtomicValidation(t *testing.T) {
	wf := featureService(t)
	b, _ := wf.CreateBatch(domain.IceCoreBatch{CoreCode: "A", DrillSite: "S", DepthIntervalM: "1-2", InitialTemperatureC: -20}, "a")
	start := time.Now().UTC().Add(-time.Hour)
	in := []domain.TemperatureInterval{{Start: start, End: start.Add(10 * time.Minute), MinTemperatureC: 3, MaxTemperatureC: 4}, {Start: start.Add(10 * time.Minute), End: start.Add(30 * time.Minute), MinTemperatureC: 5, MaxTemperatureC: 6}}
	_, e := wf.RegisterTransport(b.ID, domain.TransportDeviation{AllowedMinC: -20, AllowedMaxC: 2, Intervals: in}, b.Revision, "bad")
	if e == nil {
		t.Fatal("note required")
	}
	unchanged, _ := wf.GetBatch(b.ID)
	if unchanged.Revision != b.Revision {
		t.Fatal("validation changed revision")
	}
	got, e := wf.RegisterTransport(b.ID, domain.TransportDeviation{AllowedMinC: -20, AllowedMaxC: 2, Intervals: in, DeviationNote: "冷链处置", Disposition: "隔离复核"}, b.Revision, "ok")
	if e != nil {
		t.Fatal(e)
	}
	if got.Transport.HighExceededMinutes != 30 || got.Transport.MaxDeviationC != 4 || got.Transport.ExposureDegreeMinutes != 100 {
		t.Fatalf("metrics %#v", got.Transport)
	}
	again, e := wf.RegisterTransport(b.ID, domain.TransportDeviation{}, b.Revision, "ok")
	if e != nil || again.Revision != got.Revision {
		t.Fatalf("replay %v", e)
	}
}

func TestObservationCorrection(t *testing.T) {
	wf := featureService(t)
	b, _ := wf.CreateBatch(domain.IceCoreBatch{CoreCode: "A", DrillSite: "S", DepthIntervalM: "1-2", InitialTemperatureC: -20}, "a")
	b, _ = wf.RegisterTransport(b.ID, domain.TransportDeviation{AllowedMinC: -40, AllowedMaxC: 0, MinTemperatureC: -20, MaxTemperatureC: -10}, b.Revision, "t")
	b, _ = wf.CreatePlan(b.ID, domain.ThawPlan{AuthorID: "u", Stages: []domain.ThawStage{{Index: 1, TargetTemperatureC: 0, HoldMinutes: 5}, {Index: 2, TargetTemperatureC: 1, HoldMinutes: 5}}}, b.Revision, "p")
	b, _ = wf.ApprovePlan(b.ID, "q", b.Revision, true, "ap")
	b, _ = wf.AddObservation(b.ID, domain.ThawObservation{StageIndex: 1, ObservedTemperatureC: 9, HoldMinutes: 5}, b.Revision, "o")
	if b.Status != domain.StatusReview {
		t.Fatal("expected pause")
	}
	b, e := wf.CorrectObservation(b.ID, b.Observations[0].ID, domain.ThawObservation{ObservedTemperatureC: 0}, "录入错误", "tech", b.Revision, "c")
	if e != nil {
		t.Fatal(e)
	}
	if b.Status != domain.StatusThawing || len(b.Observations[0].Corrections) != 1 {
		t.Fatalf("correction %#v", b)
	}
	if b.Observations[0].ObservedTemperatureC != 9 {
		t.Fatal("original observation was overwritten")
	}
}

func TestSealedReportReceiptStable(t *testing.T) {
	wf := featureService(t)
	b, _ := wf.CreateBatch(domain.IceCoreBatch{CoreCode: "R", DrillSite: "S", DepthIntervalM: "1-2", InitialTemperatureC: -20}, "a")
	b, _ = wf.RegisterTransport(b.ID, domain.TransportDeviation{AllowedMinC: -40, AllowedMaxC: 0, MinTemperatureC: -20, MaxTemperatureC: -10}, b.Revision, "t")
	b, _ = wf.CreatePlan(b.ID, domain.ThawPlan{AuthorID: "u", Stages: []domain.ThawStage{{Index: 1, TargetTemperatureC: 0, HoldMinutes: 5}}}, b.Revision, "p")
	b, _ = wf.ApprovePlan(b.ID, "q", b.Revision, true, "ap")
	b, _ = wf.AddObservation(b.ID, domain.ThawObservation{StageIndex: 1, ObservedTemperatureC: 0, HoldMinutes: 5}, b.Revision, "o")
	b, e := wf.Review(b.ID, domain.ReleaseReview{ReviewerID: "r", Decision: "pass"}, b.Revision, "rv")
	if e != nil {
		t.Fatal(e)
	}
	a, e := wf.ReleaseReport(b.ID)
	if e != nil {
		t.Fatal(e)
	}
	z, e := wf.ReleaseReport(b.ID)
	if e != nil || !a.Verified || a.Digest != z.Digest || a.ChainTail != z.ChainTail {
		t.Fatalf("receipt %#v %#v %v", a, z, e)
	}
}
