package workflow

import (
	"encoding/base64"
	"fmt"
	"iceguard/internal/audit"
	"iceguard/internal/domain"
	"sort"
	"strings"
	"time"
)

type BatchListOptions struct {
	DrillSite string
	Status    *domain.BatchStatus
	Integrity *domain.IntegrityGrade
	HasUnmet  *bool
	Limit     int
	Cursor    string
}
type BatchListStats struct {
	StatusCounts         map[string]int `json:"status_counts"`
	IntegrityGradeCounts map[string]int `json:"integrity_grade_counts"`
	UnmetPreconditions   int            `json:"unmet_preconditions"`
}
type BatchListResult struct {
	Batches              []domain.IceCoreBatch `json:"batches"`
	NextCursor           string                `json:"next_cursor,omitempty"`
	Stats                BatchListStats        `json:"stats"`
	StatusCounts         map[string]int        `json:"status_counts"`
	IntegrityGradeCounts map[string]int        `json:"integrity_grade_counts"`
	UnmetPreconditions   int                   `json:"unmet_preconditions"`
}

func encodeBatchCursor(b domain.IceCoreBatch) string {
	return base64.RawURLEncoding.EncodeToString([]byte(b.CreatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00") + "|" + b.ID))
}
func decodeBatchCursor(v string) (string, string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(v)
	if err != nil {
		return "", "", fmt.Errorf("invalid cursor")
	}
	p := strings.SplitN(string(raw), "|", 2)
	if len(p) != 2 || p[0] == "" || p[1] == "" {
		return "", "", fmt.Errorf("invalid cursor")
	}
	if _, err := time.Parse("2006-01-02T15:04:05.999999999Z07:00", p[0]); err != nil {
		return "", "", fmt.Errorf("invalid cursor")
	}
	return p[0], p[1], nil
}

func (s *Service) QueryBatches(o BatchListOptions) (BatchListResult, error) {
	if o.Limit == 0 {
		o.Limit = 50
	}
	if o.Limit <= 0 {
		return BatchListResult{}, fmt.Errorf("limit must be positive")
	}
	var ctime, cid string
	if o.Cursor != "" {
		var err error
		ctime, cid, err = decodeBatchCursor(o.Cursor)
		if err != nil {
			return BatchListResult{}, err
		}
	}
	all := s.store.ListSnapshot()
	filtered := make([]domain.IceCoreBatch, 0, len(all))
	for _, b := range all {
		if o.DrillSite != "" && b.DrillSite != o.DrillSite {
			continue
		}
		if o.Status != nil && b.Status != *o.Status {
			continue
		}
		if o.Integrity != nil && b.IntegrityGrade != *o.Integrity {
			continue
		}
		risk := b.Risk
		if risk == nil || risk.Revision != b.Revision {
			rr := domain.EvaluateRisk(b)
			risk = &rr
		}
		unmet := false
		for _, p := range risk.Preconditions {
			if p.Required && !p.Satisfied {
				unmet = true
				break
			}
		}
		if len(risk.Missing) > 0 {
			unmet = true
		}
		if o.HasUnmet != nil && unmet != *o.HasUnmet {
			continue
		}
		filtered = append(filtered, b)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].CreatedAt.Equal(filtered[j].CreatedAt) {
			return filtered[i].ID < filtered[j].ID
		}
		return filtered[i].CreatedAt.Before(filtered[j].CreatedAt)
	})
	start := 0
	if ctime != "" {
		for i, b := range filtered {
			key := b.CreatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
			if key > ctime || (key == ctime && b.ID > cid) {
				start = i
				break
			}
			start = len(filtered)
		}
	}
	page := filtered[start:]
	next := ""
	if len(page) > o.Limit {
		next = encodeBatchCursor(page[o.Limit-1])
		page = page[:o.Limit]
	}
	stats := BatchListStats{StatusCounts: map[string]int{}, IntegrityGradeCounts: map[string]int{}}
	for _, st := range []domain.BatchStatus{domain.StatusDraft, domain.StatusAwaitingPlan, domain.StatusPlanPending, domain.StatusThawing, domain.StatusReview, domain.StatusRemediation, domain.StatusSealed} {
		stats.StatusCounts[string(st)] = 0
	}
	for _, g := range []domain.IntegrityGrade{domain.GradeLow, domain.GradeMedium, domain.GradeHigh, domain.GradeCritical} {
		stats.IntegrityGradeCounts[string(g)] = 0
	}
	for _, b := range filtered {
		stats.StatusCounts[string(b.Status)]++
		if b.IntegrityGrade != "" {
			stats.IntegrityGradeCounts[string(b.IntegrityGrade)]++
		}
		risk := b.Risk
		if risk == nil || risk.Revision != b.Revision {
			rr := domain.EvaluateRisk(b)
			risk = &rr
		}
		unmet := len(risk.Missing) > 0
		for _, p := range risk.Preconditions {
			if p.Required && !p.Satisfied {
				unmet = true
			}
		}
		if unmet {
			stats.UnmetPreconditions++
		}
	}
	return BatchListResult{Batches: page, NextCursor: next, Stats: stats, StatusCounts: stats.StatusCounts, IntegrityGradeCounts: stats.IntegrityGradeCounts, UnmetPreconditions: stats.UnmetPreconditions}, nil
}

type PlanDiff struct {
	FromVersion int            `json:"from_version"`
	ToVersion   int            `json:"to_version"`
	Changes     []string       `json:"changes"`
	Before      map[string]any `json:"before,omitempty"`
	After       map[string]any `json:"after,omitempty"`
}

func planDiff(a, b domain.ThawPlan) PlanDiff {
	d := PlanDiff{FromVersion: a.Version, ToVersion: b.Version, Before: map[string]any{}, After: map[string]any{}}
	targetChanged, holdChanged := a.TargetTemperatureC != b.TargetTemperatureC, a.HoldMinutes != b.HoldMinutes
	for i := 0; i < len(a.Stages) && i < len(b.Stages); i++ {
		if a.Stages[i].TargetTemperatureC != b.Stages[i].TargetTemperatureC {
			targetChanged = true
		}
		if a.Stages[i].HoldMinutes != b.Stages[i].HoldMinutes {
			holdChanged = true
		}
	}
	if targetChanged {
		d.Changes = append(d.Changes, "target_temperature_c")
		d.Before["target_temperature_c"], d.After["target_temperature_c"] = a.Stages, b.Stages
	}
	if holdChanged {
		d.Changes = append(d.Changes, "hold_minutes")
		d.Before["hold_minutes"], d.After["hold_minutes"] = a.Stages, b.Stages
	}
	if len(a.Stages) != len(b.Stages) {
		d.Changes = append(d.Changes, "stage_count")
		d.Before["stage_count"], d.After["stage_count"] = len(a.Stages), len(b.Stages)
	}
	if strings.Join(a.SafetyPreconditions, "\x00") != strings.Join(b.SafetyPreconditions, "\x00") {
		d.Changes = append(d.Changes, "safety_preconditions")
		d.Before["safety_preconditions"], d.After["safety_preconditions"] = a.SafetyPreconditions, b.SafetyPreconditions
	}
	if len(d.Changes) == 0 {
		d.Before, d.After = nil, nil
	}
	return d
}
func (s *Service) ThawPlanSummary(id string, version int) (map[string]any, error) {
	b, err := s.store.Get(id)
	if err != nil {
		return nil, err
	}
	if len(b.PlanHistory) == 0 || b.Plan == nil {
		return nil, fmt.Errorf("%w: thaw plan pending", domain.ErrInvalidState)
	}
	var requested *domain.ThawPlan
	if version > 0 {
		found := false
		for _, p := range b.PlanHistory {
			if p.Version == version {
				found = true
				cp := p
				requested = &cp
				break
			}
		}
		if !found {
			return nil, domain.ErrNotFound
		}
	}
	history := append([]domain.ThawPlan(nil), b.PlanHistory...)
	sort.Slice(history, func(i, j int) bool { return history[i].Version < history[j].Version })
	diffs := make([]PlanDiff, 0, len(history)-1)
	for i := 1; i < len(history); i++ {
		diffs = append(diffs, planDiff(history[i-1], history[i]))
	}
	cur := *b.Plan
	out := map[string]any{"current": cur, "history": history, "diffs": diffs, "batch_revision": b.Revision, "revision": b.Revision, "plan_revision": cur.Revision, "current_executable_status": cur.ApprovalStatus}
	if requested != nil {
		out["requested"] = *requested
	}
	return out, nil
}

type ObservationStageView struct {
	StageIndex    int                     `json:"stage_index"`
	Planned       domain.ThawStage        `json:"planned"`
	Status        string                  `json:"status"`
	Original      *domain.ThawObservation `json:"original,omitempty"`
	Effective     *domain.ThawObservation `json:"effective,omitempty"`
	DeviationCode string                  `json:"deviation_code,omitempty"`
	Anomaly       bool                    `json:"anomaly"`
}

func (s *Service) ObservationQuery(id string, anomaly *bool, stageIndex int) (map[string]any, error) {
	filter := ""
	if anomaly != nil {
		filter = fmt.Sprintf("%t", *anomaly)
	}
	return s.ObservationQueryFiltered(id, filter, stageIndex)
}

func (s *Service) ObservationQueryFiltered(id, anomaly string, stageIndex int) (map[string]any, error) {
	b, err := s.store.Get(id)
	if err != nil {
		return nil, err
	}
	if b.Plan == nil || b.Plan.ApprovalStatus != "approved" {
		return nil, fmt.Errorf("%w: approved plan required", domain.ErrInvalidState)
	}
	if stageIndex < 0 || stageIndex > len(b.Plan.Stages) {
		return nil, fmt.Errorf("stage_index out of range")
	}
	obsBy := map[int]domain.ThawObservation{}
	for _, o := range b.Observations {
		obsBy[o.StageIndex] = o
	}
	all := make([]ObservationStageView, 0, len(b.Plan.Stages))
	completed, anomalies, melt := 0, 0, 0.0
	paused := b.Execution.Paused
	reason := b.Execution.GateReason
	for _, st := range b.Plan.Stages {
		v := ObservationStageView{StageIndex: st.Index, Planned: st, Status: "pending"}
		if o, ok := obsBy[st.Index]; ok {
			cp := o
			eff := domain.EffectiveObservation(o)
			ec := eff.DeviationCode
			v.Status = "completed"
			v.Original = &cp
			v.Effective = &eff
			v.DeviationCode = ec
			v.Anomaly = domain.ObservationAnomaly(eff)
			completed++
			if v.Anomaly {
				anomalies++
				melt += eff.MeltwaterVolumeML
				paused = true
				if reason == "" {
					reason = ec
				}
			}
		}
		if anomaly == "true" && !v.Anomaly {
			continue
		}
		if anomaly == "false" && v.Anomaly {
			continue
		}
		if anomaly != "" && anomaly != "true" && anomaly != "false" && v.DeviationCode != anomaly {
			continue
		}
		if stageIndex > 0 && st.Index != stageIndex {
			continue
		}
		all = append(all, v)
	}
	return map[string]any{"progress": map[string]any{"completed_stages": completed, "remaining_stages": len(b.Plan.Stages) - completed, "paused": paused, "gate_reason": reason}, "stages": all, "anomaly_count": anomalies, "abnormal_meltwater_total_ml": melt, "revision": b.Revision}, nil
}

func (s *Service) ReleasePrecheck(id, reviewer string) (map[string]any, error) {
	b, err := s.store.Get(id)
	if err != nil {
		return nil, err
	}
	if b.Status == domain.StatusSealed {
		n, tail, ve := s.audit.VerifyChainSummary(id, b.Revision)
		return map[string]any{"status": "sealed", "gates": []any{}, "blocking_reasons": []string{}, "allowed_decisions": []string{"sealed"}, "audit_summary": map[string]any{"event_count": n, "chain_tail": tail, "verified": ve == nil, "verification_error": errorString(ve)}, "revision": b.Revision}, nil
	}
	type gate struct {
		Name      string `json:"name"`
		Satisfied bool   `json:"satisfied"`
		Blocking  bool   `json:"blocking"`
		Detail    string `json:"detail,omitempty"`
	}
	gates := []gate{}
	blocking := []string{}
	add := func(name string, ok bool, detail string) {
		if ok {
			detail = ""
		}
		gates = append(gates, gate{name, ok, !ok, detail})
		if !ok {
			blocking = append(blocking, detail)
		}
	}
	completed := b.Plan != nil && len(b.Observations) == len(b.Plan.Stages) && len(b.Plan.Stages) > 0
	add("approved_plan", b.Plan != nil && b.Plan.ApprovalStatus == "approved", "approved plan required")
	add("stages_complete", completed, "stages incomplete")
	risk := b.Risk
	if risk == nil || risk.Revision != b.Revision {
		rr := domain.EvaluateRisk(b)
		risk = &rr
	}
	if risk != nil {
		add("risk_evidence_complete", len(risk.Missing) == 0, "risk assessment incomplete: "+strings.Join(risk.Missing, ","))
		for _, p := range risk.Preconditions {
			if p.Required {
				add(p.Name, p.Satisfied, "risk precondition "+p.Name+" not satisfied")
			}
		}
	} else {
		add("risk_assessment", false, "risk assessment incomplete")
	}
	add("not_paused", !b.Execution.Paused, "execution paused: "+b.Execution.GateReason)
	for _, t := range b.RemediationTasks {
		add("remediation:"+t.ID, t.Status == "passed", "remediation task "+t.ID+" not passed")
	}
	independent := reviewer != "" && (b.Plan == nil || (reviewer != b.Plan.AuthorID && reviewer != b.Plan.ApproverID))
	add("independent_reviewer", independent, "independent reviewer required")
	n, tail, ve := s.audit.VerifyChainSummary(id, b.Revision)
	allowed := []string{"remediation", "reject"}
	if len(blocking) == 0 {
		allowed = []string{"pass", "remediation", "reject"}
	}
	return map[string]any{"status": string(b.Status), "gates": gates, "blocking_reasons": blocking, "allowed_decisions": allowed, "audit_summary": map[string]any{"event_count": n, "chain_tail": tail, "verified": ve == nil, "verification_error": errorString(ve)}, "revision": b.Revision}, nil
}
func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (s *Service) RiskSummary(id string) (map[string]any, error) {
	b, e := s.GetBatch(id)
	if e != nil {
		return nil, e
	}
	if b.Risk != nil && b.Risk.Revision == b.Revision {
		return map[string]any{"batch_id": id, "grade": b.Risk.Grade, "score": b.Risk.Score, "factors": b.Risk.Factors, "preconditions": b.Risk.Preconditions, "revision": b.Risk.Revision, "missing": b.Risk.Missing, "status": b.Status}, nil
	}
	r := domain.EvaluateRisk(b)
	return map[string]any{"batch_id": id, "grade": r.Grade, "score": r.Score, "factors": r.Factors, "preconditions": r.Preconditions, "revision": b.Revision, "missing": r.Missing, "status": b.Status}, nil
}
func (s *Service) Observations(id string) ([]domain.ThawObservation, error) {
	b, e := s.GetBatch(id)
	if e != nil {
		return nil, e
	}
	return b.Observations, nil
}
func (s *Service) ReleaseReport(id string) (audit.SealedReport, error) {
	b, e := s.GetBatch(id)
	if e != nil {
		return audit.SealedReport{}, e
	}
	if b.Status != domain.StatusSealed {
		return audit.SealedReport{}, domain.ErrInvalidState
	}
	if b.Review == nil || b.Review.ReportDigest == "" || b.Review.ReportDigest != b.ReportDigest() {
		return audit.SealedReport{}, fmt.Errorf("sealed snapshot digest mismatch")
	}
	n, tail, e := s.audit.VerifyChain(id, b.Revision)
	r := audit.BuildReport(b, tail)
	r.VerifiedEvents = n
	r.ChainTail = tail
	if e != nil {
		r.Verified = false
		r.VerificationError = e.Error()
		return r, e
	}
	r.Verified = true
	return r, nil
}
