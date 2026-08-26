package domain

import "time"

import "fmt"

func AssessIntegrity(t *TransportDeviation, observations []ThawObservation) (IntegrityGrade, []string) {
	score := 0
	pre := []string{}
	if t != nil && t.Exceeded {
		score += 2
		pre = append(pre, "运输偏差已复核")
	}
	for _, o := range observations {
		o = EffectiveObservation(o)
		if o.DeviationCode != "" {
			score += 2
		}
		if o.MeltwaterVolumeML > 50 {
			score++
		}
	}
	if score >= 5 {
		return GradeCritical, append(pre, "需质量负责人批准")
	}
	if score >= 3 {
		return GradeHigh, append(pre, "需双人复核")
	}
	if score >= 1 {
		return GradeMedium, append(pre, "需记录偏差说明")
	}
	return GradeLow, pre
}

func EvaluateRisk(b IceCoreBatch) RiskAssessment {
	r := RiskAssessment{Revision: b.Revision, EvaluatedAt: time.Now().UTC()}
	if b.Transport == nil {
		r.Missing = []string{"transport"}
		return r
	}
	if b.Transport.AllowedMinC == 0 && b.Transport.AllowedMaxC == 0 {
		r.Missing = []string{"allowed_temperature_range"}
		return r
	}
	add := func(name string, v, th, c float64) {
		r.Factors = append(r.Factors, RiskFactor{Name: name, Value: v, Threshold: th, Contribution: c, Triggered: v > th})
		if v > th {
			r.Score += c
		}
	}
	add("low_exceeded_minutes", b.Transport.LowExceededMinutes, 0, 2)
	add("high_exceeded_minutes", b.Transport.HighExceededMinutes, 0, 2)
	add("max_deviation_c", b.Transport.MaxDeviationC, 3, 2)
	add("exposure_degree_minutes", b.Transport.ExposureDegreeMinutes, 30, 2)
	for _, o := range b.Observations {
		o = EffectiveObservation(o)
		if o.DeviationCode != "" {
			add("observation_deviation", 1, 0, 2)
		}
		if o.MeltwaterVolumeML > 50 {
			add("abnormal_meltwater_ml", o.MeltwaterVolumeML, 50, 1)
		}
		if ObservationAnomaly(o) && o.DeviationCode == "" && o.MeltwaterVolumeML <= 50 {
			add("abnormal_sample_state", 1, 0, 2)
		}
	}
	switch {
	case r.Score >= 6:
		r.Grade = GradeCritical
	case r.Score >= 4:
		r.Grade = GradeHigh
	case r.Score >= 2:
		r.Grade = GradeMedium
	default:
		r.Grade = GradeLow
	}
	required := r.Grade == GradeMedium || r.Grade == GradeHigh || r.Grade == GradeCritical
	// High and critical risk require independent dual review; critical risk also
	// requires quality-owner approval and an explicit disposition.
	r.Preconditions = []RiskPrecondition{{Name: "deviation_note", Required: required, Satisfied: b.Transport.DeviationNote != ""}, {Name: "disposition", Required: r.Grade == GradeCritical, Satisfied: b.Transport.Disposition != ""}, {Name: "quality_approval", Required: r.Grade == GradeCritical, Satisfied: b.Transport.QualityApproved}, {Name: "dual_review", Required: r.Grade == GradeHigh || r.Grade == GradeCritical, Satisfied: b.Transport.DualReviewed}}
	for i := range r.Preconditions {
		if !r.Preconditions[i].Required {
			r.Preconditions[i].Satisfied = true
		}
	}
	return r
}
func CanCreatePlan(b IceCoreBatch) error {
	if b.Status != StatusAwaitingPlan {
		return fmt.Errorf("batch not ready for plan")
	}
	if b.Risk == nil || len(b.Risk.Missing) > 0 {
		return fmt.Errorf("risk assessment incomplete")
	}
	for _, p := range b.Risk.Preconditions {
		if p.Required && !p.Satisfied {
			return fmt.Errorf("risk precondition %s not satisfied", p.Name)
		}
	}
	return nil
}
func CanObserve(b IceCoreBatch) error {
	if b.Plan == nil || b.Plan.ApprovalStatus != "approved" {
		return fmt.Errorf("approved plan required")
	}
	if b.Status != StatusThawing {
		return fmt.Errorf("batch not thawing")
	}
	return nil
}
