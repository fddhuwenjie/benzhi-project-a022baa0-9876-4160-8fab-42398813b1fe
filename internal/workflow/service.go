package workflow

import (
	"encoding/json"
	"fmt"
	"iceguard/internal/audit"
	"iceguard/internal/domain"
	"iceguard/internal/store"
	"sync"
	"time"
)

type Service struct {
	store *store.Store
	audit *audit.Logger
	mu    sync.Mutex
}

func New(s *store.Store, a *audit.Logger) *Service { return &Service{store: s, audit: a} }
func (s *Service) replay(requestID string, out *domain.IceCoreBatch) bool {
	if requestID == "" {
		return false
	}
	raw, ok := s.store.GetIdempotency(requestID)
	if !ok {
		return false
	}
	return json.Unmarshal(raw, out) == nil
}
func (s *Service) CreateBatch(b domain.IceCoreBatch, requestID string) (domain.IceCoreBatch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.replay(requestID, &b) {
		return b, nil
	}
	if b.ID == "" {
		b.ID = fmt.Sprintf("batch-%d", time.Now().UnixNano())
	}
	b.Status = domain.StatusDraft
	b.Revision = 0
	b.CreatedAt = time.Now().UTC()
	if err := b.Normalize(); err != nil {
		return b, err
	}
	if err := b.Validate(); err != nil {
		return b, err
	}
	a1, z1, _ := domain.ParseDepthInterval(b.DepthIntervalM)
	for _, existing := range s.store.List() {
		if existing.DrillSite != b.DrillSite {
			continue
		}
		if existing.CoreCode == b.CoreCode {
			return b, fmt.Errorf("core_code conflict with batch %s", existing.ID)
		}
		a2, z2, e := domain.ParseDepthInterval(existing.DepthIntervalM)
		if e == nil && a1 < z2 && a2 < z1 {
			return b, fmt.Errorf("depth interval conflicts with batch %s", existing.ID)
		}
	}
	if err := s.store.Put(b, -1); err != nil {
		return b, err
	}
	b.Revision = 1
	_, _ = s.audit.Append(b.ID, "batch.created", b.Revision, b)
	_ = s.store.SaveIdempotency(requestID, b)
	return b, nil
}
func (s *Service) RegisterTransport(id string, t domain.TransportDeviation, expected int, requestID string) (domain.IceCoreBatch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var replay domain.IceCoreBatch
	if s.replay(requestID, &replay) {
		return replay, nil
	}
	b, err := s.store.Get(id)
	if err != nil {
		return b, err
	}
	if b.Status == domain.StatusSealed {
		return b, fmt.Errorf("sealed batch immutable")
	}
	if b.Transport != nil {
		return b, fmt.Errorf("transport already registered; use review update")
	}
	if b.Revision != expected {
		return b, domain.ErrConflict
	}
	previousGrade := b.IntegrityGrade
	if err = domain.SummarizeTransport(&t); err != nil {
		return b, err
	}
	if t.Exceeded && t.DeviationNote == "" {
		return b, fmt.Errorf("deviation_note required when transport exceeded")
	}
	b.Transport = &t
	g, pre := domain.AssessIntegrity(&t, b.Observations)
	b.IntegrityGrade = g
	b.Preconditions = pre
	risk := domain.EvaluateRisk(b)
	risk.Revision = expected + 1
	b.Risk = &risk
	b.IntegrityGrade = risk.Grade
	b.Status = domain.StatusAwaitingPlan
	if err = s.store.Put(b, expected); err != nil {
		return b, err
	}
	b.Revision = expected + 1
	_, _ = s.audit.Append(id, "transport.assessed", b.Revision, t)
	if previousGrade != b.IntegrityGrade {
		_, _ = s.audit.Append(id, "risk.changed", b.Revision, map[string]any{"before": previousGrade, "after": b.IntegrityGrade, "score": b.Risk.Score})
	}
	_ = s.store.SaveIdempotency(requestID, b)
	return b, nil
}

// ReviewTransport supplements disposition/approval evidence while the batch
// is still awaiting a thaw plan. Original temperature evidence is immutable.
func (s *Service) ReviewTransport(id string, disposition *string, qualityApproved, dualReviewed *bool, operator string, expected int, requestID string) (domain.IceCoreBatch, error) {
	return s.ReviewTransportDecision(id, disposition, qualityApproved, dualReviewed, operator, "", expected, requestID)
}

func (s *Service) ReviewTransportDecision(id string, disposition *string, qualityApproved, dualReviewed *bool, operator, reason string, expected int, requestID string) (domain.IceCoreBatch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var replay domain.IceCoreBatch
	if s.replay(requestID, &replay) {
		return replay, nil
	}
	b, err := s.store.Get(id)
	if err != nil {
		return b, err
	}
	if b.Transport == nil {
		return b, fmt.Errorf("transport registration required")
	}
	if operator == "" {
		return b, fmt.Errorf("operator required")
	}
	locked := b.Status != domain.StatusDraft && b.Status != domain.StatusAwaitingPlan
	if b.Status == domain.StatusSealed {
		return b, fmt.Errorf("sealed batch immutable")
	}
	if locked && reason == "" {
		return b, fmt.Errorf("review reason required after plan submission")
	}
	if b.Revision != expected {
		return b, domain.ErrConflict
	}
	before := b.IntegrityGrade
	t := *b.Transport
	if disposition != nil {
		t.Disposition = *disposition
	}
	if qualityApproved != nil {
		t.QualityApproved = *qualityApproved
	}
	if dualReviewed != nil {
		t.DualReviewed = *dualReviewed
	}
	t.ReviewDecisions = append(t.ReviewDecisions, domain.TransportReviewDecision{Disposition: t.Disposition, QualityApproved: t.QualityApproved, DualReviewed: t.DualReviewed, Reason: reason, Operator: operator, ReviewedAt: time.Now().UTC()})
	t.LowExceededMinutes, t.HighExceededMinutes, t.MaxDeviationC, t.ExposureDegreeMinutes = 0, 0, 0, 0
	if err = domain.SummarizeTransport(&t); err != nil {
		return b, err
	}
	if t.Exceeded && t.DeviationNote == "" {
		return b, fmt.Errorf("deviation_note required when transport exceeded")
	}
	b.Transport = &t
	risk := domain.EvaluateRisk(b)
	risk.Revision = expected + 1
	b.Risk = &risk
	b.IntegrityGrade = risk.Grade
	if err = s.store.Put(b, expected); err != nil {
		return b, err
	}
	b.Revision = expected + 1
	payload := map[string]any{"operator": operator, "transport": t, "before_risk": before, "after_risk": b.IntegrityGrade}
	_, _ = s.audit.Append(id, "transport.reviewed", b.Revision, payload)
	if before != b.IntegrityGrade {
		_, _ = s.audit.Append(id, "risk.changed", b.Revision, map[string]any{"before": before, "after": b.IntegrityGrade, "score": b.Risk.Score})
	}
	_ = s.store.SaveIdempotency(requestID, b)
	return b, nil
}
func (s *Service) CreatePlan(id string, p domain.ThawPlan, expected int, requestID string) (domain.IceCoreBatch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var replay domain.IceCoreBatch
	if s.replay(requestID, &replay) {
		return replay, nil
	}
	b, err := s.store.Get(id)
	if err != nil {
		return b, err
	}
	if b.Revision != expected {
		return b, domain.ErrConflict
	}
	if err = domain.CanCreatePlan(b); err != nil {
		return b, err
	}
	p.ID = fmt.Sprintf("plan-%d", time.Now().UnixNano())
	p.BatchID = id
	p.ApprovalStatus = "pending"
	p.Version = len(b.PlanHistory) + 1
	p.Revision = expected + 1
	if len(b.PlanHistory) > 0 {
		p.PreviousVersion = b.PlanHistory[len(b.PlanHistory)-1].Version
		if p.ChangeNote == "" {
			return b, fmt.Errorf("change_note required for revised plan")
		}
	}
	if err = p.Validate(); err != nil {
		return b, err
	}
	b.Plan = &p
	b.PlanHistory = append(b.PlanHistory, p)
	b.Status = domain.StatusPlanPending
	if err = s.store.Put(b, expected); err != nil {
		return b, err
	}
	b.Revision = expected + 1
	_, _ = s.audit.Append(id, "plan.created", b.Revision, p)
	_ = s.store.SaveIdempotency(requestID, b)
	return b, nil
}
func (s *Service) ApprovePlan(id, approver string, expected int, approve bool, requestID string, reasons ...string) (domain.IceCoreBatch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var replay domain.IceCoreBatch
	if s.replay(requestID, &replay) {
		return replay, nil
	}
	b, err := s.store.Get(id)
	if err != nil {
		return b, err
	}
	if b.Revision != expected {
		return b, domain.ErrConflict
	}
	if b.Plan == nil || b.Status != domain.StatusPlanPending {
		return b, domain.ErrInvalidState
	}
	if approver == b.Plan.AuthorID {
		return b, fmt.Errorf("approver must differ from author")
	}
	b.Plan.ApproverID = approver
	if approve {
		b.Plan.ApprovalStatus = "approved"
		b.Status = domain.StatusThawing
	} else {
		b.Plan.ApprovalStatus = "rejected"
		if len(reasons) > 0 {
			b.Plan.RejectionReason = reasons[0]
		}
		if b.Plan.RejectionReason == "" {
			return b, fmt.Errorf("rejection_reason required")
		}
		b.Status = domain.StatusAwaitingPlan
	}
	if len(b.PlanHistory) > 0 {
		b.PlanHistory[len(b.PlanHistory)-1] = *b.Plan
	}
	if err = s.store.Put(b, expected); err != nil {
		return b, err
	}
	b.Revision = expected + 1
	eventType := "plan.approved"
	if !approve {
		eventType = "plan.rejected"
	}
	_, _ = s.audit.Append(id, eventType, b.Revision, map[string]any{"approver": approver, "approved": approve, "plan_version": b.Plan.Version, "reason": b.Plan.RejectionReason})
	_ = s.store.SaveIdempotency(requestID, b)
	return b, nil
}

func (s *Service) ApprovePlanVersion(id, approver string, planVersion, expected int, approve bool, reason, requestID string) (domain.IceCoreBatch, error) {
	b, e := s.store.Get(id)
	if e != nil {
		return b, e
	}
	if b.Plan == nil || planVersion != b.Plan.Version {
		return b, domain.ErrConflict
	}
	return s.ApprovePlan(id, approver, expected, approve, requestID, reason)
}
func (s *Service) AddObservation(id string, o domain.ThawObservation, expected int, requestID string) (domain.IceCoreBatch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var replay domain.IceCoreBatch
	if s.replay(requestID, &replay) {
		return replay, nil
	}
	b, err := s.store.Get(id)
	if err != nil {
		return b, err
	}
	if b.Revision != expected {
		return b, domain.ErrConflict
	}
	if err = domain.CanObserve(b); err != nil {
		return b, err
	}
	if o.StageIndex != len(b.Observations)+1 {
		return b, fmt.Errorf("stage must be sequential")
	}
	stage := b.Plan.Stages[o.StageIndex-1]
	if o.HoldMinutes <= 0 {
		o.HoldMinutes = stage.HoldMinutes
	}
	if o.HoldMinutes < stage.HoldMinutes {
		o.DeviationCode = "HOLD_INSUFFICIENT"
	}
	if o.ObservedTemperatureC < -60 || o.ObservedTemperatureC > 60 {
		return b, fmt.Errorf("observed_temperature_c out of range")
	}
	if o.ObservedTemperatureC < stage.TargetTemperatureC-2 || o.ObservedTemperatureC > stage.TargetTemperatureC+2 {
		o.DeviationCode = "TEMP_OUT_OF_TOLERANCE"
	}
	if o.MeltwaterVolumeML < 0 {
		return b, fmt.Errorf("meltwater_volume_ml must be non-negative")
	}
	if o.DeviationCode == "" && o.MeltwaterVolumeML > 50 {
		o.DeviationCode = "MELTWATER_ABNORMAL"
	}
	if o.DeviationCode == "" && domain.ObservationAnomaly(o) {
		o.DeviationCode = "SAMPLE_STATE_ABNORMAL"
	}
	o.ID = fmt.Sprintf("obs-%d", time.Now().UnixNano())
	o.BatchID = id
	o.PlanID = b.Plan.ID
	o.RecordedAt = time.Now().UTC()
	b.Observations = append(b.Observations, o)
	b.Execution.CompletedStages = len(b.Observations)
	b.Execution.RemainingStages = len(b.Plan.Stages) - len(b.Observations)
	if o.DeviationCode != "" || domain.ObservationAnomaly(o) {
		b.Status = domain.StatusReview
		b.Execution.Paused = true
		b.Execution.GateReason = o.DeviationCode
	}
	if len(b.Observations) == len(b.Plan.Stages) && b.Status == domain.StatusThawing {
		b.Status = domain.StatusReview
		b.Execution.GateReason = "all_stages_completed"
	}
	previousGrade := b.IntegrityGrade
	risk := domain.EvaluateRisk(b)
	risk.Revision = expected + 1
	b.Risk = &risk
	b.IntegrityGrade = risk.Grade
	if err = s.store.Put(b, expected); err != nil {
		return b, err
	}
	b.Revision = expected + 1
	_, _ = s.audit.Append(id, "observation.recorded", b.Revision, o)
	if previousGrade != b.IntegrityGrade {
		_, _ = s.audit.Append(id, "risk.changed", b.Revision, map[string]any{"before": previousGrade, "after": b.IntegrityGrade, "score": risk.Score})
	}
	_ = s.store.SaveIdempotency(requestID, b)
	return b, nil
}
func (s *Service) Review(id string, r domain.ReleaseReview, expected int, requestID string) (domain.IceCoreBatch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var replay domain.IceCoreBatch
	if s.replay(requestID, &replay) {
		return replay, nil
	}
	b, err := s.store.Get(id)
	if err != nil {
		return b, err
	}
	if b.Revision != expected {
		return b, domain.ErrConflict
	}
	if b.Status != domain.StatusReview && b.Status != domain.StatusRemediation {
		return b, domain.ErrInvalidState
	}
	if b.Status == domain.StatusSealed {
		return b, fmt.Errorf("sealed batch immutable")
	}
	if r.ReviewerID == "" {
		return b, fmt.Errorf("reviewer_id required")
	}
	if b.Plan != nil && (r.ReviewerID == b.Plan.AuthorID || r.ReviewerID == b.Plan.ApproverID) {
		return b, fmt.Errorf("independent reviewer required")
	}
	if r.Decision == "pass" {
		for _, t := range b.RemediationTasks {
			if t.Status != "passed" {
				return b, fmt.Errorf("remediation tasks incomplete")
			}
		}
	}
	if r.Decision == "pass" && (!domain.ReleaseReady(b) || b.Execution.Paused) {
		return b, fmt.Errorf("release preconditions not satisfied")
	}
	r.ID = fmt.Sprintf("review-%d", time.Now().UnixNano())
	r.BatchID = id
	b.Review = &r
	if r.Decision == "pass" && r.RetestResult != "fail" {
		now := time.Now().UTC()
		r.SealedAt = &now
		b.Review = &r
		b.Status = domain.StatusSealed
		b.Revision = expected + 1
		r.ReportDigest = b.ReportDigest()
	} else {
		b.Status = domain.StatusRemediation
		if len(r.Tasks) > 0 {
			for i := range r.Tasks {
				if r.Tasks[i].Finding == "" || r.Tasks[i].Assignee == "" || r.Tasks[i].Action == "" || r.Tasks[i].DueAt == nil {
					return b, fmt.Errorf("remediation finding, assignee, action and due_at required")
				}
				if r.Tasks[i].ID == "" {
					r.Tasks[i].ID = fmt.Sprintf("task-%d-%d", time.Now().UnixNano(), i)
				}
				r.Tasks[i].Status = "pending"
			}
			b.RemediationTasks = append(b.RemediationTasks, r.Tasks...)
		} else if len(b.RemediationTasks) == 0 {
			return b, fmt.Errorf("structured remediation tasks required")
		}
	}
	if err = s.store.Put(b, expected); err != nil {
		return b, err
	}
	b.Revision = expected + 1
	_, _ = s.audit.Append(id, "release.reviewed", b.Revision, r)
	_ = s.store.SaveIdempotency(requestID, b)
	return b, nil
}
func (s *Service) GetBatch(id string) (domain.IceCoreBatch, error) { return s.store.Get(id) }
func (s *Service) ListBatches() []domain.IceCoreBatch              { return s.store.ListSnapshot() }
