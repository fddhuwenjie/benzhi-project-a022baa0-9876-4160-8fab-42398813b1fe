package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

type Event struct {
	ID             string    `json:"id"`
	BatchID        string    `json:"batch_id"`
	Type           string    `json:"type"`
	Revision       int       `json:"revision"`
	Payload        any       `json:"payload"`
	PreviousDigest string    `json:"previous_digest"`
	Digest         string    `json:"digest"`
	At             time.Time `json:"at"`
}
type Logger struct {
	mu   sync.Mutex
	path string
	tail string
	file *os.File
	fi   os.FileInfo
}

func New(path string) *Logger { return &Logger{path: path} }

// statTailLocked rescans the active file at l.path to recompute the chain tail.
// It is used after a log rotation so subsequent events chain onto the real
// active file rather than a stale in-memory tail.
func (l *Logger) statTailLocked() {
	l.tail = ""
	f, err := os.Open(l.path)
	if err != nil {
		return
	}
	defer f.Close()
	var prev string
	dec := json.NewDecoder(f)
	for {
		var e Event
		if err := dec.Decode(&e); err != nil {
			if err == io.EOF {
				break
			}
			return
		}
		prev = e.Digest
	}
	l.tail = prev
}

// rotatedLocked reports whether the cached file handle no longer points at the
// file currently living at l.path (e.g. because ops renamed audit.jsonl and
// created a fresh file at the same path). os.SameFile compares the underlying
// identity (device + inode on POSIX systems), so a renamed file and a newly
// created file at the same path are treated as different.
func (l *Logger) rotatedLocked() bool {
	if l.file == nil {
		return true
	}
	cur, err := os.Stat(l.path)
	if err != nil {
		return true
	}
	return !os.SameFile(l.fi, cur)
}

// reopenLocked closes any stale handle and opens the active file for
// appending, recording its identity and recomputing the chain tail so the
// next event chains onto whatever is currently in the file.
func (l *Logger) reopenLocked() error {
	if l.file != nil {
		l.file.Close()
		l.file = nil
	}
	l.fi = nil
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	l.file = f
	if l.fi, err = f.Stat(); err != nil {
		l.file.Close()
		l.file = nil
		l.fi = nil
		return err
	}
	l.statTailLocked()
	return nil
}
func (l *Logger) Append(batchID, typ string, rev int, payload any) (Event, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.rotatedLocked() {
		if err := l.reopenLocked(); err != nil {
			return Event{}, err
		}
	}
	e := Event{ID: fmt.Sprintf("evt-%d", time.Now().UnixNano()), BatchID: batchID, Type: typ, Revision: rev, Payload: payload, PreviousDigest: l.tail, At: time.Now().UTC()}
	raw := canonicalEvent(e)
	h := sha256.Sum256(raw)
	e.Digest = hex.EncodeToString(h[:])
	l.tail = e.Digest
	enc := json.NewEncoder(l.file)
	if err := enc.Encode(e); err != nil {
		return e, err
	}
	return e, l.file.Sync()
}
func canonicalEvent(e Event) []byte {
	e.Digest = ""
	raw, _ := json.Marshal(e)
	var v any
	_ = json.Unmarshal(raw, &v)
	raw, _ = json.Marshal(v)
	return raw
}
func (l *Logger) Verify() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	f, err := os.Open(l.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	var prev string
	dec := json.NewDecoder(f)
	for {
		var e Event
		if err := dec.Decode(&e); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		if e.PreviousDigest != prev {
			return fmt.Errorf("audit chain broken")
		}
		prev = e.Digest
	}
	l.tail = prev
	return nil
}
func (l *Logger) Tail() string { l.mu.Lock(); defer l.mu.Unlock(); return l.tail }
