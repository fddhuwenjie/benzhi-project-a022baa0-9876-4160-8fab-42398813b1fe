package domain

import (
	"fmt"
	"time"
)

func TemperatureExceeded(min, max, allowedMin, allowedMax float64) bool {
	return min < allowedMin || max > allowedMax
}
func DeviationSeverity(t TransportDeviation) string {
	if !t.Exceeded {
		return "none"
	}
	if t.LowExceededMinutes+t.HighExceededMinutes >= 120 || t.ExposureDegreeMinutes >= 180 {
		return "severe"
	}
	d := t.AllowedMinC - t.MinTemperatureC
	if x := t.MaxTemperatureC - t.AllowedMaxC; x > d {
		d = x
	}
	if d > 10 {
		return "severe"
	}
	if d > 3 {
		return "moderate"
	}
	return "minor"
}

func SummarizeTransport(t *TransportDeviation) error {
	if t == nil {
		return fmt.Errorf("transport required")
	}
	if t.AllowedMinC < -60 || t.AllowedMaxC > 60 || t.AllowedMinC >= t.AllowedMaxC {
		return fmt.Errorf("invalid allowed temperature range")
	}
	if len(t.Intervals) == 0 {
		t.Exceeded = TemperatureExceeded(t.MinTemperatureC, t.MaxTemperatureC, t.AllowedMinC, t.AllowedMaxC)
		t.MaxDeviationC = 0
		if t.MinTemperatureC < t.AllowedMinC {
			t.MaxDeviationC = t.AllowedMinC - t.MinTemperatureC
		}
		if d := t.MaxTemperatureC - t.AllowedMaxC; d > t.MaxDeviationC {
			t.MaxDeviationC = d
		}
		t.Severity = DeviationSeverity(*t)
		return nil
	}
	ints := t.Intervals
	var prev time.Time
	for _, in := range ints {
		if in.End.IsZero() || in.Start.IsZero() || !in.End.After(in.Start) {
			return fmt.Errorf("temperature_intervals duration must be positive")
		}
		if !prev.IsZero() && in.Start.Before(prev) {
			return fmt.Errorf("temperature_intervals overlap or unsorted")
		}
		if in.MinTemperatureC < -60 || in.MaxTemperatureC > 60 || in.MinTemperatureC > in.MaxTemperatureC {
			return fmt.Errorf("temperature interval out of range")
		}
		prev = in.End
		mins := in.End.Sub(in.Start).Minutes()
		low := t.AllowedMinC - in.MinTemperatureC
		if low > 0 {
			t.LowExceededMinutes += mins
		}
		high := in.MaxTemperatureC - t.AllowedMaxC
		if high > 0 {
			t.HighExceededMinutes += mins
		}
		if low > t.MaxDeviationC {
			t.MaxDeviationC = low
		}
		if high > t.MaxDeviationC {
			t.MaxDeviationC = high
		}
		if low > 0 {
			t.ExposureDegreeMinutes += low * mins
		}
		if high > 0 {
			t.ExposureDegreeMinutes += high * mins
		}
	}
	t.Exceeded = t.LowExceededMinutes > 0 || t.HighExceededMinutes > 0
	t.Severity = DeviationSeverity(*t)
	return nil
}
