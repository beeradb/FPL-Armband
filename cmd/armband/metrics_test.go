package main

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"armband/internal/fpl"
)

// TestMetricsRouteServesAllThreeSeries pins the wiring: a GET to /metrics
// answers with the three named series in Prometheus text-exposition format.
// The state-transition logic itself (when StaleServing flips, what
// StaleAgeSeconds and LiveFetchFailures count) is internal/fpl's own
// responsibility and is tested there — this only checks that cmd/armband
// exposes what that package already tracks.
func TestMetricsRouteServesAllThreeSeries(t *testing.T) {
	s := &squadServer{client: fpl.New(t.TempDir(), time.Hour, time.Hour)}

	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	s.metrics(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"armband_serving_stale_data 0",
		"armband_stale_data_age_seconds 0",
		"armband_live_fetch_failures_total 0",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics body missing %q:\n%s", want, body)
		}
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain", ct)
	}
}

// TestMetricsRouteRejectsNonGET matches every other read route's courtesy
// method check (see playerDetail in playerdetail.go).
func TestMetricsRouteRejectsNonGET(t *testing.T) {
	s := &squadServer{client: fpl.New(t.TempDir(), time.Hour, time.Hour)}

	req := httptest.NewRequest("POST", "/metrics", nil)
	rec := httptest.NewRecorder()
	s.metrics(rec, req)

	if rec.Code != 405 {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

// TestMetricsIsReachableThroughServeHTTP pins that the route is actually
// wired into the dispatcher, not just callable directly — the loopback Host
// check in ServeHTTP is the one thing a direct call to s.metrics skips.
func TestMetricsIsReachableThroughServeHTTP(t *testing.T) {
	s := &squadServer{client: fpl.New(t.TempDir(), time.Hour, time.Hour)}

	req := httptest.NewRequest("GET", "/metrics", nil)
	req.Host = "127.0.0.1"
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200 (route not wired into ServeHTTP?)", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "armband_serving_stale_data") {
		t.Errorf("expected metrics body, got: %s", rec.Body.String())
	}
}
