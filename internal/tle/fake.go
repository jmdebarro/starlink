package tle

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// shell describes one group of orbits, like a real Starlink shell.
type shell struct {
	inclination float64 // degrees
	altitudeKm  float64
	planes      int
	perPlane    int
}

// shells matches the table in docs/design.md.
var shells = []shell{
	{53.0, 550, 72, 22},
	{53.2, 540, 72, 22},
	{70.0, 570, 36, 20},
	{97.6, 560, 6, 58},
	{43.0, 530, 28, 54},
}

// FakeCount is how many satellites Fake generates.
func FakeCount() int {
	n := 0
	for _, s := range shells {
		n += s.planes * s.perPlane
	}
	return n
}

// Fake generates syntactically valid TLEs (3-line format) for a made-up
// constellation, with the given epoch. It lets us develop without touching
// CelesTrak.
func Fake(epoch time.Time) []byte {
	epoch = epoch.UTC()
	// TLE epoch = two-digit year + day of year with a fraction (Jan 1 = day 1).
	year := epoch.Year() % 100
	secondsToday := float64(epoch.Hour()*3600+epoch.Minute()*60+epoch.Second()) +
		float64(epoch.Nanosecond())/1e9
	day := float64(epoch.YearDay()) + secondsToday/86400

	var b strings.Builder
	n := 0
	for _, s := range shells {
		meanMotion := meanMotionFor(s.altitudeKm, s.inclination)
		for plane := 0; plane < s.planes; plane++ {
			raan := 360 * float64(plane) / float64(s.planes)
			// Shift each plane a little so satellites don't line up in rows.
			phase := math.Mod(float64(plane)*360/float64(s.perPlane)*0.37, 360)
			for k := 0; k < s.perPlane; k++ {
				anomaly := math.Mod(360*float64(k)/float64(s.perPlane)+phase, 360)
				n++
				id := 90000 + n - 1

				// Line 1 is built column by column; see the TLE format spec.
				l1 := fmt.Sprintf("1 %05dU 24001A   %02d%012.8f  .00000000  00000-0  00000-0 0  %3d",
					id, year, day, n%1000)
				l2 := fmt.Sprintf("2 %05d %8.4f %8.4f 0001000 %8.4f %8.4f %11.8f%5d",
					id, s.inclination, raan, 90.0, anomaly, meanMotion, n%100000)

				fmt.Fprintf(&b, "FAKE-%04d\n%s%d\n%s%d\n", n,
					l1, checksum(l1), l2, checksum(l2))
			}
		}
	}
	return []byte(b.String())
}

// meanMotionFor converts a circular-orbit altitude to the TLE's mean motion
// in revolutions per day.
//
// Kepler's third law gives a first guess. But the number in a TLE is a
// "Kozai" mean motion, which SGP4 converts to a slightly different orbit
// size (the Earth's bulge, J2, matters more at high inclination: up to ~20 km
// of altitude). So we repeat SGP4's own conversion and nudge the mean motion
// until the orbit comes out at the altitude we asked for.
func meanMotionFor(altitudeKm, inclinationDeg float64) float64 {
	const (
		earthRadiusKm = 6378.137
		xke           = 0.0743669161331734 // sqrt(mu) in Earth radii^1.5 per minute
		k2            = 0.5 * 0.00108262998905
	)
	targetA := (earthRadiusKm + altitudeKm) / earthRadiusKm // in Earth radii
	cosi := math.Cos(inclinationDeg * math.Pi / 180)

	n := xke / math.Pow(targetA, 1.5) // radians per minute, Kepler guess
	for i := 0; i < 20; i++ {
		// This mirrors the "initl" step of SGP4 for a circular orbit.
		a1 := math.Pow(xke/n, 2.0/3.0)
		d1 := 1.5 * k2 * (3*cosi*cosi - 1) / (a1 * a1)
		a0 := a1 * (1 - d1/3 - d1*d1 - 134.0/81.0*d1*d1*d1)
		d0 := 1.5 * k2 * (3*cosi*cosi - 1) / (a0 * a0)
		a := a0 / (1 - d0) // SGP4's own semi-major axis (no'' = n/(1+d0))
		// Too big an orbit means too slow: speed up in proportion.
		n *= math.Pow(a/targetA, 1.5)
	}
	return n * 1440 / (2 * math.Pi) // rad/min -> revolutions/day
}
