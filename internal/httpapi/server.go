package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"iceguard/internal/domain"
	"iceguard/internal/workflow"
	"net/http"
	"reflect"
	"strconv"
	"strings"
)

type Server struct{ wf *workflow.Service }

func New(wf *workflow.Service) *Server { return &Server{wf: wf} }
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/batches", s.Batches)
	mux.HandleFunc("/v1/batches/", s.BatchSubresource)
	mux.HandleFunc("/healthz", s.Health)
	return withRequestID(mux)
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func decode(r *http.Request, v any) error { return json.NewDecoder(r.Body).Decode(v) }
func reqID(r *http.Request) string {
	v := r.Header.Get("X-Request-ID")
	if v == "" {
		v = r.Header.Get("Idempotency-Key")
	}
	return v
}
func expected(r *http.Request) int {
	var n int
	_ = json.NewDecoder(strings.NewReader(r.Header.Get("X-Expected-Revision"))).Decode(&n)
	return n
}
func (s *Server) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]string{"status": "ok"})
}
func (s *Server) Batches(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		o := workflow.BatchListOptions{DrillSite: r.URL.Query().Get("drill_site"), Limit: 0, Cursor: r.URL.Query().Get("cursor")}
		if v := r.URL.Query().Get("status"); v != "" {
			st := domain.BatchStatus(v)
			switch st {
			case domain.StatusDraft, domain.StatusAwaitingPlan, domain.StatusPlanPending, domain.StatusThawing, domain.StatusReview, domain.StatusRemediation, domain.StatusSealed:
				o.Status = &st
			default:
				writeJSON(w, 400, map[string]string{"error": "invalid status"})
				return
			}
		}
		if v := r.URL.Query().Get("integrity_grade"); v != "" {
			g := domain.IntegrityGrade(v)
			switch g {
			case domain.GradeLow, domain.GradeMedium, domain.GradeHigh, domain.GradeCritical:
				o.Integrity = &g
			default:
				writeJSON(w, 400, map[string]string{"error": "invalid integrity_grade"})
				return
			}
		}
		unmetParam := r.URL.Query().Get("has_unmet_preconditions")
		if unmetParam == "" {
			unmetParam = r.URL.Query().Get("unmet_preconditions")
		}
		if v := unmetParam; v != "" {
			x, e := strconv.ParseBool(v)
			if e != nil {
				writeJSON(w, 400, map[string]string{"error": "invalid has_unmet_preconditions"})
				return
			}
			o.HasUnmet = &x
		}
		if v := r.URL.Query().Get("limit"); v != "" {
			n, e := strconv.Atoi(v)
			if e != nil {
				writeJSON(w, 400, map[string]string{"error": "invalid limit"})
				return
			}
			o.Limit = n
		}
		out, e := s.wf.QueryBatches(o)
		if e != nil {
			writeJSON(w, 400, map[string]string{"error": e.Error()})
			return
		}
		writeJSON(w, 200, out)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		return
	}
	var b domain.IceCoreBatch
	if err := decode(r, &b); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	out, err := s.wf.CreateBatch(b, reqID(r))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 201, out)
}
func (s *Server) BatchSubresource(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		writeJSON(w, 404, nil)
		return
	}
	id := parts[2]
	if len(parts) == 3 && r.Method == http.MethodGet {
		b, e := s.wf.GetBatch(id)
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 200, b)
		return
	}
	if len(parts) < 4 {
		writeJSON(w, 404, nil)
		return
	}
	switch parts[3] {
	case "transport", "transport-deviation", "deviations":
		s.transport(w, r, id)
	case "thaw-plan":
		if len(parts) > 4 && parts[4] == "approve" {
			s.approve(w, r, id)
		} else {
			s.plan(w, r, id)
		}
	case "approve":
		s.approve(w, r, id)
	case "observations", "thaw-observations":
		if len(parts) > 5 && parts[5] == "correction" {
			s.correction(w, r, id, parts[4])
		} else {
			s.observation(w, r, id)
		}
	case "risk", "risk-summary":
		if !methodGuard(w, r, http.MethodGet) {
			return
		}
		v, e := s.wf.RiskSummary(id)
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 200, v)
	case "remediation":
		if len(parts) > 5 && parts[5] == "verify" {
			s.verifyRemediation(w, r, id, parts[4])
		} else if len(parts) > 5 && parts[5] == "submit" {
			s.submitRemediation(w, r, id, parts[4])
		} else {
			s.remediation(w, r, id)
		}
	case "release-review":
		s.review(w, r, id)
	case "release-report":
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		report, e := s.wf.ReleaseReport(id)
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, http.StatusOK, report)
	default:
		writeJSON(w, 404, nil)
	}
}
func (s *Server) transport(w http.ResponseWriter, r *http.Request, id string) {
	if !methodGuard(w, r, http.MethodPost) {
		return
	}
	var raw map[string]json.RawMessage
	if err := decode(r, &raw); err != nil {
		writeErr(w, err)
		return
	}
	var t domain.TransportDeviation
	b0, _ := s.wf.GetBatch(id)
	expRev := expected(r)
	if rawRev, ok := raw["expected_revision"]; ok && expRev == 0 {
		_ = json.Unmarshal(rawRev, &expRev)
	}
	if b0.Transport != nil {
		var incoming domain.TransportDeviation
		_ = json.Unmarshal(mustJSON(raw), &incoming)
		for _, k := range []string{"summary", "min_temperature_c", "max_temperature_c", "allowed_min_c", "allowed_max_c", "temperature_intervals", "deviation_note", "exceeded"} {
			if _, ok := raw[k]; !ok {
				continue
			}
			different := false
			switch k {
			case "summary":
				different = incoming.Summary != b0.Transport.Summary
			case "min_temperature_c":
				different = incoming.MinTemperatureC != b0.Transport.MinTemperatureC
			case "max_temperature_c":
				different = incoming.MaxTemperatureC != b0.Transport.MaxTemperatureC
			case "allowed_min_c":
				different = incoming.AllowedMinC != b0.Transport.AllowedMinC
			case "allowed_max_c":
				different = incoming.AllowedMaxC != b0.Transport.AllowedMaxC
			case "temperature_intervals":
				different = !reflect.DeepEqual(incoming.Intervals, b0.Transport.Intervals)
			case "deviation_note":
				different = incoming.DeviationNote != b0.Transport.DeviationNote
			case "exceeded":
				different = incoming.Exceeded != b0.Transport.Exceeded
			}
			if different {
				writeErr(w, domain.ErrInvalidState)
				return
			}
		}
		var in struct {
			Disposition     *string `json:"disposition"`
			QualityApproved *bool   `json:"quality_approved"`
			DualReviewed    *bool   `json:"dual_reviewed"`
			Operator        string  `json:"operator"`
			Reason          string  `json:"reason"`
		}
		buf, _ := json.Marshal(raw)
		_ = json.Unmarshal(buf, &in)
		b, e := s.wf.ReviewTransportDecision(id, in.Disposition, in.QualityApproved, in.DualReviewed, in.Operator, in.Reason, expRev, reqID(r))
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 200, b)
		return
	}
	buf, _ := json.Marshal(raw)
	if err := json.Unmarshal(buf, &t); err != nil {
		writeErr(w, err)
		return
	}
	b, e := s.wf.RegisterTransport(id, t, expRev, reqID(r))
	if e != nil {
		writeErr(w, e)
		return
	}
	writeJSON(w, 200, b)
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
func (s *Server) plan(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method == http.MethodGet {
		v := 0
		if q := r.URL.Query().Get("version"); q != "" {
			n, e := strconv.Atoi(q)
			if e != nil || n <= 0 {
				writeJSON(w, 400, map[string]string{"error": "invalid version"})
				return
			}
			v = n
		}
		out, e := s.wf.ThawPlanSummary(id, v)
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 200, out)
		return
	}
	if !methodGuard(w, r, http.MethodPost) {
		return
	}
	var p domain.ThawPlan
	if err := decode(r, &p); err != nil {
		writeErr(w, err)
		return
	}
	b, e := s.wf.CreatePlan(id, p, expected(r), reqID(r))
	if e != nil {
		writeErr(w, e)
		return
	}
	writeJSON(w, 201, b)
}
func (s *Server) approve(w http.ResponseWriter, r *http.Request, id string) {
	if !methodGuard(w, r, http.MethodPost) {
		return
	}
	var in struct {
		ApproverID      string `json:"approver_id"`
		Approve         bool   `json:"approve"`
		RejectionReason string `json:"rejection_reason"`
		PlanVersion     int    `json:"plan_version"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if in.PlanVersion <= 0 {
		writeErr(w, fmt.Errorf("plan_version required"))
		return
	}
	var b domain.IceCoreBatch
	var e error
	b, e = s.wf.ApprovePlanVersion(id, in.ApproverID, in.PlanVersion, expected(r), in.Approve, in.RejectionReason, reqID(r))
	if e != nil {
		writeErr(w, e)
		return
	}
	writeJSON(w, 200, b)
}
func (s *Server) observation(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method == http.MethodGet {
		anomaly := r.URL.Query().Get("anomaly")
		si := 0
		if q := r.URL.Query().Get("stage_index"); q != "" {
			n, e := strconv.Atoi(q)
			if e != nil || n <= 0 {
				writeJSON(w, 400, map[string]string{"error": "invalid stage_index"})
				return
			}
			si = n
		}
		out, e := s.wf.ObservationQueryFiltered(id, anomaly, si)
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 200, out)
		return
	}
	if !methodGuard(w, r, http.MethodPost) {
		return
	}
	var o domain.ThawObservation
	if err := decode(r, &o); err != nil {
		writeErr(w, err)
		return
	}
	if o.HoldMinutes <= 0 {
		writeErr(w, fmt.Errorf("hold_minutes required"))
		return
	}
	b, e := s.wf.AddObservation(id, o, expected(r), reqID(r))
	if e != nil {
		writeErr(w, e)
		return
	}
	writeJSON(w, 200, b)
}
func (s *Server) correction(w http.ResponseWriter, r *http.Request, id, obsID string) {
	if !methodGuard(w, r, http.MethodPost) {
		return
	}
	var in struct {
		ObservedTemperatureC float64 `json:"observed_temperature_c"`
		MeltwaterVolumeML    float64 `json:"meltwater_volume_ml"`
		AppearanceNote       string  `json:"appearance_note"`
		DeviationCode        string  `json:"deviation_code"`
		Reason               string  `json:"reason"`
		Operator             string  `json:"operator"`
	}
	if e := decode(r, &in); e != nil {
		writeErr(w, e)
		return
	}
	b, e := s.wf.CorrectObservation(id, obsID, domain.ThawObservation{ObservedTemperatureC: in.ObservedTemperatureC, MeltwaterVolumeML: in.MeltwaterVolumeML, AppearanceNote: in.AppearanceNote, DeviationCode: in.DeviationCode}, in.Reason, in.Operator, expected(r), reqID(r))
	if e != nil {
		writeErr(w, e)
		return
	}
	writeJSON(w, 200, b)
}
func (s *Server) remediation(w http.ResponseWriter, r *http.Request, id string) {
	if !methodGuard(w, r, http.MethodPost) {
		return
	}
	var in struct {
		Tasks []domain.RemediationTask `json:"tasks"`
	}
	if e := decode(r, &in); e != nil {
		writeErr(w, e)
		return
	}
	b, e := s.wf.AddRemediation(id, in.Tasks, expected(r), reqID(r))
	if e != nil {
		writeErr(w, e)
		return
	}
	writeJSON(w, 200, b)
}
func (s *Server) submitRemediation(w http.ResponseWriter, r *http.Request, id, taskID string) {
	if !methodGuard(w, r, http.MethodPost) {
		return
	}
	var t domain.RemediationTask
	if e := decode(r, &t); e != nil {
		writeErr(w, e)
		return
	}
	t.ID = taskID
	b, e := s.wf.SubmitRemediation(id, t, expected(r), reqID(r))
	if e != nil {
		writeErr(w, e)
		return
	}
	writeJSON(w, 200, b)
}
func (s *Server) verifyRemediation(w http.ResponseWriter, r *http.Request, id, taskID string) {
	if !methodGuard(w, r, http.MethodPost) {
		return
	}
	var in struct {
		ReviewerID string `json:"reviewer_id"`
		Decision   string `json:"decision"`
		Note       string `json:"note"`
	}
	if e := decode(r, &in); e != nil {
		writeErr(w, e)
		return
	}
	b, e := s.wf.VerifyRemediation(id, taskID, in.ReviewerID, in.Decision, in.Note, expected(r), reqID(r))
	if e != nil {
		writeErr(w, e)
		return
	}
	writeJSON(w, 200, b)
}
func (s *Server) review(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method == http.MethodGet {
		reviewer := r.URL.Query().Get("reviewer_id")
		if reviewer == "" {
			reviewer = r.Header.Get("X-Actor-ID")
		}
		out, e := s.wf.ReleasePrecheck(id, reviewer)
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 200, out)
		return
	}
	if !methodGuard(w, r, http.MethodPost) {
		return
	}
	var rv domain.ReleaseReview
	if err := decode(r, &rv); err != nil {
		writeErr(w, err)
		return
	}
	b, e := s.wf.Review(id, rv, expected(r), reqID(r))
	if e != nil {
		writeErr(w, e)
		return
	}
	writeJSON(w, 200, b)
}
func writeErr(w http.ResponseWriter, e error) {
	status := 400
	if errors.Is(e, domain.ErrNotFound) {
		status = 404
	}
	if errors.Is(e, domain.ErrConflict) {
		status = 409
	}
	if errors.Is(e, domain.ErrInvalidState) {
		status = 422
	}
	if errors.Is(e, domain.ErrIdempotent) {
		status = 200
	}
	if strings.Contains(e.Error(), "sealed") || strings.Contains(e.Error(), "audit") || strings.Contains(e.Error(), "verification") {
		status = 422
	}
	writeJSON(w, status, map[string]string{"error": e.Error()})
}
