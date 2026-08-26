package domain

import "strings"

func ObservationAnomaly(o ThawObservation) bool {
	state := strings.ToLower(o.SampleState + " " + o.AppearanceNote)
	return o.DeviationCode != "" || o.MeltwaterVolumeML > 50 || o.ObservedTemperatureC > 8 || strings.Contains(state, "abnormal") || strings.Contains(state, "异常")
}

func EffectiveObservation(o ThawObservation) ThawObservation {
	if len(o.Corrections) == 0 {
		return o
	}
	v := o.Corrections[len(o.Corrections)-1].Value
	v.ID = o.ID
	v.BatchID = o.BatchID
	v.PlanID = o.PlanID
	v.StageIndex = o.StageIndex
	v.Corrections = o.Corrections
	return v
}
