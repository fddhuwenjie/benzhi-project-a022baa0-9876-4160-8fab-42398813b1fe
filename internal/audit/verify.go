package audit

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
)

func (l *Logger) VerifyRevision(batchID string, revision int) bool {
	ev, err := l.Events()
	if err != nil {
		return false
	}
	for _, e := range ev {
		if e.BatchID == batchID && e.Revision > revision {
			return false
		}
	}
	return true
}

func (l *Logger) VerifyChain(batchID string, revision int) (int, string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	f, err := os.Open(l.path)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	prev := ""
	count := 0
	maxBatchRevision := 0
	lastBatchRevision := 0
	tail := ""
	foundSeal := false
	for scanner.Scan() {
		rawLine := append([]byte(nil), scanner.Bytes()...)
		var e Event
		if err = json.Unmarshal(rawLine, &e); err != nil {
			return count, tail, err
		}
		b := canonicalEvent(e)
		h := sha256.Sum256(b)
		valid := hex.EncodeToString(h[:]) == e.Digest
		if !valid {
			marker := []byte(`"digest":"` + e.Digest + `"`)
			legacy := bytes.Replace(rawLine, marker, []byte(`"digest":""`), 1)
			old := sha256.Sum256(legacy)
			valid = hex.EncodeToString(old[:]) == e.Digest
		}
		if !valid {
			return count, tail, fmt.Errorf("audit digest mismatch at %s", e.ID)
		}
		if e.PreviousDigest != prev {
			return count, tail, fmt.Errorf("audit chain broken at %s", e.ID)
		}
		prev = e.Digest
		tail = e.Digest
		if e.BatchID == batchID {
			count++
			if e.Revision != lastBatchRevision && e.Revision != lastBatchRevision+1 {
				return count, tail, fmt.Errorf("revision continuity failure at %s", e.ID)
			}
			lastBatchRevision = e.Revision
			if e.Revision > maxBatchRevision {
				maxBatchRevision = e.Revision
			}
			if e.Revision > revision {
				return count, tail, fmt.Errorf("revision continuity failure at %s", e.ID)
			}
			if e.Revision == revision {
				foundSeal = true
			}
		}
		if foundSeal {
			break
		}
	}
	if err = scanner.Err(); err != nil {
		return count, tail, err
	}
	if !foundSeal || maxBatchRevision != revision {
		return count, tail, fmt.Errorf("revision continuity failure: expected %d got %d", revision, maxBatchRevision)
	}
	return count, tail, nil
}

// VerifyChainSummary validates the global hash chain and revision continuity
// up to a batch's current revision without requiring the batch to be sealed.
func (l *Logger) VerifyChainSummary(batchID string, revision int) (int, string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	f, err := os.Open(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, "", nil
		}
		return 0, "", err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	prev, tail := "", ""
	count, last := 0, 0
	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		var e Event
		if err := json.Unmarshal(line, &e); err != nil {
			return count, tail, err
		}
		h := sha256.Sum256(canonicalEvent(e))
		if hex.EncodeToString(h[:]) != e.Digest {
			marker := []byte(`"digest":"` + e.Digest + `"`)
			legacy := bytes.Replace(line, marker, []byte(`"digest":""`), 1)
			old := sha256.Sum256(legacy)
			if hex.EncodeToString(old[:]) != e.Digest {
				return count, tail, fmt.Errorf("audit digest mismatch at %s", e.ID)
			}
		}
		if e.PreviousDigest != prev {
			return count, tail, fmt.Errorf("audit chain broken at %s", e.ID)
		}
		prev, tail = e.Digest, e.Digest
		if e.BatchID == batchID {
			count++
			if e.Revision != last && e.Revision != last+1 {
				return count, tail, fmt.Errorf("revision continuity failure at %s", e.ID)
			}
			last = e.Revision
			if e.Revision > revision {
				return count, tail, fmt.Errorf("revision exceeds current")
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return count, tail, err
	}
	if count > 0 && last != revision {
		return count, tail, fmt.Errorf("revision continuity failure: expected %d got %d", revision, last)
	}
	return count, tail, nil
}
