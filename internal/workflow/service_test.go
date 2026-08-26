package workflow

import (
	"iceguard/internal/audit"
	"iceguard/internal/domain"
	"iceguard/internal/store"
	"path/filepath"
	"testing"
)

func TestLifecycle(t *testing.T) {
	d := t.TempDir()
	st, _ := store.New(filepath.Join(d, "s.json"))
	a := audit.New(filepath.Join(d, "a.log"))
	wf := New(st, a)
	b, e := wf.CreateBatch(domain.IceCoreBatch{CoreCode: "C", DrillSite: "S", DepthIntervalM: "1-2", InitialTemperatureC: -30}, "r1")
	if e != nil {
		t.Fatal(e)
	}
	b, e = wf.RegisterTransport(b.ID, domain.TransportDeviation{Summary: "ok", AllowedMinC: -40, AllowedMaxC: 0}, b.Revision, "r2")
	if e != nil {
		t.Fatal(e)
	}
	b, e = wf.CreatePlan(b.ID, domain.ThawPlan{AuthorID: "a", Stages: []domain.ThawStage{{Index: 1, TargetTemperatureC: -1, HoldMinutes: 5}}}, b.Revision, "r3")
	if e != nil {
		t.Fatal(e)
	}
	b, e = wf.ApprovePlan(b.ID, "q", b.Revision, true, "r4")
	if e != nil {
		t.Fatal(e)
	}
	b, e = wf.AddObservation(b.ID, domain.ThawObservation{StageIndex: 1, ObservedTemperatureC: -1}, b.Revision, "r5")
	if e != nil {
		t.Fatal(e)
	}
	b, e = wf.Review(b.ID, domain.ReleaseReview{ReviewerID: "rv", Decision: "pass"}, b.Revision, "r6")
	if e != nil || b.Status != domain.StatusSealed {
		t.Fatalf("review %v %s", e, b.Status)
	}
}
