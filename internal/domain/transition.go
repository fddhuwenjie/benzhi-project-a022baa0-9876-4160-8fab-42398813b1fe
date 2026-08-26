package domain

import "fmt"

func TransitionAllowed(from, to BatchStatus) bool {
	allowed := map[BatchStatus]map[BatchStatus]bool{StatusDraft: {StatusAwaitingPlan: true}, StatusAwaitingPlan: {StatusPlanPending: true}, StatusPlanPending: {StatusThawing: true, StatusAwaitingPlan: true}, StatusThawing: {StatusReview: true}, StatusReview: {StatusRemediation: true, StatusSealed: true}, StatusRemediation: {StatusReview: true, StatusSealed: true}}
	return allowed[from][to]
}
func RequireTransition(from, to BatchStatus) error {
	if !TransitionAllowed(from, to) {
		return fmt.Errorf("transition %s -> %s denied", from, to)
	}
	return nil
}
