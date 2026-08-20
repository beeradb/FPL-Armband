package main

import (
	"github.com/prometheus/client_golang/prometheus"
)

// appMetrics is this process's Prometheus registry for everything that is not
// per-squadServer state — the HTTP dispatch, the render mutex, and the two
// timed pipelines (Optimize and the page build).
//
// Its own registry, never prometheus.DefaultRegisterer: package main is every
// subcommand of this binary (squad, transfers, replay, serve, backtest...),
// not just serve, so registering onto the global default would put these
// series in front of a subcommand that never scrapes and never wanted them
// registered at all — and would panic on a second `serve` construction in the
// same process, which the test suite does routinely.
var appMetrics = newInstruments()

// instruments is one process's worth of the series /metrics exposes beyond
// the three staleness gauges, which live on each squadServer instead because
// they close over that server's *fpl.Client — see metricsRegistry.
type instruments struct {
	registry *prometheus.Registry

	// httpRequests and httpRequestDuration count ORIGIN traffic only: the
	// deployment caches "/", "/app" and "/api/state" for 60 seconds ahead of
	// this process, so a request that reaches this handler is already a
	// cache miss. The deployment's own edge metrics are the instrument for
	// visitor traffic; these two will legitimately disagree with it by
	// roughly the cache hit ratio, and that gap is not a bug in either one.
	// This has already caused one round of confusion, hence spelling it out
	// in the HELP text below rather than only here. Which proxy sits in
	// front of this process is a deployment detail this public repository
	// does not describe.
	httpRequests        *prometheus.CounterVec
	httpRequestDuration *prometheus.HistogramVec

	// renderMutexWait is how long a request waited on squadServer.mu before a
	// render could start — see lockRender in serve.go, the one place that
	// takes the lock now.
	renderMutexWait *prometheus.HistogramVec

	// optimizeDuration times Engine.Optimize itself, wired through
	// analysis.ObserveOptimize by cmdServe. It is the search alone: no
	// mutex wait, no JSON marshal, no week-view re-optimisation.
	optimizeDuration prometheus.Histogram

	// pageBuildSeconds is buildSquadPage's own stage breakdown (optimize,
	// weekviews, transfer plan, overrides, page assemble) — see page.go's
	// mark closure. Distinct from optimizeDuration: this is unconditional and
	// scraped, where FPL_SERVE_TIMINGS's stderr print stays opt-in.
	pageBuildSeconds *prometheus.HistogramVec
}

// httpBuckets covers a slow /api/state under load up to this deployment's own
// 90-second upstream timeout — prometheus.DefBuckets tops out at 10s, which
// would silently clip every request slow enough to be worth alerting on.
var httpBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 90}

// mutexWaitBuckets shares httpBuckets' 90-second ceiling for the same reason
// — a render holding the mutex is itself bounded by the same upstream
// timeout — but starts a decade finer, since an uncontended lock() should
// read in the sub-millisecond bucket, not share a floor with network latency.
var mutexWaitBuckets = []float64{0.001, 0.01, 0.1, 0.5, 1, 2, 5, 10, 30, 60, 90}

// pipelineBuckets covers Optimize and the page-build stages, which run
// seconds not milliseconds and never approach the HTTP layer's 90s ceiling —
// the optimiser is the dominant cost in a build, but it is not the network
// call, so its own histogram tops out an order of magnitude lower.
var pipelineBuckets = []float64{0.1, 0.25, 0.5, 1, 1.5, 2, 3, 5, 8, 15, 30, 60}

func newInstruments() *instruments {
	reg := prometheus.NewRegistry()
	in := &instruments{
		registry: reg,
		httpRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "armband_http_requests_total",
			Help: "HTTP requests this process answered directly, by route, method and " +
				"status code — ORIGIN traffic only. The deployment caches \"/\", " +
				"\"/app\" and \"/api/state\" for 60 seconds ahead of this process, so " +
				"this counter only sees cache misses; the deployment's own edge " +
				"metrics are the instrument for visitor traffic, and the two will " +
				"legitimately disagree by roughly the cache hit ratio.",
		}, []string{"route", "method", "code"}),
		httpRequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "armband_http_request_duration_seconds",
			Help: "How long this process took to answer a request, by route — ORIGIN " +
				"traffic only, for the same reason as armband_http_requests_total: the " +
				"deployment's 60-second cache ahead of this process means this counter " +
				"only ever times a cache miss, and the deployment's own edge metrics " +
				"are what a visitor actually experienced.",
			Buckets: httpBuckets,
		}, []string{"route"}),
		renderMutexWait: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "armband_render_mutex_wait_seconds",
			Help:    "Time a request spent waiting to acquire squadServer's render mutex, by route.",
			Buckets: mutexWaitBuckets,
		}, []string{"route"}),
		optimizeDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "armband_optimize_duration_seconds",
			Help:    "Wall-clock time of one Engine.Optimize call — the search alone.",
			Buckets: pipelineBuckets,
		}),
		pageBuildSeconds: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "armband_page_build_seconds",
			Help:    "buildSquadPage's own stage breakdown, by stage.",
			Buckets: pipelineBuckets,
		}, []string{"stage"}),
	}
	reg.MustRegister(
		in.httpRequests,
		in.httpRequestDuration,
		in.renderMutexWait,
		in.optimizeDuration,
		in.pageBuildSeconds,
	)
	return in
}

// metricsRegistry is this server's own collectors: the three staleness
// series metrics.go has always exposed, now via client_golang's GaugeFunc and
// CounterFunc rather than hand-rolled text. They cannot live on the
// package-level appMetrics registry above, because they close over s.client
// and appMetrics is built before any squadServer exists.
//
// sync.Once rather than a constructor: metrics_test.go and every other
// caller build a squadServer as a bare struct literal (`&squadServer{client:
// ...}`), so first-use lazy init is the only place guaranteed to run
// regardless of how the struct was built. Each GaugeFunc/CounterFunc closure
// reads s.client at Collect time — i.e. at scrape time — so this is a live
// read on every call, never a rebuild or a cached value, and it touches
// nothing s.mu guards.
func (s *squadServer) metricsRegistry() *prometheus.Registry {
	s.metricsOnce.Do(func() {
		reg := prometheus.NewRegistry()
		reg.MustRegister(
			prometheus.NewGaugeFunc(prometheus.GaugeOpts{
				Name: "armband_serving_stale_data",
				Help: "1 if the process is currently serving FPL data older than its cache TTL because a live fetch failed, 0 otherwise.",
			}, func() float64 {
				if s.client.StaleServing() {
					return 1
				}
				return 0
			}),
			prometheus.NewGaugeFunc(prometheus.GaugeOpts{
				Name: "armband_stale_data_age_seconds",
				Help: "Age in seconds of the most recently served stale data, 0 if none has been served this process's lifetime.",
			}, func() float64 {
				return float64(s.client.StaleAgeSeconds())
			}),
			prometheus.NewCounterFunc(prometheus.CounterOpts{
				Name: "armband_live_fetch_failures_total",
				Help: "Live fetches to the FPL API that have failed since the process started.",
			}, func() float64 {
				return float64(s.client.LiveFetchFailures())
			}),
		)
		s.metricsReg = reg
	})
	return s.metricsReg
}
