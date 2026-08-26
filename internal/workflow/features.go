package workflow

import (
	"fmt"
	"iceguard/internal/domain"
	"time"
)

func (s *Service) Risk(id string) (domain.RiskAssessment, error) {
	b, e := s.store.Get(id)
	if e != nil {
		return domain.RiskAssessment{}, e
	}
	if b.Risk == nil {
		r := domain.EvaluateRisk(b)
		if len(r.Missing) > 0 {
			return r, nil
		}
		return r, nil
	}
	if b.Risk.Revision != b.Revision {
		r := domain.EvaluateRisk(b)
		return r, nil
	}
	return *b.Risk, nil
}

func (s *Service) CorrectObservation(id, obsID string, value domain.ThawObservation, reason, operator string, expected int, requestID string) (domain.IceCoreBatch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var replay domain.IceCoreBatch
	if s.replay(requestID, &replay) {
		return replay, nil
	}
	b, e := s.store.Get(id)
	if e != nil {
		return b, e
	}
	if b.Status == domain.StatusSealed {
		return b, fmt.Errorf("sealed batch immutable")
	}
	if b.Revision != expected {
		return b, domain.ErrConflict
	}
	if reason == "" {
		return b, fmt.Errorf("correction reason required")
	}
	if operator == "" {
		return b, fmt.Errorf("operator required")
	}
	for i := range b.Observations {
		if b.Observations[i].ID != obsID {
			continue
		}
		effectiveBefore := domain.EffectiveObservation(b.Observations[i])
		if value.HoldMinutes <= 0 {
			value.HoldMinutes = effectiveBefore.HoldMinutes
		}
		value.StageIndex = b.Observations[i].StageIndex
		if b.Plan != nil && value.StageIndex > 0 && value.StageIndex <= len(b.Plan.Stages) {
			stage := b.Plan.Stages[value.StageIndex-1]
			if value.HoldMinutes < stage.HoldMinutes {
				value.DeviationCode = "HOLD_INSUFFICIENT"
			}
			if value.ObservedTemperatureC < stage.TargetTemperatureC-2 || value.ObservedTemperatureC > stage.TargetTemperatureC+2 {
				value.DeviationCode = "TEMP_OUT_OF_TOLERANCE"
			}
		}
		if value.DeviationCode == "" && value.MeltwaterVolumeML > 50 {
			value.DeviationCode = "MELTWATER_ABNORMAL"
		}
		if value.DeviationCode == "" && domain.ObservationAnomaly(value) {
			value.DeviationCode = "SAMPLE_STATE_ABNORMAL"
		}
		value.ID = obsID
		value.BatchID = id
		value.PlanID = b.Observations[i].PlanID
		c := ObservationCorrectionCompat(value, reason, operator)
		b.Observations[i].Corrections = append(b.Observations[i].Corrections, c)
		anomaly := false
		for _, ob := range b.Observations {
			effective := domain.EffectiveObservation(ob)
			if effective.DeviationCode != "" || domain.ObservationAnomaly(effective) {
				anomaly = true
			}
		}
		if anomaly {
			b.Status = domain.StatusReview
			b.Execution.Paused = true
			b.Execution.GateReason = "corrected_observation_anomaly"
		} else if b.Plan != nil && len(b.Observations) < len(b.Plan.Stages) {
			b.Status = domain.StatusThawing
			b.Execution.Paused = false
			b.Execution.GateReason = ""
		}
		previousGrade := b.IntegrityGrade
		r := domain.EvaluateRisk(b)
		r.Revision = expected + 1
		b.Risk = &r
		b.IntegrityGrade = r.Grade
		if e = s.store.Put(b, expected); e != nil {
			return b, e
		}
		b.Revision = expected + 1
		_, _ = s.audit.Append(id, "observation.corrected", b.Revision, map[string]any{"observation_id": obsID, "reason": reason})
		if previousGrade != b.IntegrityGrade {
			_, _ = s.audit.Append(id, "risk.changed", b.Revision, map[string]any{"before": previousGrade, "after": b.IntegrityGrade, "score": r.Score})
		}
		_ = s.store.SaveIdempotency(requestID, b)
		return b, nil
	}
	return b, domain.ErrNotFound
}
func ObservationCorrectionCompat(v domain.ThawObservation, reason, operator string) domain.ObservationCorrection {
	return domain.ObservationCorrection{ID: fmt.Sprintf("corr-%d", time.Now().UnixNano()), Value: v, Reason: reason, Operator: operator, CreatedAt: time.Now().UTC()}
}

func (s *Service) AddRemediation(id string, tasks []domain.RemediationTask, expected int, requestID string) (domain.IceCoreBatch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var replay domain.IceCoreBatch
	if s.replay(requestID, &replay) {
		return replay, nil
	}
	b, e := s.store.Get(id)
	if e != nil {
		return b, e
	}
	if b.Revision != expected {
		return b, domain.ErrConflict
	}
	if b.Status != domain.StatusReview && b.Status != domain.StatusRemediation {
		return b, domain.ErrInvalidState
	}
	for i := range tasks {
		if tasks[i].ID == "" {
			tasks[i].ID = fmt.Sprintf("task-%d", time.Now().UnixNano())
		}
		if tasks[i].Status == "" {
			tasks[i].Status = "pending"
		}
		if tasks[i].Finding == "" || tasks[i].Assignee == "" || tasks[i].Action == "" || tasks[i].DueAt == nil {
			return b, fmt.Errorf("finding, assignee, action and due_at required")
		}
	}
	b.RemediationTasks = append(b.RemediationTasks, tasks...)
	b.Status = domain.StatusRemediation
	if e = s.store.Put(b, expected); e != nil {
		return b, e
	}
	b.Revision = expected + 1
	_, _ = s.audit.Append(id, "remediation.created", b.Revision, tasks)
	_ = s.store.SaveIdempotency(requestID, b)
	return b, nil
}
func (s *Service) SubmitRemediation(id string, task domain.RemediationTask, expected int, requestID string) (domain.IceCoreBatch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var replay domain.IceCoreBatch
	if s.replay(requestID, &replay) {
		return replay, nil
	}
	b, e := s.store.Get(id)
	if e != nil {
		return b, e
	}
	if b.Revision != expected {
		return b, domain.ErrConflict
	}
	if task.Evidence == "" || task.Action == "" || task.SubmittedBy == "" {
		return b, fmt.Errorf("action, evidence and submitted_by required")
	}
	for i := range b.RemediationTasks {
		if b.RemediationTasks[i].ID == task.ID {
			if b.RemediationTasks[i].DueAt != nil && b.RemediationTasks[i].DueAt.Before(time.Now().UTC()) && task.OverdueReason == "" {
				return b, fmt.Errorf("overdue_reason required")
			}
			b.RemediationTasks[i].Evidence = task.Evidence
			b.RemediationTasks[i].SubmittedBy = task.SubmittedBy
			b.RemediationTasks[i].OverdueReason = task.OverdueReason
			b.RemediationTasks[i].Status = "submitted"
			if e = s.store.Put(b, expected); e != nil {
				return b, e
			}
			b.Revision = expected + 1
			_, _ = s.audit.Append(id, "remediation.submitted", b.Revision, task)
			_ = s.store.SaveIdempotency(requestID, b)
			return b, nil
		}
	}
	return b, domain.ErrNotFound
}
func (s *Service) VerifyRemediation(id, taskID, reviewer, decision, note string, expected int, requestID string) (domain.IceCoreBatch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var replay domain.IceCoreBatch
	if s.replay(requestID, &replay) {
		return replay, nil
	}
	b, e := s.store.Get(id)
	if e != nil {
		return b, e
	}
	if b.Revision != expected {
		return b, domain.ErrConflict
	}
	for i := range b.RemediationTasks {
		t := &b.RemediationTasks[i]
		if t.ID != taskID {
			continue
		}
		if reviewer == "" || reviewer == t.SubmittedBy {
			return b, fmt.Errorf("independent reviewer required")
		}
		if decision != "pass" && decision != "reject" {
			return b, fmt.Errorf("decision must be pass or reject")
		}
		if decision == "reject" && note == "" {
			return b, fmt.Errorf("verification note required when rejected")
		}
		if t.Status != "submitted" {
			return b, fmt.Errorf("task not submitted")
		}
		t.VerifiedBy = reviewer
		t.Verification = decision
		t.VerificationNote = note
		if decision == "pass" {
			t.Status = "passed"
		} else {
			t.Status = "rejected"
		}
		all := true
		for _, x := range b.RemediationTasks {
			if x.Status != "passed" {
				all = false
			}
		}
		if all && decision == "pass" {
			b.Status = domain.StatusReview
			b.Execution.Paused = false
			b.Execution.GateReason = "remediation_passed"
		}
		if e = s.store.Put(b, expected); e != nil {
			return b, e
		}
		b.Revision = expected + 1
		_, _ = s.audit.Append(id, "remediation.verified", b.Revision, map[string]any{"task_id": taskID, "decision": decision})
		_ = s.store.SaveIdempotency(requestID, b)
		return b, nil
	}
	return b, domain.ErrNotFound
}
