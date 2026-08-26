package domain

import "testing"

func TestAssessIntegrity(t *testing.T) {
	g, _ := AssessIntegrity(&TransportDeviation{Exceeded: true}, nil)
	if g != GradeMedium {
		t.Fatalf("got %s", g)
	}
}
func TestStageValidation(t *testing.T) {
	if (ThawPlan{Stages: []ThawStage{{Index: 2, TargetTemperatureC: 1, HoldMinutes: 10}}}).Validate() == nil {
		t.Fatal("expected error")
	}
}
