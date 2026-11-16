// Package tle downloads Starlink orbit data (TLEs) from CelesTrak and keeps
// a local copy, so we only hit CelesTrak when our copy is out of date.
package tle

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// URL is CelesTrak's current Starlink TLE list.
const URL = "https://celestrak.org/NORAD/elements/gp.php?GROUP=starlink&FORMAT=tle"

// MaxAge is how long a saved copy counts as fresh. CelesTrak updates about
// every 2 hours and blocks repeat downloads before then.
const MaxAge = 2 * time.Hour

// Load returns the TLE data and where it came from ("cache" or "download").
//
//  1. If the file at path is younger than MaxAge, read it from disk.
//  2. Otherwise download a fresh copy and save it to path.
//  3. If the download fails but an older copy exists, use that instead.
func Load(path string) ([]byte, string, error) {
	data, source, _, err := LoadWithTime(path)
	return data, source, err
}

// LoadWithTime is Load plus the moment the data was last downloaded (for a
// cached file, that is the file's modification time). The server shows this
// to users as "TLE updated".
func LoadWithTime(path string) ([]byte, string, time.Time, error) {
	info, statErr := os.Stat(path)
	if statErr == nil && time.Since(info.ModTime()) < MaxAge {
		data, err := os.ReadFile(path)
		return data, "cache", info.ModTime(), err
	}

	data, err := download()
	if err == nil {
		if err := save(path, data); err != nil {
			return nil, "", time.Time{}, fmt.Errorf("saving %s: %w", path, err)
		}
		return data, "download", time.Now(), nil
	}

	// Download failed: fall back to an older saved copy if we have one.
	if statErr == nil {
		fmt.Fprintf(os.Stderr, "warning: %v; using saved copy from %s\n",
			err, info.ModTime().Format(time.DateTime))
		data, readErr := os.ReadFile(path)
		return data, "cache (stale)", info.ModTime(), readErr
	}
	return nil, "", time.Time{}, err
}

// download fetches the TLE list and fails on any non-200 response, so an
// error page is never mistaken for data.
func download() ([]byte, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(URL)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading body failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("CelesTrak returned %s: %s", resp.Status, body)
	}
	return body, nil
}

// save writes to a temporary file first and then renames it, so a crash
// mid-write never leaves a half-written file behind.
func save(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
