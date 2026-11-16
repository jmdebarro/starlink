package orbit

import (
	"math"
	"testing"
	"time"

	"github.com/jmdebarro/starlink/internal/tle"
)

func TestFakeSatellitesLookRight(t *testing.T) {
	now := time.Now().UTC()
	records := tle.Parse(tle.Fake(now))

	// First satellite of each shell: index = sum of earlier shells' counts.
	// Shells have 1584, 1584, 720, 348, 1512 satellites.
	cases := []struct {
		index       int
		inclination float64
		altitude    float64
	}{
		{0, 53.0, 550},
		{1584, 53.2, 540},
		{3168, 70.0, 570},
		{3888, 97.6, 560},
		{4236, 43.0, 530},
	}
	for _, c := range cases {
		sat, err := New(records[c.index].Line1, records[c.index].Line2)
		if err != nil {
			t.Fatal(err)
		}
		// Check a few times over one orbit.
		for m := 0; m < 100; m += 10 {
			lat, lon, alt, err := sat.At(now.Add(time.Duration(m) * time.Minute))
			if err != nil {
				t.Fatal(err)
			}
			// Altitude above the (flattened) Earth is lowest over the equator
			// and rises by up to ~25 km towards the poles, so allow for that.
			if alt < c.altitude-15 || alt > c.altitude+40 {
				t.Errorf("shell %.1f deg: altitude %.1f km, want ~%.0f", c.inclination, alt, c.altitude)
			}
			// Highest latitude reached is the inclination (or 180 - it).
			maxLat := math.Min(c.inclination, 180-c.inclination)
			if math.Abs(lat) > maxLat+0.5 {
				t.Errorf("lat %.2f exceeds %.1f", lat, maxLat)
			}
			if lon < -180 || lon > 180 {
				t.Errorf("lon %.2f out of range", lon)
			}
		}
	}
}

func TestBadTLEReturnsError(t *testing.T) {
	if _, err := New("1 short", "2 short"); err == nil {
		t.Error("expected an error for a truncated TLE")
	}
}
