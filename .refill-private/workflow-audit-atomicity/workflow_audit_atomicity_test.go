package workflow_audit_atomicity_test

import (
	"errors"
	"iceguard/internal/audit"
	"iceguard/internal/domain"
	"iceguard/internal/store"
	"iceguard/internal/workflow"
	"path/filepath"
	"testing"
)

func TestCreateBatchFailsAtomicallyWhenAuditCannotPersist(t *testing.T) {
	root := t.TempDir()
	st, err := store.New(filepath.Join(root, "snapshot.json"))
	if err != nil {
		t.Fatal(err)
	}
	wf := workflow.New(st, audit.New(filepath.Join(root, "missing", "audit.jsonl")))
	in := domain.IceCoreBatch{ID: "batch-audit", CoreCode: "CORE-A", DrillSite: "SITE-A", DepthIntervalM: "1-2", InitialTemperatureC: -20}
	out, err := wf.CreateBatch(in, "request-audit")
	if err == nil {
		t.Fatal("CreateBatch reported success although its audit event was not durable")
	}
	if _, getErr := st.Get(out.ID); !errors.Is(getErr, domain.ErrNotFound) {
		t.Fatalf("batch survived failed audited transaction: %v", getErr)
	}
}
