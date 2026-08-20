package main

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// metrics serves this process's Prometheus text-format metrics: the
// staleness signal the deliberate stale-fallback in internal/fpl.Client
// needs to be paged on — see that type's doc comment for why the fallback
// exists — plus the HTTP, render-mutex and pipeline-timing series added
// alongside it. See instruments.go for what each series means.
//
// Deliberately outside squadServer.mu, like /-/upstream: a scrape must be
// answerable even while a render is in flight — or wedged — under that
// mutex, and ServeHTTP calls this handler directly, bypassing the
// statusRecorder/Observe wrapping every other route gets, for the same
// reason: a scrape hitting every replica on its own interval must not
// pollute the very counters it exposes.
//
// client_golang rather than the hand-rolled exposition text this used to
// write directly: a histogram needs correctly ascending cumulative bucket
// counts, a mandatory "+Inf" bucket exactly equal to _count, and a sum
// updated consistently under concurrent observations — three properties a
// hand-rolled writer can get plausibly, silently wrong, which is exactly the
// failure class ("looks right, reads a wrong percentile") this project's own
// standing rules exist to catch elsewhere. Three gauges never needed that;
// five histograms do. The dependency is pure Go — no cgo — so the distroless
// static build this ships as is unaffected.
//
// This route carries no auth check of its own, unlike every write route on
// this server. That is a deliberate division of labour, not an oversight:
// keeping a metrics endpoint off the public internet is the deployment's
// job, the same way paging on what it reports is — this handler's only
// responsibility is to answer correctly whenever it is asked.
func (s *squadServer) metrics(w http.ResponseWriter, r *http.Request) {
	// GET only, like every other read route on this server states its method
	// explicitly. The handler has no side effect regardless, so this is a
	// courtesy to a caller rather than a guard against one. promhttp does not
	// give this away for free, so it stays a manual check ahead of the handoff.
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "the metrics route takes a GET", http.StatusMethodNotAllowed)
		return
	}

	// Two registries, gathered as one response: appMetrics is process-wide
	// and built before any squadServer exists; s.metricsRegistry() is this
	// server's own three staleness series, which need s.client to read from.
	promhttp.HandlerFor(
		prometheus.Gatherers{appMetrics.registry, s.metricsRegistry()},
		promhttp.HandlerOpts{},
	).ServeHTTP(w, r)
}
