package tle

import (
	"math"
	"testing"
	"time"
)

func TestInfo(t *testing.T) {
	recs := Parse(Fake(time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)))
	if len(recs) == 0 {
		t.Fatal("no records")
	}
	got := recs[0].Info() // first fake shell: 53.0°, 550 km
	if got.LaunchID != "2024-001A" {
		t.Errorf("LaunchID = %q, want 2024-001A", got.LaunchID)
	}
	if math.Abs(got.Inclination-53.0) > 0.001 {
		t.Errorf("Inclination = %v, want 53", got.Inclination)
	}
	if got.PeriodMin < 94 || got.PeriodMin > 97 { // ~95.6 min at 550 km
		t.Errorf("PeriodMin = %v, want about 95.6", got.PeriodMin)
	}
}
