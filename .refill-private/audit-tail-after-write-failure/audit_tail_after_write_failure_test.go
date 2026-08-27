package audit_tail_after_write_failure_test

import (
	"iceguard/internal/audit"
	"os"
	"path/filepath"
	"testing"
)

func TestAuditAppendFailureDoesNotPoisonChain(t *testing.T) {
	root := t.TempDir()
	logDir := filepath.Join(root, "audit")
	logPath := filepath.Join(logDir, "audit.jsonl")
	logger := audit.New(logPath)

	if _, err := logger.Append("batch-recovery", "batch.created", 1, map[string]string{"attempt": "failed"}); err == nil {
		t.Fatal("首次追加应因日志目录不存在而失败")
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := logger.Append("batch-recovery", "batch.created", 1, map[string]string{"attempt": "persisted"}); err != nil {
		t.Fatalf("资源恢复后的追加应成功: %v", err)
	}

	if err := logger.Verify(); err != nil {
		t.Fatalf("失败的追加不得污染后续持久化链: %v", err)
	}
	events, err := logger.Events()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].PreviousDigest != "" {
		t.Fatalf("活动日志应从持久化链尾开始: %#v", events)
	}
}
