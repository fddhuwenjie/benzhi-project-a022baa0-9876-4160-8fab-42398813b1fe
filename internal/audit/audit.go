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
}

func New(path string) *Logger { return &Logger{path: path} }
func (l *Logger) Append(batchID, typ string, rev int, payload any) (Event, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e := Event{ID: fmt.Sprintf("evt-%d", time.Now().UnixNano()), BatchID: batchID, Type: typ, Revision: rev, Payload: payload, PreviousDigest: l.tail, At: time.Now().UTC()}
	raw := canonicalEvent(e)
	h := sha256.Sum256(raw)
	e.Digest = hex.EncodeToString(h[:])
	l.tail = e.Digest
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return e, err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	return e, enc.Encode(e)
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
