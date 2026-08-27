package release_report_cancel_read_test

import (
	"bytes"
	"context"
	"iceguard/internal/audit"
	"iceguard/internal/domain"
	"iceguard/internal/httpapi"
	"iceguard/internal/store"
	"iceguard/internal/workflow"
	"net/http/httptest"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestReleaseReportCancellationInterruptsAuditRead(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "snapshot.json")
	auditPath := filepath.Join(dir, "audit.pipe")
	if err := syscall.Mkfifo(auditPath, 0600); err != nil {
		t.Fatal(err)
	}

	st, err := store.New(storePath)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1700000000, 0).UTC()
	b := domain.IceCoreBatch{
		ID:       "sealed-cancel",
		Status:   domain.StatusSealed,
		Revision: 1,
		Review:   &domain.ReleaseReview{Decision: "pass", SealedAt: &now},
	}
	b.Review.ReportDigest = b.ReportDigest()
	if err = st.Put(b, -1); err != nil {
		t.Fatal(err)
	}

	h := httpapi.New(workflow.New(st, audit.New(auditPath))).Handler()
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/v1/batches/sealed-cancel/release-report", nil).WithContext(ctx)
	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		h.ServeHTTP(recorder, req)
		close(done)
	}()

	writer, err := os.OpenFile(auditPath, os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, errno := syscall.Syscall(syscall.SYS_FCNTL, writer.Fd(), uintptr(syscall.F_SETPIPE_SZ), 4096); errno != 0 {
		_ = writer.Close()
		t.Fatalf("set pipe capacity: %v", errno)
	}
	// The write is larger than the controlled pipe capacity but smaller than
	// bufio.Scanner's token limit. Its completion proves VerifyChainContext is
	// blocked inside Scan waiting for the unfinished JSON line.
	partial := append([]byte{'{'}, bytes.Repeat([]byte{' '}, 32*1024)...)
	if _, err = writer.Write(partial); err != nil {
		_ = writer.Close()
		t.Fatal(err)
	}
	cancel()
	select {
	case <-done:
		_ = writer.Close()
		if recorder.Body.String() != "{\"error\":\"context canceled\"}\n" {
			t.Fatalf("expected context cancellation response, got %s", recorder.Body.String())
		}
	case <-time.After(time.Second):
		_ = writer.Close()
		<-done
		t.Fatal("canceled request remained blocked in audit read")
	}
}
