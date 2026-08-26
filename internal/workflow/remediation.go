package workflow

import "iceguard/internal/domain"

func (s *Service) SubmitRetest(id, result string, expected int, requestID string) (domain.IceCoreBatch, error) {
	r := domain.ReleaseReview{RetestResult: result, Decision: "pass", ReviewerID: "retest"}
	return s.Review(id, r, expected, requestID)
}
