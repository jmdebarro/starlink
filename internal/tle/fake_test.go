package tle

import (
	"strings"
	"testing"
	"time"
)

func TestFake(t *testing.T) {
	data := Fake(time.Date(2026, 11, 15, 12, 30, 0, 0, time.UTC))

	// Every line must have the exact TLE length and a correct checksum.
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line[0] != '1' && line[0] != '2' {
			continue // name line
		}
		if !validLine(line, line[0]) {
			t.Fatalf("invalid TLE line (len %d): %q", len(line), line)
		}
	}

	// Every satellite must survive Parse, and the count must match the table.
	records := Parse(data)
	if want := FakeCount(); len(records) != want {
		t.Fatalf("parsed %d records, want %d", len(records), want)
	}
	if want := 5748; FakeCount() != want {
		t.Fatalf("FakeCount = %d, want %d", FakeCount(), want)
	}
	if records[0].Name != "FAKE-0001" || records[0].NoradID != 90000 {
		t.Errorf("first record = %+v", records[0])
	}
}

func TestParseSkipsMalformed(t *testing.T) {
	good := string(Fake(time.Now()))
	lines := strings.Split(good, "\n")
	// Entry 1 with a broken checksum, entry 2 untouched but with CRLF and
	// trailing spaces, entry 3 truncated.
	wrong := string(rune('0' + (checksum(lines[1][:68])+1)%10))
	bad := lines[0] + "\n" + lines[1][:68] + wrong + "\n" + lines[2] + "\n" +
		lines[3] + "\r\n" + lines[4] + "   \r\n" + lines[5] + " \r\n" +
		lines[6] + "\n" + lines[7][:40] + "\n" + lines[8] + "\n"
	got := Parse([]byte(bad))
	if len(got) != 1 || got[0].Name != "FAKE-0002" {
		t.Fatalf("got %+v, want only FAKE-0002", got)
	}
}
