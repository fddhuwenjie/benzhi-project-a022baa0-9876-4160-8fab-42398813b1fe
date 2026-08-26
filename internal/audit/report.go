package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"iceguard/internal/domain"
	"time"
)

type SealedReport struct {
	Batch             domain.IceCoreBatch `json:"batch"`
	GeneratedAt       time.Time           `json:"generated_at"`
	ChainTail         string              `json:"chain_tail"`
	Digest            string              `json:"digest"`
	Verified          bool                `json:"verified"`
	VerifiedEvents    int                 `json:"verified_events"`
	VerificationError string              `json:"verification_error,omitempty"`
}

func BuildReport(b domain.IceCoreBatch, tail string) SealedReport {
	generated := time.Unix(0, 0).UTC()
	if b.Review != nil && b.Review.SealedAt != nil {
		generated = *b.Review.SealedAt
	}
	r := SealedReport{Batch: b, GeneratedAt: generated, ChainTail: tail}
	raw, _ := json.Marshal(r)
	h := sha256.Sum256(raw)
	r.Digest = hex.EncodeToString(h[:])
	return r
}
