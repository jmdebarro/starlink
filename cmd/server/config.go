package main

import (
	"flag"
	"fmt"
	"os"
	"time"
)

// config holds every setting of the server.
type config struct {
	addr       string
	dataFile   string
	fake       bool
	webDir     string
	corsOrigin string
	refresh    time.Duration
}

// envOr returns the environment variable's value, or fallback if it is unset
// or empty.
func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

// parseConfig reads flags. Each flag's default comes from an environment
// variable (12-factor style, so a Kubernetes Deployment can set env vars).
// Because flag.Parse runs after the defaults are set, a flag on the command
// line always wins over the environment.
func parseConfig() (config, error) {
	var c config

	// FAKE=1 (or "true") turns fake mode on.
	fakeEnv := envOr("FAKE", "")
	fakeDefault := fakeEnv == "1" || fakeEnv == "true"

	// Parse the REFRESH env var up front so a typo gives a clear error.
	refreshDefault := 2 * time.Hour
	if v := os.Getenv("REFRESH"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return c, fmt.Errorf("bad REFRESH %q: %w", v, err)
		}
		refreshDefault = d
	}

	flag.StringVar(&c.addr, "addr", envOr("ADDR", ":8080"), "listen address (env ADDR)")
	flag.StringVar(&c.dataFile, "data", envOr("DATA_FILE", "data/starlink.tle"), "TLE cache file (env DATA_FILE)")
	flag.BoolVar(&c.fake, "fake", fakeDefault, "use generated fake TLEs, never touch CelesTrak (env FAKE=1)")
	flag.StringVar(&c.webDir, "web", envOr("WEB_DIR", "web"), "directory served at / ; empty disables (env WEB_DIR)")
	flag.StringVar(&c.corsOrigin, "cors-origin", envOr("CORS_ORIGIN", "*"), "Access-Control-Allow-Origin on /api/* (env CORS_ORIGIN)")
	flag.DurationVar(&c.refresh, "refresh", refreshDefault, "how often to reload TLEs (env REFRESH)")
	flag.Parse()

	if c.refresh <= 0 {
		return c, fmt.Errorf("-refresh must be positive")
	}
	return c, nil
}
