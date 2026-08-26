package domain

import (
	"encoding/json"
	"sort"
)

func StableSnapshot(b IceCoreBatch) []byte {
	cp := b
	sort.Slice(cp.Observations, func(i, j int) bool { return cp.Observations[i].StageIndex < cp.Observations[j].StageIndex })
	v, _ := json.Marshal(cp)
	return v
}
func ReleaseReady(b IceCoreBatch) bool {
	if b.Status != StatusReview || b.Plan == nil || b.Plan.ApprovalStatus != "approved" || len(b.Plan.Stages) == 0 || len(b.Observations) != len(b.Plan.Stages) || b.Execution.Paused {
		return false
	}
	risk := b.Risk
	if risk == nil || risk.Revision != b.Revision {
		r := EvaluateRisk(b)
		risk = &r
	}
	if len(risk.Missing) > 0 {
		return false
	}
	for _, p := range risk.Preconditions {
		if p.Required && !p.Satisfied {
			return false
		}
	}
	for _, t := range b.RemediationTasks {
		if t.Status != "passed" {
			return false
		}
	}
	return true
}
