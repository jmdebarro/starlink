// Command server serves live Starlink positions as JSON, plus the web page.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// Logs go to stdout (the log package defaults to stderr); Kubernetes
	// collects whatever the container prints.
	log.SetOutput(os.Stdout)

	cfg, err := parseConfig()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("starting: addr=%s fake=%v data=%s web=%q cors=%s refresh=%s",
		cfg.addr, cfg.fake, cfg.dataFile, cfg.webDir, cfg.corsOrigin, cfg.refresh)

	// A context is a "cancel" signal we can pass around. NotifyContext cancels
	// it when the process gets SIGINT (Ctrl-C) or SIGTERM (sent by Kubernetes).
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	st := &state{cfg: cfg}

	// First load. If it fails we keep going: /healthz says 503 and the
	// refresh loop below keeps retrying.
	if err := st.reload(); err != nil {
		log.Printf("initial load failed (will retry in %s): %v", retryAfterFailure, err)
	}

	// "go" starts a goroutine: a function running concurrently with main.
	go st.refreshLoop(ctx)

	srv := &http.Server{
		Addr:              cfg.addr,
		Handler:           st.routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// ListenAndServe blocks, so run it in a goroutine and report its result
	// on a channel. main waits on the channel and on ctx at the same time.
	serverErr := make(chan error, 1)
	go func() { serverErr <- srv.ListenAndServe() }()

	select {
	case err := <-serverErr:
		log.Fatalf("server failed: %v", err)
	case <-ctx.Done():
		log.Print("shutting down")
	}

	// Give in-flight requests a few seconds to finish.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("shutdown error: %v", err)
	}
	log.Print("bye")
}

// refreshLoop reloads the TLEs every cfg.refresh. After a failed load it
// retries sooner (every 10 minutes) until it works.
func (s *state) refreshLoop(ctx context.Context) {
	wait := s.nextWait()
	for {
		log.Printf("next reload in %s", wait.Round(time.Second))
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return // shutting down
		}

		if err := s.reload(); err != nil {
			log.Printf("reload failed: %v", err)
		}
		wait = s.nextWait()
	}
}

// nextWait decides how long to sleep before the next reload, based on how old
// the data we are serving is (not on when the last reload ran). That way a
// restart doesn't reset the 2 h clock, and a stale fallback is retried soon.
func (s *state) nextWait() time.Duration {
	set := s.current.Load()
	switch {
	case set == nil, set.source == "cache (stale)":
		// Nothing loaded, or the download failed and we're serving an old
		// copy: try again soon.
		return retryAfterFailure
	case set.source == "fake":
		return s.cfg.refresh
	}
	// Reload when the data turns cfg.refresh old, but never sooner than the
	// retry interval (avoids hammering CelesTrak if clocks are odd).
	wait := time.Until(set.updated.Add(s.cfg.refresh))
	return max(wait, retryAfterFailure)
}
