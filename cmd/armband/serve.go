package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"armband/internal/analysis"
	"armband/internal/config"
	"armband/internal/fpl"
	"armband/internal/present"
	"armband/internal/signup"
	"armband/internal/webui"
)

// signupDSNEnv names the environment variable carrying the signup database URL.
//
// Set means the landing page's gate records what it accepts; unset means it accepts and
// discards. There is no flag equivalent, on purpose — see cmdServe.
const signupDSNEnv = "ARMBAND_SIGNUPS_DSN"

// cmdServe hosts the client application over HTTP: the embedded documents from
// internal/webui — one per page, however many pages exist — and the model behind them at
// /api/state.
//
// It is built by the same pipeline the terminal commands use (buildSquadPage) — the
// difference is that it is live. `armband squad -html` no longer writes a page at all;
// it refuses and says where the page went.
//
// # Why the page re-renders on every GET
//
// The alternative is a cached page invalidated on POST, and a cache with
// invalidation is this project's signature bug class: the page silently shows
// last week's squad, or last week's overrides. Rebuilding costs the optimiser's
// seconds, which a person clicking through a page will not notice, and it means
// the page can never be stale — the one correct path, no fallback.
//
// A single mutex serialises requests. This is a one-user tool and the engine
// was not written for concurrent access; the mutex is cheaper than auditing it.
// applyPreviewHorizon narrows the engine to the preview's window and says so.
//
// Separate from cmdServe because that function binds a socket, and the thing worth testing
// here is the decision rather than the server: which window the preview scores over, and
// whether the reader is told. A test that set Weights.Horizon itself and then checked the
// document reported it would pass unchanged if the flag stopped being read at all.
//
// Zero means "use whatever the config says", which is how a reader turns the preview's
// shortening off. A negative value means the same rather than an error: the flag's contract
// is a gameweek count, and there is no sensible engine state for minus two.
//
// It returns the sentence to print, or "" when nothing changed. The narrowing is not
// cosmetic — every projection on the page is over this window — so a silent change of it
// would leave the page quoting numbers whose meaning the reader cannot see.
func applyPreviewHorizon(e *analysis.Engine, horizon int) string {
	if horizon <= 0 || horizon == e.Weights.Horizon {
		return ""
	}
	was := e.Weights.Horizon
	e.Weights.Horizon = horizon
	return fmt.Sprintf("Scoring the preview over %d gameweeks rather than the configured %d "+
		"(-horizon). Every projection on the page is over that window.", horizon, was)
}

func cmdServe(ctx context.Context, cfg config.Config, cfgPath, teamPath string,
	client *fpl.Client, e *analysis.Engine, weeks int) error {

	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", "127.0.0.1:8080", "address to serve on (loopback only)")
	persist := fs.Bool("persist", false, "write lock/boot overrides back to the files "+
		"(a boot to -config, a lock to -team); without it they live in a "+
		"browser-session cookie")
	// A shorter horizon than the terminal commands use.
	//
	// The scoring horizon is how many gameweeks a player's Score is averaged over, and it
	// is the dominant cost in the build: the optimiser prices every candidate over every
	// week in it. Four rather than the configured five is a deliberate trade of a little
	// lookahead for a page that answers quickly, and it is a PREVIEW-only choice —
	// `squad`, `transfers` and the replay are untouched.
	//
	// ⚠️ It changes the numbers, not just the speed. Every projection on the page is over
	// four gameweeks rather than five, which is why State carries the horizon and the page
	// is told rather than left to imply one.
	horizon := fs.Int("horizon", 4, "gameweeks the preview scores over (0 uses the config's)")
	if err := fs.Parse(flag.Args()[1:]); err != nil {
		return err
	}
	if err := validateServeAddr(*addr); err != nil {
		return err
	}
	token, err := newServeToken()
	if err != nil {
		return err
	}

	if note := applyPreviewHorizon(e, *horizon); note != "" {
		fmt.Fprintf(os.Stderr, "%s\n", dim(note))
	}
	if weeks <= 0 {
		weeks = e.Weights.Horizon
	}

	// The signup store comes from the ENVIRONMENT, not from a flag.
	//
	// A DSN carries a password, and a flag puts it in the process table for every
	// other user on the box, in the Kubernetes manifest that describes the pod, and
	// in the shell history of whoever last ran it by hand. An environment variable
	// sourced from a Secret is the one that can be granted without being published.
	//
	// Unset is a supported mode rather than an error: the local `armband serve` is one
	// reader who does not need to be captured, and its landing page posts to the live
	// site anyway. Which mode is in force is printed below, because a capture that is
	// silently off is the failure this whole change exists to remove.
	var signups signup.Store
	if dsn := os.Getenv(signupDSNEnv); dsn != "" {
		store, err := signup.Open(ctx, dsn)
		if err != nil {
			return err
		}
		defer store.Close()
		signups = store
	}

	// Wired here and only here: cmdServe is the one path this call is expensive
	// enough to page on. Every other subcommand — squad, transfers, replay, a
	// backtest sweep — leaves analysis.ObserveOptimize nil, which Optimize
	// checks before doing anything, so this line is the entire cost of the hook
	// existing at all when nothing is listening.
	analysis.ObserveOptimize = func(d time.Duration) {
		appMetrics.optimizeDuration.Observe(d.Seconds())
	}

	s := &squadServer{
		token:           token,
		cfg:             &cfg,
		cfgPath:         cfgPath,
		teamPath:        teamPath,
		client:          client,
		engine:          e,
		weeks:           weeks,
		persist:         *persist,
		signups:         signups,
		wildcardEnabled: cfg.WildcardEnabled,
	}

	if s.wildcardEnabled {
		fmt.Fprintf(os.Stderr, "%s\n", dim("/wildcard is enabled (config.json wildcard_enabled)."))
	} else {
		fmt.Fprintf(os.Stderr, "%s\n", dim("/wildcard is disabled — config.json wildcard_enabled "+
			"is false or unset. /wildcard and /api/wildcard both 404, and the "+
			"\"If we chipped\" link is stripped from every page that carries it."))
	}
	fmt.Fprintf(os.Stderr, "\n%s\n", dim("Serving the squad page on http://"+*addr+"/?t="+token))
	fmt.Fprintf(os.Stderr, "%s\n", dim("Open that exact URL — the token gates the page's write actions."))
	if *persist {
		// Both destinations, named. A lock goes to the team file and a boot to
		// the config, and `config.SavePair` REFUSES a lock when -team is absent
		// rather than dropping it — so saying only the config path here would
		// leave the reader to discover the refusal by pressing the button.
		where := cfgPath
		if teamPath != "" {
			where += " (and locks to " + teamPath + ")"
		} else {
			where += " — but NOT locks: pass -team to have somewhere to write them"
		}
		fmt.Fprintf(os.Stderr, "%s\n", dim("Overrides write back to "+where+" (-persist)."))
	} else {
		fmt.Fprintf(os.Stderr, "%s\n", dim("Overrides live in a browser-session cookie — "+
			"neither config.json nor the team file is touched. Run serve -persist "+
			"to write them back instead."))
	}
	if signups != nil {
		fmt.Fprintf(os.Stderr, "%s\n", dim("Landing-page signups are recorded to the "+
			"database named by "+signupDSNEnv+"."))
	} else {
		fmt.Fprintf(os.Stderr, "%s\n", dim("Landing-page signups are NOT recorded — "+
			signupDSNEnv+" is unset, so /gate refuses with a 503. The landing "+
			"page posts to the live site, so this is the ordinary local mode."))
	}
	fmt.Fprintf(os.Stderr, "%s\n\n", dim("Ctrl-C stops the server."))
	// ListenAndServe never looks at the context, and signal.NotifyContext
	// replaces the default SIGINT/SIGTERM behaviour — so without this, Ctrl-C
	// cancels the context and the server keeps serving, which is the one
	// failure mode a user WILL hit. Closing the listener on cancellation is
	// what makes "Ctrl-C stops the server" true.
	srv := &http.Server{Addr: *addr, Handler: s}
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	// ErrServerClosed is the shutdown the go routine above asked for — the
	// deliberate end of the command, not an error to print and exit 1 on.
	return nil
}

// squadServer is the whole server: the page and the write actions.
type squadServer struct {
	mu    sync.Mutex
	token string
	cfg   *config.Config
	// teamPath is the owner's team file, and may be empty — the deployed server
	// is never given one, which is the point: a chip plan and a squad lock are
	// one manager's decisions and the site has no business holding them.
	//
	// It matters only under -persist, where the page writes back. A LOCK lives
	// in that file now, so the lock button on the owner's own page needs
	// somewhere to put it; config.SavePair refuses the write rather than
	// dropping it when this is empty. See its comment.
	teamPath string

	// cfgPath is where -persist saves changes. It may be empty, which makes a
	// persisted override unsaveable — and the handler refuses the action
	// rather than showing an override the next run would not have.
	cfgPath string
	client  *fpl.Client
	engine  *analysis.Engine
	weeks   int
	// persist switches the write actions' store: true writes back to
	// config.json, the default false keeps them in a browser-session cookie.
	// The default config stays a default — the page never mutates it unless
	// the user asked.
	persist bool
	// signups records the landing page's email gate. Nil means no store is
	// configured, and the gate then REFUSES with a 503 rather than accepting
	// and discarding — a deployment that lost its database URL must not tell
	// readers they signed up. See cmdServe and the gate handler.
	//
	// Deliberately NOT guarded by the mutex above. That mutex serialises the
	// optimiser, and the deployment's nginx sidecar permits POST /gate through
	// its perimeter specifically because the gate does not queue behind a
	// render; taking the engine's lock here would make that justification
	// false and put a form submission behind a multi-second squad rebuild.
	// The store is safe for concurrent use on its own.
	signups signup.Store
	// clock is the wall clock, injectable so a test can pin the date the page
	// is ABOUT. It is the only non-determinism left in a rendered page: the
	// squad is a function of the bootstrap, but the override staleness rule
	// asks how many days ago each override was last checked, so a page built
	// today and the same page built tomorrow differ. Nil means time.Now — see
	// now().
	clock func() time.Time
	// seed draws the variety seed for a session that has none. Injectable for the same
	// reason the clock is, and for a sharper one: two requests without a cookie each mint
	// their own, so a test that reads the state through Go and then again through a
	// browser was comparing two DIFFERENT squads and failing on the difference. Nil means
	// a real random seed.
	seed func() int64
	// fetchSummary is the seam over Client.ElementSummary, injectable for the same reason:
	// a test for /api/player needs a fixed history with no live client and no network call.
	// Nil means client.ElementSummary. See playerdetail.go.
	fetchSummary func(ctx context.Context, id int) (*fpl.ElementSummary, error)
	// fetchEntry and fetchPicks are the same seam over Client.EntryUncached and
	// Client.PicksUncached, for PUT /api/import — a test needs a fixed entry and picks
	// response with no live client and no network call, and it needs to drive every
	// branch (not found, unreachable, a real squad) without standing up a fake HTTP
	// server for each. Nil means client.EntryUncached / client.PicksUncached. See
	// importteam.go.
	fetchEntry func(ctx context.Context, id int) (*fpl.Entry, error)
	fetchPicks func(ctx context.Context, entryID, event int) (*fpl.EntryPicks, error)

	// chips is GET /api/wildcard's cache of the two rebuilt WeekViews for one
	// gameweek — see chipCache's own comment for why the route needs one at
	// all. Guarded by mu above, the same render lock the build itself takes.
	// Nil until the first build. See chipCacheGet/invalidateChipCache.
	chips *chipCache

	// wildcardEnabled gates /wildcard and /api/wildcard (both 404 when
	// false) and hides the "If we chipped" link from landing.html, app.html
	// and team.html. Mirrors cfg.WildcardEnabled — see that field's own
	// comment for why this ships off by default and lives in config.json
	// rather than a flag or an environment variable.
	wildcardEnabled bool

	// metricsOnce and metricsReg are this server's lazily-built Prometheus
	// registry — the three staleness series, which close over client above.
	// See metricsRegistry in instruments.go for why lazy: there is no
	// constructor, so first-scrape init is the only place guaranteed to run
	// regardless of how the struct was built.
	metricsOnce sync.Once
	metricsReg  *prometheus.Registry
}

// nextSeed is the variety seed for a session that has none.
func (s *squadServer) nextSeed() int64 {
	if s.seed == nil {
		return newSeed()
	}
	return s.seed()
}

// now is the clock the page build reads. A nil clock means the real one, so
// every existing construction of squadServer keeps working and only a test has
// to say anything.
func (s *squadServer) now() time.Time {
	if s.clock == nil {
		return time.Now()
	}
	return s.clock()
}

// lockRender is s.mu.Lock(), instrumented — a drop-in replacement for
// `s.mu.Lock(); defer s.mu.Unlock()`, used as `defer s.lockRender("state")()`.
// It is the only place in this file (or webroutes.go) that may call
// s.mu.Lock() directly; TestEveryRenderLockGoesThroughLockRender greps for
// the alternative and fails if one turns up. It exists at exactly the three
// sites that render under this mutex — action here, state and saveSession in
// webroutes.go — and is deliberately a straight substitution: nothing that
// ran before or after the lock at any of those sites moves. saveSession in
// particular relies on this — its token and origin checks run BEFORE the
// lock is even requested, so a flood of 403s never reaches this call at all.
func (s *squadServer) lockRender(route string) func() {
	start := time.Now()
	s.mu.Lock()
	appMetrics.renderMutexWait.WithLabelValues(route).Observe(time.Since(start).Seconds())
	return s.mu.Unlock
}

// routeFor resolves a request path to its handler and the label every other
// instrument on this server keys on — one table, stated once, so routing and
// the metrics label cannot disagree. The rejected alternative was having each
// handler label itself: eight call sites to keep in sync, and the fallback
// 404 branch has no handler of its own to annotate.
func (s *squadServer) routeFor(path string) (http.Handler, string) {
	// The asset tree is the one prefix route; everything else below is an
	// exact path, so a typo answers 404 rather than falling into a handler
	// that half-matches.
	if strings.HasPrefix(path, prefixAssets) {
		return webui.StaticHandler(prefixAssets), "assets"
	}
	if strings.HasPrefix(path, prefixPlayer) {
		return http.HandlerFunc(s.playerDetail), "player"
	}
	switch path {
	case routeLanding:
		// The app itself is the front door now: no email, no redirect, nothing to pass
		// through first. See routeApp and routeAbout below for where the two routes
		// this used to arbitrate between now live.
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s.servePage(w, r, "app")
		}), "app"
	case routeApp:
		// /app is kept as a redirect rather than deleted -- every link and bookmark
		// anyone has made to it during the gated era still resolves. 302, not 301: a
		// 301 gets cached by browsers indefinitely, and whether /app should exist at
		// all past this transition is a genuinely open, reversible question the
		// product owner has not settled.
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, routeLanding, http.StatusFound)
		}), "app-redirect"
	case routeAbout:
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s.servePage(w, r, "landing")
		}), "about"
	case routeGate:
		return http.HandlerFunc(s.gate), "gate"
	case routeState:
		return http.HandlerFunc(s.state), "state"
	case routeArmbandTeam:
		// Ungated, deliberately: see routeArmbandTeam's own comment.
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s.servePage(w, r, "team")
		}), "armband-team"
	case routeArmbandTeamState:
		return http.HandlerFunc(s.armbandTeamState), "armband-team-state"
	case routeWildcard:
		// Ungated, deliberately: see routeWildcard's own comment. "Ungated"
		// is about the auth token, not about wildcardEnabled — a feature
		// still off for everyone answers 404 for everyone the same way, auth
		// or not, same as a route this binary had never heard of.
		if !s.wildcardEnabled {
			return http.HandlerFunc(http.NotFound), "wildcard-disabled"
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s.servePage(w, r, "wildcard")
		}), "wildcard"
	case routeWildcardState:
		if !s.wildcardEnabled {
			return http.HandlerFunc(http.NotFound), "wildcard-state-disabled"
		}
		return http.HandlerFunc(s.apiChipTeams), "wildcard-state"
	case routeSession:
		return http.HandlerFunc(s.saveSession), "session"
	case routeImport:
		return http.HandlerFunc(s.importTeam), "import"
	case routeResults:
		return http.HandlerFunc(s.apiResults), "results"
	case routeTransfers:
		return http.HandlerFunc(s.apiTransfers), "transfers"
	case routeMetrics:
		return http.HandlerFunc(s.metrics), "metrics"
	case "/action":
		return http.HandlerFunc(s.action), "action"
	default:
		return http.HandlerFunc(http.NotFound), "notfound"
	}
}

// statusRecorder captures the status code a handler answered with, so
// ServeHTTP can label a request's duration and count by the code it actually
// sent rather than assuming 200.
//
// No http.Flusher/http.Hijacker/io.ReaderFrom passthrough. Nothing on this
// server streams, hijacks a connection, or serves from an *os.File — assets
// come from an embed.FS via http.FileServer, which does its own io.Copy —
// so there is nothing here that would ever need one; do not add passthrough
// on the assumption something does.
type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (rec *statusRecorder) WriteHeader(code int) {
	rec.status = code
	rec.wrote = true
	rec.ResponseWriter.WriteHeader(code)
}

func (rec *statusRecorder) Write(b []byte) (int, error) {
	// net/http's own ResponseWriter defaults an unannounced status to 200 the
	// same way, the moment Write is first called without a prior WriteHeader.
	if !rec.wrote {
		rec.status = http.StatusOK
		rec.wrote = true
	}
	return rec.ResponseWriter.Write(b)
}

func (s *squadServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// The Host check is the other half of the loopback bind, and it runs
	// before routeFor, unconditionally: a browser that has been DNS-rebound
	// arrives at this socket with a foreign Host header, and from that
	// browser's point of view the ORIGIN is the foreign name — so
	// same-origin policy hands the answer to the page the attacker controls,
	// token and all. Rejecting the Host is what keeps the token readable
	// only by pages whose origin really is this listener. A rejected Host is
	// not attributed to any route label; it never reaches one.
	if !loopbackHost(r.Host) {
		http.Error(w, "unrecognised host", http.StatusForbidden)
		return
	}

	handler, label := s.routeFor(r.URL.Path)
	if label == "metrics" {
		// Never wrapped, never measured: a scrape must be answerable even
		// mid-render — or wedged — under s.mu, and observing it would let a
		// scrape hitting every replica on its own interval pollute the very
		// counters it exposes. See metrics.go's doc comment.
		handler.ServeHTTP(w, r)
		return
	}

	rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	start := time.Now()
	handler.ServeHTTP(rec, r)
	appMetrics.httpRequestDuration.WithLabelValues(label).Observe(time.Since(start).Seconds())
	appMetrics.httpRequests.WithLabelValues(label, metricsMethod(r.Method), strconv.Itoa(rec.status)).Inc()
}

// metricsMethod collapses r.Method to the fixed set this server actually
// handles (GET, POST, PUT) before it becomes a label value. r.Method is
// whatever the caller sent — net/http accepts any RFC 7230 token, unbounded
// and client-controlled — so using it as a label verbatim would let anyone
// who can reach this port mint an unbounded number of Prometheus series by
// sending a distinct method string each time.
func metricsMethod(m string) string {
	switch m {
	case http.MethodGet, http.MethodPost, http.MethodPut:
		return m
	default:
		return "other"
	}
}

// action applies one lock or boot and answers with a redirect.
//
// ⚠️ Nothing in the shipped application posts here. The retired page's forms did; the
// client does not, so lock and boot change the page in front of the reader and reach
// neither store. The handler and the session store below are kept because connecting them
// is the next piece of work and their tests are the only thing protecting the
// config-default / browser-session split — not because anything calls them today.
//
// The whole write path: the CSRF token gates it, the action and the code are
// validated, the change is SAVED before it is adopted — a failed save leaves
// the server and the config exactly as they were, rather than showing an
// override the next run will not have — and the redirect lands the reader back
// on the view and query they acted from.
func (s *squadServer) action(w http.ResponseWriter, r *http.Request) {
	// Clamped BEFORE the mutex and the token check: the body parse is the one
	// thing an unauthenticated caller can make arbitrarily expensive, and the
	// form carries four small fields. Anything bigger is refused rather than
	// parsed.
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	defer s.lockRender("action")()

	if !s.tokenOK(r.PostFormValue("t")) {
		http.Error(w, "missing or wrong token", http.StatusForbidden)
		return
	}
	act := r.PostFormValue("a")
	code, err := strconv.Atoi(r.PostFormValue("c"))
	if err != nil {
		http.Error(w, "the action needs a player code", http.StatusBadRequest)
		return
	}
	// The code must resolve against the bootstrap, or a stale form could
	// persist an override on a player this season does not contain — a
	// nameless card in the excluded list forever. The page only ever posts
	// codes it read from the bootstrap, so a miss means the form is stale.
	name := s.playerName(code)
	if name == "" {
		http.Error(w, "no such player code", http.StatusBadRequest)
		return
	}

	next := *s.cfg
	today := s.now().Format("2006-01-02")
	// The reasons are canned rather than free text: this is a button, not the
	// agent's review, and a free-text field would carry whatever the browser
	// sent into the system prompt of every future run. Provenance is what a
	// page-created override owes its reviewer, and the date is carried by
	// SetOn.
	pageOverride := func(name, reason string) config.RosterOverride {
		return config.RosterOverride{
			Code: code, Name: name, Reason: reason,
			SetOn: today, LastChecked: today,
		}
	}
	switch act {
	case "lock":
		err = next.Roster.Set("lock", pageOverride(name, "locked from the squad page"), nil)
	case "boot":
		err = next.Roster.Set("exclude", pageOverride(name, "booted from the squad page"), nil)
	case "unlock":
		err = next.Roster.Remove("lock", code)
	case "unboot":
		err = next.Roster.Remove("exclude", code)
	default:
		http.Error(w, fmt.Sprintf("unknown action %q", act), http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Always a redirect. This used to be able to answer with a freshly rendered
	// page for the page's own script to morph into place; there is no
	// server-rendered page any more, and the application re-fetches /api/state
	// instead. A 303 is also the answer browsers apply a just-set cookie before
	// following, which is what the session store needs.

	if s.persist {
		// Saved before adopted: a failed save leaves the server and the
		// config exactly as they were.
		if s.cfgPath == "" {
			http.Error(w, "no config path — the override was not saved", http.StatusInternalServerError)
			return
		}
		// SavePair, not Save: "lock" and "unlock" move `Roster.Lock`, which
		// lives in the team file. Save would marshal the config without it and
		// the button would report success having written nothing.
		if err := config.SavePair(s.cfgPath, s.teamPath, *s.cfg, next); err != nil {
			http.Error(w, fmt.Sprintf("saving config: %v", err), http.StatusInternalServerError)
			return
		}
		s.cfg = &next
	} else {
		// The default store is the browser session: the cookie mutates, the
		// config file stays a default. Session overrides ride on top of the
		// config for every build of this page, and they die with the browser
		// session.
		sess := s.readValidSession(r).applyAction(act, code)
		if err := sess.write(w); err != nil {
			http.Error(w, fmt.Sprintf("storing the session: %v", err),
				http.StatusInternalServerError)
			return
		}
	}

	// Back where the reader acted from. The redirect is what applies the cookie:
	// browsers set it before following a 303, so the next request already carries
	// the override.
	ret := r.PostFormValue("ret")
	if !safeRetPath(ret) {
		// routeLanding, not routeApp: the tool now lives at "/", and routeApp is only
		// a redirect there.
		ret = routeLanding
	}
	http.Redirect(w, r, ret, http.StatusSeeOther)
}

// The session store lives in session.go. It used to be a pair of code lists declared here
// and it now carries the reader's whole team, so it has a file of its own.

// effectiveCfg is the config this request builds the page from: the real
// config with the session's overrides applied on top. Session wins on
// conflict — a session boot clears a config lock for THIS page, never the
// file — so the page's controls always express the reader's latest decision
// without touching config.json. In persist mode the session is ignored and
// the config is the one store.
func (s *squadServer) effectiveCfg(r *http.Request) config.Config {
	return s.effectiveCfgFrom(s.readValidSession(r))
}

func (s *squadServer) effectiveCfgFrom(sess session) config.Config {
	// The session is applied in BOTH modes.
	//
	// It used to be discarded under -persist, on the reasoning that config was then the one
	// store. It is not, yet: saveSession writes the file, and the reader's chip placements,
	// their fifteen and any correction not yet flushed live only in the session. Discarding
	// it meant a block under -persist lit its badge, reported `store: "config"`, left the
	// player in the squad and touched no file — the branch's own defect in the mode with the
	// strongest claim to be durable.
	//
	// There is no double-application: under -persist saveSession clears lock and exclude
	// from the session as it writes them to the file, so exactly one store holds each.
	//
	// ⚠️ forPlanner is NOT applied under -persist.
	//
	// It strips the analysis layer's locks so they do not bind the reader's planner. Under
	// -persist the page writes locks into config.json itself, on the reader's instruction —
	// stripping those would mean the lock button saved a lock to the file and the very next
	// build removed it, which is the button lying twice over.
	base := forPlanner(*s.cfg)
	if s.persist {
		base = *s.cfg
	}
	return sess.applyTo(base, s.engine, s.now().Format("2006-01-02"))
}

// markSessionOverrides flags every override this browser's session owns, so
// the page renders their controls as live — and config-sourced ones as
// disabled, because the session-mode page cannot clear what it does not own.
func markSessionOverrides(p *present.Page, sess session) {
	if len(sess.Lock) == 0 && len(sess.Exclude) == 0 {
		return
	}
	isSession := func(code int) bool {
		for _, c := range sess.Lock {
			if c == code {
				return true
			}
		}
		for _, c := range sess.Exclude {
			if c == code {
				return true
			}
		}
		return false
	}
	for id, ov := range p.Overrides {
		if isSession(ov.Code) {
			ov.Session = true
			p.Overrides[id] = ov
		}
	}
	if p.Reasoning != nil {
		for i := range p.Reasoning.Overrides {
			if isSession(p.Reasoning.Overrides[i].Code) {
				p.Reasoning.Overrides[i].Session = true
			}
		}
	}
	if p.Watch != nil {
		for i := range p.Watch.Excluded {
			if isSession(p.Watch.Excluded[i].Code) {
				p.Watch.Excluded[i].Session = true
			}
		}
	}
}

// readerRequest is the request the enhanced page render should read its
// state from: the POST's `ret` field, which carries the reader's full URI —
// their filters, sort, page and view. Without it the enhanced answer would
// render the action URL's state (defaults) and the morph would silently
// discard what the reader was looking at while the address bar still shows
// it. An absent or unsafe ret falls back to the request itself.

// viewParam reads which tab the page opens on. The watchlist's links carry
// v=watch so a filter, sort or page lands the reader back on the list, not
// on the eleven.

// safeRetPath reports whether a form-supplied redirect target is path-relative.
//
// The obvious check — starts with "/" but not "//" — misses the browsers'
// parsing of a BACKSLASH after the leading slash as the authority delimiter,
// so "\\evil.com" is rejected too, along with control characters that the
// URL parser strips before reading the location. A real page URL never
// contains either.
func safeRetPath(ret string) bool {
	if ret == "" || !strings.HasPrefix(ret, "/") || strings.HasPrefix(ret, "//") {
		return false
	}
	for _, r := range ret {
		if r == '\\' || r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// watchQuery parses the watchlist's filter, sort and page parameters.
//
// Anything unparseable falls back to the default rather than erroring: the
// watchlist is a view, and a bad parameter rendering the default view is the
// right failure — the alternative is a 400 for a hand-edited URL. The sort
// column is validated against the renderer's own set, the direction resolves
// to the column's natural one when absent, and the page is clamped downstream
// in Apply.
func watchQuery(r *http.Request) present.WatchQuery {
	q := present.DefaultWatchQuery()
	vals := r.URL.Query()
	if s := vals.Get("sort"); present.ValidWatchSort(s) {
		q.Sort = s
		q.Desc = present.WatchNaturalDir(s) == "desc"
	}
	if d := vals.Get("dir"); d == "asc" || d == "desc" {
		q.Desc = d == "desc"
	}
	q.Q = strings.TrimSpace(vals.Get("q"))
	switch pos := vals.Get("pos"); pos {
	case "GKP", "DEF", "MID", "FWD":
		q.Pos = pos
	}
	q.Team = strings.ToUpper(strings.TrimSpace(vals.Get("team")))
	if p, err := strconv.Atoi(vals.Get("p")); err == nil {
		q.Page = p
	}
	return q
}

// playerName is the override's display name, from the bootstrap by permanent
// code. WebName rather than FirstName+SecondName, because that is the name the
// page displays on every row — an override named "Bukayo Saka" for a player
// the squad lists as "Saka" reads as a different footballer. The code came
// from the page's own map, so the element always exists; an empty name would
// mean the bootstrap changed under the server, and is carried rather than
// guessed at.
func (s *squadServer) playerName(code int) string {
	if s.engine == nil {
		return ""
	}
	for i := range s.engine.Boot.Elements {
		if s.engine.Boot.Elements[i].Code == code {
			return s.engine.Boot.Elements[i].WebName
		}
	}
	return ""
}

// newServeToken is a per-startup secret that gates the write actions.
//
// The token is printed once in the URL to open, and the page embeds it in its
// action forms. Anything else on the network — or a hostile page open in the
// same browser, which can fire an unauthenticated POST at localhost — does not
// have it, and the write path checks it before touching config.
func newServeToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate serve token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// validateServeAddr accepts only loopback listeners.
//
// The served page can write config.json, so the listener is the outer
// perimeter of that write path. Bound to anything but loopback, the token is
// the only gate between a mutation and every host that can reach the port, and
// a random per-startup token was never meant to carry a perimeter alone. A
// non-loopback bind is always a decision, so refusing it early is the cheap
// half of the threat model.
func validateServeAddr(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("serve -addr must be host:port, got %q: %w", addr, err)
	}
	if loopbackHost(host) {
		return nil
	}
	return fmt.Errorf("serve refuses %q: the page can write config.json, so it binds "+
		"loopback only — use -addr 127.0.0.1:<port>", addr)
}

// loopbackHost reports whether a host is one of the loopback names this
// server will answer for, with or without a port.
//
// It serves both ends of the perimeter: validateServeAddr keeps the listener
// on loopback, and ServeHTTP checks the request's Host against it — the bind
// alone cannot, because a DNS-rebound browser arrives on this socket with a
// foreign Host and reads the answer same-origin.
func loopbackHost(host string) bool {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	switch strings.ToLower(host) {
	case "127.0.0.1", "::1", "localhost":
		return true
	}
	return false
}

// tokenOK compares a submitted token against the server's, in constant time.
//
// The length guard is not an optimisation — ConstantTimeCompare leaks length,
// and length leaks nothing worth having here, so the early return keeps the
// comparison on equal-length inputs where its timing says nothing.
func (s *squadServer) tokenOK(got string) bool {
	// A server with no token grants nothing. Without this, ConstantTimeCompare on two
	// zero-length slices returns 1 and an empty token opens every write route -- and
	// `authed` already guards the same case, so the two would disagree about what an
	// unconfigured server means. cmdServe cannot build one today; the next constructor
	// will not know that.
	if s.token == "" {
		return false
	}
	if len(got) != len(s.token) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) == 1
}
