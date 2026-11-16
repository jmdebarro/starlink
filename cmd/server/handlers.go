package main

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// routes builds the URL router.
func (s *state) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /api/positions", s.handlePositions)
	mux.HandleFunc("GET /api/catalog", s.handleCatalog)

	if s.cfg.webDir != "" {
		// Serve the web/ folder at "/". The more specific routes above win.
		mux.Handle("GET /", http.FileServer(http.Dir(s.cfg.webDir)))
	}
	return s.cors(mux)
}

// cors adds the CORS header to /api/* responses and answers preflight
// (OPTIONS) requests. A "middleware" is just a function that wraps a handler.
func (s *state) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Access-Control-Allow-Origin", s.cfg.corsOrigin)
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *state) handleHealth(w http.ResponseWriter, r *http.Request) {
	if s.current.Load() == nil {
		http.Error(w, "no TLE data loaded yet", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte("ok\n"))
}

func (s *state) handlePositions(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	snap, ok := s.positions(now)
	if !ok {
		http.Error(w, "no TLE data loaded yet", http.StatusServiceUnavailable)
		return
	}
	// Tell caches to keep the response until the window ends (at least 1 s).
	maxAge := snap.t0 + windowSeconds - now.Unix()
	if maxAge < 1 {
		maxAge = 1
	}
	w.Header().Set("Cache-Control", "public, max-age="+strconv.FormatInt(maxAge, 10))
	writeJSON(w, r, snap.payload)
}

func (s *state) handleCatalog(w http.ResponseWriter, r *http.Request) {
	set := s.current.Load()
	if set == nil {
		http.Error(w, "no TLE data loaded yet", http.StatusServiceUnavailable)
		return
	}
	// The catalog only changes on reload, so a few minutes of caching is safe.
	w.Header().Set("Cache-Control", "public, max-age=300")
	writeJSON(w, r, set.catalog)
}

// writeJSON sends the gzip bytes if the client accepts them, else plain.
func writeJSON(w http.ResponseWriter, r *http.Request, p payload) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Vary", "Accept-Encoding")
	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Length", strconv.Itoa(len(p.gz)))
		w.Write(p.gz)
		return
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(p.plain)))
	w.Write(p.plain)
}
