package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"log"
	"math"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jmdebarro/starlink/internal/orbit"
	"github.com/jmdebarro/starlink/internal/tle"
)

// windowSeconds is the length of one position window: the API returns where
// each satellite is at the start (t0) and end (t1) of the window, and the
// browser animates in between.
const windowSeconds = 60

// retryAfterFailure is how long we wait before trying again when a load fails.
const retryAfterFailure = 10 * time.Minute

// entry is one satellite we can propagate.
type entry struct {
	id   int
	name string
	prop *orbit.Sat
}

// satSet is a complete, immutable set of satellites. We never modify one
// after building it: a reload builds a brand-new satSet and swaps it in, so
// request handlers never see a half-built set.
type satSet struct {
	sats    []entry
	source  string
	updated time.Time

	catalog payload // the /api/catalog response, built once up front
}

// payload is a JSON response in both plain and gzip-compressed form. We
// compress once and serve the same bytes to everybody.
type payload struct {
	plain []byte
	gz    []byte
}

func newPayload(plain []byte) payload {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Write(plain) // writing into a bytes.Buffer cannot fail
	zw.Close()
	return payload{plain: plain, gz: buf.Bytes()}
}

// snapshot is the computed /api/positions response for one 60 s window.
type snapshot struct {
	set *satSet // which satellite set it was computed from
	t0  int64
	payload
}

// state is everything the HTTP handlers share.
type state struct {
	cfg config

	// current holds the active satSet (nil until the first successful load).
	// atomic.Pointer lets the reload goroutine replace it while handlers read
	// it, with no locks: a Load() always returns a complete pointer.
	current atomic.Pointer[satSet]

	// snap caches the latest snapshot. snapMu makes sure only one request
	// computes a new snapshot when the window rolls over; the others wait and
	// then reuse the result.
	snap   atomic.Pointer[snapshot]
	snapMu sync.Mutex
}

// reload loads TLEs, builds a new satSet and swaps it in.
func (s *state) reload() error {
	var data []byte
	var source string
	var updated time.Time

	if s.cfg.fake {
		updated = time.Now()
		data, source = tle.Fake(updated), "fake"
	} else {
		var err error
		data, source, updated, err = tle.LoadWithTime(s.cfg.dataFile)
		if err != nil {
			return err
		}
	}

	set := buildSet(data, source, updated)
	if len(set.sats) == 0 {
		return errNoSatellites
	}
	s.current.Store(set) // the atomic swap
	log.Printf("loaded %d satellites (source: %s, tle updated: %s)",
		len(set.sats), source, updated.UTC().Format(time.RFC3339))
	return nil
}

type simpleError string

func (e simpleError) Error() string { return string(e) }

const errNoSatellites = simpleError("no usable satellites in TLE data")

// buildSet parses the TLE text and prepares each satellite for SGP4.
func buildSet(data []byte, source string, updated time.Time) *satSet {
	records := tle.Parse(data)
	set := &satSet{source: source, updated: updated}
	skipped := 0
	var rows [][]any // catalog rows, built alongside set.sats
	for _, r := range records {
		prop, err := orbit.New(r.Line1, r.Line2)
		if err != nil {
			skipped++
			continue
		}
		set.sats = append(set.sats, entry{id: r.NoradID, name: r.Name, prop: prop})

		// Catalog row: [id, "name", "launch id", inclination°, period min].
		info := r.Info()
		rows = append(rows, []any{r.NoradID, r.Name, info.LaunchID,
			round(info.Inclination, 2), round(info.PeriodMin, 1)})
	}
	if skipped > 0 {
		log.Printf("skipped %d satellites that failed SGP4 init", skipped)
	}

	// json.Marshal takes care of quoting the names.
	body, _ := json.Marshal(map[string]any{"sats": rows})
	set.catalog = newPayload(body)
	return set
}

// positions returns the snapshot for the 60 s window containing now,
// computing it only if the cached one is out of date. ok is false when no
// TLEs are loaded yet.
func (s *state) positions(now time.Time) (snap *snapshot, ok bool) {
	set := s.current.Load()
	if set == nil {
		return nil, false
	}
	t0 := now.Unix() / windowSeconds * windowSeconds // round down to a multiple of 60

	// Fast path: no lock needed when the cached snapshot is still valid.
	if c := s.snap.Load(); c != nil && c.t0 == t0 && c.set == set {
		return c, true
	}

	// Slow path. Lock so that concurrent requests don't all recompute.
	s.snapMu.Lock()
	defer s.snapMu.Unlock() // runs when the function returns, releasing the lock
	// Someone else may have finished the work while we waited for the lock.
	if c := s.snap.Load(); c != nil && c.t0 == t0 && c.set == set {
		return c, true
	}

	c := computeSnapshot(set, t0)
	s.snap.Store(c)
	return c, true
}

// computeSnapshot propagates every satellite to t0 and t0+60 and builds the
// JSON body by hand (appending to a byte slice), which is much faster than
// encoding thousands of small slices with encoding/json.
func computeSnapshot(set *satSet, t0 int64) *snapshot {
	start := time.Now()
	time0 := time.Unix(t0, 0)
	time1 := time.Unix(t0+windowSeconds, 0)

	rows := make([]byte, 0, len(set.sats)*48)
	count := 0
	for _, e := range set.sats {
		lat0, lon0, alt0, err0 := e.prop.At(time0)
		lat1, lon1, alt1, err1 := e.prop.At(time1)
		if err0 != nil || err1 != nil {
			continue // decayed or broken: skip it, never crash
		}
		if count > 0 {
			rows = append(rows, ',')
		}
		count++
		rows = append(rows, '[')
		rows = strconv.AppendInt(rows, int64(e.id), 10)
		rows = appendPos(rows, lat0, lon0, alt0)
		rows = appendPos(rows, lat1, lon1, alt1)
		rows = append(rows, ']')
	}

	var b []byte
	b = append(b, `{"source":`...)
	b = strconv.AppendQuote(b, set.source)
	b = append(b, `,"tleUpdated":"`...)
	b = set.updated.UTC().AppendFormat(b, time.RFC3339)
	b = append(b, `","t0":`...)
	b = strconv.AppendInt(b, t0, 10)
	b = append(b, `,"step":`...)
	b = strconv.AppendInt(b, windowSeconds, 10)
	b = append(b, `,"count":`...)
	b = strconv.AppendInt(b, int64(count), 10)
	b = append(b, `,"sats":[`...)
	b = append(b, rows...)
	b = append(b, "]}"...)

	snap := &snapshot{set: set, t0: t0, payload: newPayload(b)}
	log.Printf("computed positions for t0=%d: %d satellites in %s (%d bytes, %d gzipped)",
		t0, count, time.Since(start).Round(time.Millisecond), len(snap.plain), len(snap.gz))
	return snap
}

// appendPos appends ",lat,lon,alt" with lat/lon to 2 decimals, alt in whole km.
func appendPos(b []byte, lat, lon, alt float64) []byte {
	b = append(b, ',')
	b = strconv.AppendFloat(b, lat, 'f', 2, 64)
	b = append(b, ',')
	b = strconv.AppendFloat(b, lon, 'f', 2, 64)
	b = append(b, ',')
	return strconv.AppendInt(b, int64(math.Round(alt)), 10)
}

// round rounds v to the given number of decimal places (keeps JSON short).
func round(v float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(v*p) / p
}
