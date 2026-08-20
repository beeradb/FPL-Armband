package main

import (
	"fmt"
	"net/http"
)

// metrics serves this process's Prometheus text-format metrics: the
// staleness signal the deliberate stale-fallback in internal/fpl.Client
// needs to be paged on — see that type's doc comment for why the fallback
// exists — and nothing else.
//
// Deliberately outside squadServer.mu, like /-/upstream: a scrape must be
// answerable even while a render is in flight — or wedged — under that
// mutex, and this handler is three atomic loads, never a rebuild.
//
// Hand-rolled exposition format rather than a client_golang dependency:
// three metrics do not need one, and this binary ships static and
// dependency-light on purpose.
//
// Not reachable from the public internet: the nginx sidecar this is proxied
// through serves it on a separate port Traefik's routing table never
// forwards to — see k8s/armband/nginx.conf's :8082 server block in the
// deployment repository, the same pattern the nginx-prometheus-exporter
// sidecar's own port 9113 already uses.
func (s *squadServer) metrics(w http.ResponseWriter, r *http.Request) {
	// GET only, like every other read route on this server states its method
	// explicitly. The handler has no side effect regardless, so this is a
	// courtesy to a caller rather than a guard against one.
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "the metrics route takes a GET", http.StatusMethodNotAllowed)
		return
	}

	stale := 0
	if s.client.StaleServing() {
		stale = 1
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprint(w, "# HELP armband_serving_stale_data 1 if the process is currently serving FPL data older than its cache TTL because a live fetch failed, 0 otherwise.\n")
	fmt.Fprint(w, "# TYPE armband_serving_stale_data gauge\n")
	fmt.Fprintf(w, "armband_serving_stale_data %d\n", stale)

	fmt.Fprint(w, "# HELP armband_stale_data_age_seconds Age in seconds of the most recently served stale data, 0 if none has been served this process's lifetime.\n")
	fmt.Fprint(w, "# TYPE armband_stale_data_age_seconds gauge\n")
	fmt.Fprintf(w, "armband_stale_data_age_seconds %d\n", s.client.StaleAgeSeconds())

	fmt.Fprint(w, "# HELP armband_live_fetch_failures_total Live fetches to the FPL API that have failed since the process started.\n")
	fmt.Fprint(w, "# TYPE armband_live_fetch_failures_total counter\n")
	fmt.Fprintf(w, "armband_live_fetch_failures_total %d\n", s.client.LiveFetchFailures())
}
