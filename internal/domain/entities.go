package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type BatchStatus string

const (
	StatusDraft        BatchStatus = "draft"
	StatusAwaitingPlan BatchStatus = "awaiting_plan"
	StatusPlanPending  BatchStatus = "plan_pending"
	StatusThawing      BatchStatus = "thawing"
	StatusReview       BatchStatus = "review"
	StatusRemediation  BatchStatus = "remediation"
	StatusSealed       BatchStatus = "sealed"
)

type IntegrityGrade string

const (
	GradeLow      IntegrityGrade = "low"
	GradeMedium   IntegrityGrade = "medium"
	GradeHigh     IntegrityGrade = "high"
	GradeCritical IntegrityGrade = "critical"
)

type IceCoreBatch struct {
	ID                  string              `json:"id"`
	CoreCode            string              `json:"core_code"`
	DrillSite           string              `json:"drill_site"`
	DepthIntervalM      string              `json:"depth_interval_m"`
	CollectionTime      time.Time           `json:"collection_time"`
	InitialTemperatureC float64             `json:"initial_temperature_c"`
	IntegrityGrade      IntegrityGrade      `json:"integrity_grade"`
	Status              BatchStatus         `json:"status"`
	Revision            int                 `json:"revision"`
	CreatedAt           time.Time           `json:"created_at"`
	Transport           *TransportDeviation `json:"transport,omitempty"`
	Preconditions       []string            `json:"preconditions,omitempty"`
	Plan                *ThawPlan           `json:"plan,omitempty"`
	Observations        []ThawObservation   `json:"observations,omitempty"`
	Review              *ReleaseReview      `json:"review,omitempty"`
	Risk                *RiskAssessment     `json:"risk,omitempty"`
	PlanHistory         []ThawPlan          `json:"plan_history,omitempty"`
	RemediationTasks    []RemediationTask   `json:"remediation_tasks,omitempty"`
	Execution           ExecutionSummary    `json:"execution"`
}

type TemperatureInterval struct {
	Start           time.Time `json:"start"`
	End             time.Time `json:"end"`
	MinTemperatureC float64   `json:"min_temperature_c"`
	MaxTemperatureC float64   `json:"max_temperature_c"`
}
type RiskFactor struct {
	Name         string  `json:"name"`
	Value        float64 `json:"value"`
	Threshold    float64 `json:"threshold"`
	Contribution float64 `json:"contribution"`
	Triggered    bool    `json:"triggered"`
}
type RiskPrecondition struct {
	Name      string `json:"name"`
	Required  bool   `json:"required"`
	Satisfied bool   `json:"satisfied"`
}
type ExecutionSummary struct {
	CompletedStages int    `json:"completed_stages"`
	RemainingStages int    `json:"remaining_stages"`
	GateReason      string `json:"gate_reason,omitempty"`
	Paused          bool   `json:"paused"`
}
type RiskAssessment struct {
	Grade         IntegrityGrade     `json:"grade"`
	Score         float64            `json:"score"`
	Factors       []RiskFactor       `json:"factors"`
	Preconditions []RiskPrecondition `json:"preconditions"`
	Revision      int                `json:"revision"`
	EvaluatedAt   time.Time          `json:"evaluated_at"`
	Missing       []string           `json:"missing,omitempty"`
}
type RemediationTask struct {
	ID               string     `json:"id"`
	Finding          string     `json:"finding"`
	Assignee         string     `json:"assignee"`
	Action           string     `json:"action"`
	DueAt            *time.Time `json:"due_at,omitempty"`
	Status           string     `json:"status"`
	Evidence         string     `json:"evidence,omitempty"`
	SubmittedBy      string     `json:"submitted_by,omitempty"`
	VerifiedBy       string     `json:"verified_by,omitempty"`
	Verification     string     `json:"verification,omitempty"`
	VerificationNote string     `json:"verification_note,omitempty"`
	OverdueReason    string     `json:"overdue_reason,omitempty"`
}

type TransportDeviation struct {
	Summary               string                    `json:"summary"`
	MinTemperatureC       float64                   `json:"min_temperature_c"`
	MaxTemperatureC       float64                   `json:"max_temperature_c"`
	AllowedMinC           float64                   `json:"allowed_min_c"`
	AllowedMaxC           float64                   `json:"allowed_max_c"`
	Exceeded              bool                      `json:"exceeded"`
	DeviationNote         string                    `json:"deviation_note"`
	Intervals             []TemperatureInterval     `json:"temperature_intervals,omitempty"`
	LowExceededMinutes    float64                   `json:"low_exceeded_minutes"`
	HighExceededMinutes   float64                   `json:"high_exceeded_minutes"`
	MaxDeviationC         float64                   `json:"max_deviation_c"`
	ExposureDegreeMinutes float64                   `json:"exposure_degree_minutes"`
	Severity              string                    `json:"severity"`
	Disposition           string                    `json:"disposition,omitempty"`
	QualityApproved       bool                      `json:"quality_approved,omitempty"`
	DualReviewed          bool                      `json:"dual_reviewed,omitempty"`
	ReviewDecisions       []TransportReviewDecision `json:"review_decisions,omitempty"`
}
type TransportReviewDecision struct {
	Disposition     string    `json:"disposition,omitempty"`
	QualityApproved bool      `json:"quality_approved,omitempty"`
	DualReviewed    bool      `json:"dual_reviewed,omitempty"`
	Reason          string    `json:"reason"`
	Operator        string    `json:"operator"`
	ReviewedAt      time.Time `json:"reviewed_at"`
}
type ThawStage struct {
	Index              int     `json:"index"`
	TargetTemperatureC float64 `json:"target_temperature_c"`
	HoldMinutes        int     `json:"hold_minutes"`
}
type ThawPlan struct {
	ID                  string      `json:"id"`
	BatchID             string      `json:"batch_id"`
	Stages              []ThawStage `json:"stages"`
	TargetTemperatureC  float64     `json:"target_temperature_c"`
	HoldMinutes         int         `json:"hold_minutes"`
	SafetyPreconditions []string    `json:"safety_preconditions"`
	AuthorID            string      `json:"author_id"`
	ApproverID          string      `json:"approver_id"`
	ApprovalStatus      string      `json:"approval_status"`
	Revision            int         `json:"revision"`
	Version             int         `json:"version"`
	PreviousVersion     int         `json:"previous_version,omitempty"`
	ChangeNote          string      `json:"change_note,omitempty"`
	RejectionReason     string      `json:"rejection_reason,omitempty"`
}
type ThawObservation struct {
	ID                   string                  `json:"id"`
	BatchID              string                  `json:"batch_id"`
	PlanID               string                  `json:"plan_id"`
	StageIndex           int                     `json:"stage_index"`
	ObservedTemperatureC float64                 `json:"observed_temperature_c"`
	MeltwaterVolumeML    float64                 `json:"meltwater_volume_ml"`
	AppearanceNote       string                  `json:"appearance_note"`
	DeviationCode        string                  `json:"deviation_code"`
	RecordedBy           string                  `json:"recorded_by"`
	RecordedAt           time.Time               `json:"recorded_at"`
	HoldMinutes          int                     `json:"hold_minutes"`
	SampleState          string                  `json:"sample_state,omitempty"`
	Corrections          []ObservationCorrection `json:"corrections,omitempty"`
}
type ObservationCorrection struct {
	ID        string          `json:"id"`
	Value     ThawObservation `json:"value"`
	Reason    string          `json:"reason"`
	Operator  string          `json:"operator"`
	CreatedAt time.Time       `json:"created_at"`
}
type ReleaseReview struct {
	ID               string            `json:"id"`
	BatchID          string            `json:"batch_id"`
	ReviewerID       string            `json:"reviewer_id"`
	Findings         string            `json:"findings"`
	CorrectiveAction string            `json:"corrective_action"`
	RetestResult     string            `json:"retest_result"`
	Decision         string            `json:"decision"`
	ReportDigest     string            `json:"report_digest"`
	SealedAt         *time.Time        `json:"sealed_at,omitempty"`
	Tasks            []RemediationTask `json:"tasks,omitempty"`
}

func (b IceCoreBatch) Validate() error {
	if b.CoreCode == "" || b.DrillSite == "" {
		return fmt.Errorf("core_code and drill_site required")
	}
	if b.InitialTemperatureC < -60 || b.InitialTemperatureC > 5 {
		return fmt.Errorf("initial temperature out of range")
	}
	if b.DepthIntervalM == "" {
		return fmt.Errorf("depth interval required")
	}
	if _, _, err := ParseDepthInterval(b.DepthIntervalM); err != nil {
		return err
	}
	if !b.CollectionTime.IsZero() && b.CollectionTime.After(time.Now().UTC()) {
		return fmt.Errorf("collection_time must not be in the future")
	}
	return nil
}
func ParseDepthInterval(v string) (float64, float64, error) {
	nums := regexp.MustCompile(`[0-9]+(?:\.[0-9]+)?`).FindAllString(strings.TrimSpace(v), -1)
	if len(nums) != 2 {
		return 0, 0, fmt.Errorf("invalid depth interval")
	}
	a, e := strconv.ParseFloat(nums[0], 64)
	if e != nil {
		return 0, 0, fmt.Errorf("invalid depth interval")
	}
	z, e := strconv.ParseFloat(nums[1], 64)
	if e != nil || a >= z {
		return 0, 0, fmt.Errorf("depth interval must be positive and ordered")
	}
	return a, z, nil
}
func (b *IceCoreBatch) Normalize() error {
	a, z, e := ParseDepthInterval(b.DepthIntervalM)
	if e != nil {
		return e
	}
	b.CoreCode = strings.ToUpper(strings.TrimSpace(b.CoreCode))
	b.DrillSite = strings.TrimSpace(b.DrillSite)
	b.DepthIntervalM = fmt.Sprintf("%g-%g", a, z)
	return nil
}
func (p ThawPlan) Validate() error {
	if len(p.Stages) == 0 {
		return fmt.Errorf("stages required")
	}
	for i, s := range p.Stages {
		if s.Index != i+1 {
			return fmt.Errorf("stage index must be sequential")
		}
		if s.TargetTemperatureC < -5 || s.TargetTemperatureC > 20 || s.HoldMinutes <= 0 {
			return fmt.Errorf("invalid stage parameters")
		}
	}
	return nil
}
func (b IceCoreBatch) ReportDigest() string {
	if b.Review != nil {
		cp := *b.Review
		cp.ReportDigest = ""
		b.Review = &cp
	}
	v := StableSnapshot(b)
	h := sha256.Sum256(v)
	return hex.EncodeToString(h[:])
}
