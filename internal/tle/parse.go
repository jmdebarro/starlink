package tle

import (
	"strconv"
	"strings"
)

// Record is one satellite from a TLE file.
type Record struct {
	Name    string
	Line1   string
	Line2   string
	NoradID int
}

// lineLen is the fixed length of a valid TLE line.
const lineLen = 69

// Parse reads TLE text in the usual "3-line" layout (a name line followed by
// line 1 and line 2). Malformed entries are skipped rather than reported:
// one bad satellite should not stop the other thousands.
func Parse(data []byte) []Record {
	// Normalise line endings and strip trailing spaces from every line.
	lines := strings.Split(string(data), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t\r")
	}

	var records []Record
	for i := 0; i+1 < len(lines); i++ {
		l1, l2 := lines[i], lines[i+1]
		if !validLine(l1, '1') || !validLine(l2, '2') {
			continue
		}
		// Both lines must describe the same satellite.
		if l1[2:7] != l2[2:7] {
			continue
		}
		id, err := strconv.Atoi(strings.TrimSpace(l1[2:7]))
		if err != nil {
			continue
		}

		// The name is the line just before line 1 (if there is one).
		name := ""
		if i > 0 && !strings.HasPrefix(lines[i-1], "1 ") && !strings.HasPrefix(lines[i-1], "2 ") {
			name = strings.TrimSpace(strings.TrimPrefix(lines[i-1], "0 "))
		}
		if name == "" {
			name = "NORAD " + strconv.Itoa(id)
		}

		records = append(records, Record{Name: name, Line1: l1, Line2: l2, NoradID: id})
		i++ // we used line 2 as well, so skip over it
	}
	return records
}

// validLine checks the length, the leading line number and the checksum.
func validLine(line string, number byte) bool {
	if len(line) != lineLen || line[0] != number || line[1] != ' ' {
		return false
	}
	return checksum(line[:lineLen-1]) == int(line[lineLen-1]-'0')
}

// checksum is the TLE checksum: add up all the digits, count each '-' as 1,
// ignore everything else, and keep the last digit of the total.
func checksum(s string) int {
	sum := 0
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9':
			sum += int(c - '0')
		case c == '-':
			sum++
		}
	}
	return sum % 10
}

// Info holds the extra facts the web page shows when you click a satellite.
// All of them are read straight from the two TLE lines.
type Info struct {
	LaunchID    string  // international designator, e.g. "2019-074A" (launch year, launch number, piece)
	Inclination float64 // degrees between the orbit and the equator
	PeriodMin   float64 // minutes per orbit
}

// Info pulls the launch ID, inclination and period out of the record's TLE
// lines. TLEs are fixed-width, so each value sits in known columns. Fields
// that don't parse are left as zero values instead of failing.
func (r Record) Info() Info {
	var info Info

	// Line 1, columns 10-17: "19074A  " = launched 2019, launch 074, piece A.
	if id := strings.TrimSpace(r.Line1[9:17]); len(id) >= 5 {
		if yy, err := strconv.Atoi(id[:2]); err == nil {
			year := 2000 + yy
			if yy >= 57 { // the space age started in 1957
				year = 1900 + yy
			}
			info.LaunchID = strconv.Itoa(year) + "-" + id[2:]
		}
	}

	// Line 2, columns 9-16: inclination in degrees.
	info.Inclination, _ = strconv.ParseFloat(strings.TrimSpace(r.Line2[8:16]), 64)

	// Line 2, columns 53-63: mean motion in revolutions per day.
	if revs, err := strconv.ParseFloat(strings.TrimSpace(r.Line2[52:63]), 64); err == nil && revs > 0 {
		info.PeriodMin = 24 * 60 / revs
	}
	return info
}
