// Package orbit turns a TLE into a position (latitude, longitude, altitude)
// using the SGP4 model from the go-satellite library.
package orbit

import (
	"fmt"
	"math"
	"time"

	satellite "github.com/joshuaferrara/go-satellite"
)

// Sat is one satellite that is ready to be propagated.
type Sat struct {
	sat satellite.Satellite
}

// New parses the two TLE lines and initialises SGP4.
//
// go-satellite slices the lines by fixed column numbers, so a short or odd
// line makes it panic. A panic would take down the whole server, so we turn
// any panic into an ordinary error with recover.
func New(line1, line2 string) (s *Sat, err error) {
	// "defer" runs this function when New returns, even during a panic.
	// recover() stops the panic and gives us its value. Because err is a
	// *named* result, we can set it from inside the deferred closure.
	defer func() {
		if r := recover(); r != nil {
			s, err = nil, fmt.Errorf("bad TLE: %v", r)
		}
	}()

	// The library does not check the lines itself, so do the basics here.
	if len(line1) != 69 || len(line2) != 69 || line1[0] != '1' || line2[0] != '2' {
		return nil, fmt.Errorf("TLE lines must be 69 characters starting with 1 and 2")
	}

	sat := satellite.TLEToSat(line1, line2, satellite.GravityWGS84)
	if sat.Error != 0 {
		return nil, fmt.Errorf("sgp4 init failed: %s", sat.ErrorStr)
	}
	return &Sat{sat: sat}, nil
}

// At returns the satellite's latitude and longitude in degrees and its
// altitude in km above the WGS84 ellipsoid at time t.
//
// The library's Propagate hides its own error flag (it works on a copy of
// the satellite), so we detect failure from the result instead: a decayed or
// broken orbit gives NaN, a zero vector or an absurd altitude.
func (s *Sat) At(t time.Time) (lat, lon, alt float64, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("sgp4 panic: %v", r)
		}
	}()

	t = t.UTC()
	pos, _ := satellite.Propagate(s.sat, t.Year(), int(t.Month()), t.Day(),
		t.Hour(), t.Minute(), t.Second())
	if math.IsNaN(pos.X+pos.Y+pos.Z) || (pos.X == 0 && pos.Y == 0 && pos.Z == 0) {
		return 0, 0, 0, fmt.Errorf("propagation failed (decayed or invalid orbit)")
	}

	// GMST (Greenwich sidereal time) tells us how far the Earth has turned,
	// which converts the inertial (ECI) position to latitude/longitude.
	gmst := satellite.GSTimeFromDate(t.Year(), int(t.Month()), t.Day(),
		t.Hour(), t.Minute(), t.Second())
	alt, _, ll := satellite.ECIToLLA(pos, gmst)
	if math.IsNaN(alt) || alt < 100 || alt > 100000 {
		return 0, 0, 0, fmt.Errorf("implausible altitude %.0f km", alt)
	}

	lat = ll.Latitude * 180 / math.Pi
	// Longitude from the library is not wrapped; bring it into -180..180.
	lon = math.Remainder(ll.Longitude, 2*math.Pi) * 180 / math.Pi
	return lat, lon, alt, nil
}
