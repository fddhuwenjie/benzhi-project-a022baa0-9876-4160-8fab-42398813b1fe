package audit_log_rotation_loss_test

import (
	"iceguard/internal/audit"
	"os"
	"path/filepath"
	"testing"
)

func TestAuditAppendFollowsReplacedLog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	logger := audit.New(path)

	if _, err := logger.Append("batch-rotation", "batch.created", 1, map[string]string{"status": "draft"}); err != nil {
		t.Fatalf("append initial event: %v", err)
	}
	if err := os.Rename(path, path+".rotated"); err != nil {
		t.Fatalf("rotate audit log: %v", err)
	}
	if err := os.WriteFile(path, nil, 0644); err != nil {
		t.Fatalf("create replacement audit log: %v", err)
	}

	if _, err := logger.Append("batch-rotation", "transport.assessed", 2, map[string]string{"status": "awaiting_plan"}); err != nil {
		t.Fatalf("append after rotation: %v", err)
	}
	events, err := logger.Events()
	if err != nil {
		t.Fatalf("read active audit log: %v", err)
	}
	if len(events) != 1 || events[0].Type != "transport.assessed" || events[0].Revision != 2 {
		t.Fatalf("active audit log lost post-rotation event: got %#v", events)
	}
}
