package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"armband/internal/analysis"
	"armband/internal/config"
	"armband/internal/fpl"
	"armband/internal/present"
)

// cmdServe hosts the squad page over HTTP.
//
// It is the same page `armband squad -html` writes to disk, built by the same
// pipeline (buildSquadPage) — the difference is that it is live, and once the
// page carries write actions those actions POST back to this listener.
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
func cmdServe(ctx context.Context, cfg config.Config, cfgPath string,
	client *fpl.Client, e *analysis.Engine, weeks int) error {

	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", "127.0.0.1:8080", "address to serve on (loopback only)")
	persist := fs.Bool("persist", false, "write lock/boot overrides back to config.json; "+
		"without it they live in a browser-session cookie")
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

	s := &squadServer{
		token:   token,
		cfg:     &cfg,
		cfgPath: cfgPath,
		client:  client,
		engine:  e,
		weeks:   weeks,
		persist: *persist,
	}

	fmt.Fprintf(os.Stderr, "\n%s\n", dim("Serving the squad page on http://"+*addr+"/?t="+token))
	fmt.Fprintf(os.Stderr, "%s\n", dim("Open that exact URL — the token gates the page's write actions."))
	if *persist {
		fmt.Fprintf(os.Stderr, "%s\n", dim("Overrides write back to "+cfgPath+" (-persist)."))
	} else {
		fmt.Fprintf(os.Stderr, "%s\n", dim("Overrides live in a browser-session cookie — "+
			"config.json is untouched. Run serve -persist to write them back instead."))
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
	return srv.ListenAndServe()
}

// squadServer is the whole server: the page and the write actions.
type squadServer struct {
	mu    sync.Mutex
	token string
	cfg   *config.Config
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
}

func (s *squadServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// The Host check is the other half of the loopback bind. A browser that
	// has been DNS-rebound arrives at this socket with a foreign Host header,
	// and from that browser's point of view the ORIGIN is the foreign name —
	// so same-origin policy hands the answer to the page the attacker
	// controls, token and all. Rejecting the Host is what keeps the token
	// readable only by pages whose origin really is this listener.
	if !loopbackHost(r.Host) {
		http.Error(w, "unrecognised host", http.StatusForbidden)
		return
	}
	switch r.URL.Path {
	case "/":
		s.page(w, r)
	case "/action":
		s.action(w, r)
	default:
		http.NotFound(w, r)
	}
}

// page renders the full squad page, rebuilt from scratch for this request.
func (s *squadServer) page(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.render(w, r, s.effectiveCfg(r), readSessionOverrides(r))
}

// render writes the squad page for one request: the given config (already
// merged with the session when not persisting) and session state for the
// ownership marking, stamped with the token, query, view and return path.
//
// Both GET / and an enhanced action POST land here — the enhanced response
// must be the page the action produced, which is how the page updates in
// place without relying on a just-set cookie surviving the redirect.
func (s *squadServer) render(w http.ResponseWriter, r *http.Request, cfg config.Config, so sessionOverrides) {
	b, err := buildSquadPage(r.Context(), cfg, s.client, s.engine, s.weeks, true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	b.Page.Token = s.token
	b.Page.WatchQuery = watchQuery(r)
	b.Page.View = viewParam(r)
	b.Page.SessionMode = !s.persist
	if !s.persist {
		markSessionOverrides(&b.Page, so)
	}
	// The whole request URI, not just the query: the redirect after an action
	// must be path-relative, and a bare query would be rejected by the
	// path-prefix check in action as an open redirect.
	b.Page.Ret = r.URL.RequestURI()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := present.Render(w, b.Page); err != nil {
		// The template is static text compiled once; an execution error is a
		// programming error, and it arrives after the response has started, so
		// the only honest report is to the server's own stderr.
		fmt.Fprintf(os.Stderr, "render: %v\n", err)
	}
}

// action applies one lock or boot and answers with a redirect, so the browser
// re-fetches the page and the squad regenerates with the override in force.
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
	s.mu.Lock()
	defer s.mu.Unlock()

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
	today := time.Now().Format("2006-01-02")
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
		err = next.Roster.Set("lock", pageOverride(name, "locked from the squad page"))
	case "boot":
		err = next.Roster.Set("exclude", pageOverride(name, "booted from the squad page"))
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

	// The enhanced path (the page's script) wants the answer to BE the fresh
	// page, built from the action's own result — no redirect, no reliance on
	// a just-set cookie surviving one. A plain form submission still gets the
	// 303, where browsers apply the cookie before following.
	enhanced := r.PostFormValue("enhanced") == "1"

	if s.persist {
		// Saved before adopted: a failed save leaves the server and the
		// config exactly as they were.
		if s.cfgPath == "" {
			http.Error(w, "no config path — the override was not saved", http.StatusInternalServerError)
			return
		}
		if err := config.Save(s.cfgPath, next); err != nil {
			http.Error(w, fmt.Sprintf("saving config: %v", err), http.StatusInternalServerError)
			return
		}
		s.cfg = &next
		if enhanced {
			s.render(w, r, next, sessionOverrides{})
			return
		}
	} else {
		// The default store is the browser session: the cookie mutates, the
		// config file stays a default. Session overrides ride on top of the
		// config for every build of this page, and they die with the browser
		// session.
		so := readSessionOverrides(r).apply(act, code)
		if err := so.setCookie(w); err != nil {
			http.Error(w, fmt.Sprintf("encoding the session overrides: %v", err),
				http.StatusInternalServerError)
			return
		}
		if enhanced {
			s.render(w, r, s.effectiveCfgFrom(so), so)
			return
		}
	}

	// Back to the view the reader acted from, token included. The redirect is
	// what makes the page regenerate: the browser now GETs it fresh, with the
	// override already in force.
	ret := r.PostFormValue("ret")
	if !safeRetPath(ret) {
		ret = "/?t=" + s.token
	}
	http.Redirect(w, r, ret, http.StatusSeeOther)
}

// sessionOverrides is the lock/boot state of ONE browser session — the
// page's default store. Codes only: the bootstrap is authoritative for
// names, and a code that stops resolving is dropped at build time rather
// than carried. It is deliberately not the config.RosterOverride struct —
// this store has no reasons and no dates, it is a scratch surface for one
// reader's session, not the standing record.
type sessionOverrides struct {
	Lock    []int `json:"lock"`
	Exclude []int `json:"exclude"`
}

// overrideCookieName is the session cookie carrying the overrides. The value
// is base64 JSON because a bare JSON array contains commas, which the cookie
// grammar forbids; base64's alphabet is entirely cookie-safe.
const overrideCookieName = "fpl_overrides"

func readSessionOverrides(r *http.Request) sessionOverrides {
	c, err := r.Cookie(overrideCookieName)
	if err != nil {
		return sessionOverrides{}
	}
	raw, err := base64.StdEncoding.DecodeString(c.Value)
	if err != nil {
		return sessionOverrides{}
	}
	var so sessionOverrides
	if err := json.Unmarshal(raw, &so); err != nil {
		return sessionOverrides{}
	}
	return so
}

// setCookie writes the session store, or clears the cookie when the session
// holds nothing — an empty override set and a dead cookie must not read
// differently on the next request.
func (so sessionOverrides) setCookie(w http.ResponseWriter) error {
	if len(so.Lock) == 0 && len(so.Exclude) == 0 {
		http.SetCookie(w, &http.Cookie{Name: overrideCookieName, Path: "/", MaxAge: -1})
		return nil
	}
	raw, err := json.Marshal(so)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     overrideCookieName,
		Value:    base64.StdEncoding.EncodeToString(raw),
		Path:     "/",
		SameSite: http.SameSiteStrictMode,
		// HttpOnly: the page's script never reads the store — it posts
		// actions and the server answers with the next render.
		HttpOnly: true,
	})
	return nil
}

// apply mirrors config.Roster.Set/Remove semantics on the session store:
// lock and boot are mutually exclusive, unlock and unboot lift one list.
func (so sessionOverrides) apply(action string, code int) sessionOverrides {
	drop := func(codes []int, code int) []int {
		out := codes[:0:0]
		for _, c := range codes {
			if c != code {
				out = append(out, c)
			}
		}
		return out
	}
	add := func(codes []int, code int) []int {
		for _, c := range codes {
			if c == code {
				return codes
			}
		}
		return append(codes, code)
	}
	switch action {
	case "lock":
		so.Exclude = drop(so.Exclude, code)
		so.Lock = add(so.Lock, code)
	case "boot":
		so.Lock = drop(so.Lock, code)
		so.Exclude = add(so.Exclude, code)
	case "unlock":
		so.Lock = drop(so.Lock, code)
	case "unboot":
		so.Exclude = drop(so.Exclude, code)
	}
	return so
}

// effectiveCfg is the config this request builds the page from: the real
// config with the session's overrides applied on top. Session wins on
// conflict — a session boot clears a config lock for THIS page, never the
// file — so the page's controls always express the reader's latest decision
// without touching config.json. In persist mode the session is ignored and
// the config is the one store.
func (s *squadServer) effectiveCfg(r *http.Request) config.Config {
	return s.effectiveCfgFrom(readSessionOverrides(r))
}

func (s *squadServer) effectiveCfgFrom(so sessionOverrides) config.Config {
	cfg := *s.cfg
	if s.persist {
		return cfg
	}
	if len(so.Lock) == 0 && len(so.Exclude) == 0 {
		return cfg
	}
	today := time.Now().Format("2006-01-02")
	entry := func(code int, mode, name string) config.RosterOverride {
		reason := "locked from the squad page — browser session"
		if mode == "exclude" {
			reason = "booted from the squad page — browser session"
		}
		return config.RosterOverride{
			Code: code, Name: name, Reason: reason, SetOn: today, LastChecked: today,
		}
	}
	// A code the bootstrap no longer contains is dropped: the session is a
	// scratch surface, and a nameless override would litter the page exactly
	// as it would litter config.
	for _, code := range so.Exclude {
		if name := s.playerName(code); name != "" {
			_ = cfg.Roster.Set("exclude", entry(code, "exclude", name))
		}
	}
	for _, code := range so.Lock {
		if name := s.playerName(code); name != "" {
			_ = cfg.Roster.Set("lock", entry(code, "lock", name))
		}
	}
	return cfg
}

// markSessionOverrides flags every override this browser's session owns, so
// the page renders their controls as live — and config-sourced ones as
// disabled, because the session-mode page cannot clear what it does not own.
func markSessionOverrides(p *present.Page, so sessionOverrides) {
	if len(so.Lock) == 0 && len(so.Exclude) == 0 {
		return
	}
	isSession := func(code int) bool {
		for _, c := range so.Lock {
			if c == code {
				return true
			}
		}
		for _, c := range so.Exclude {
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

// viewParam reads which tab the page opens on. The watchlist's links carry
// v=watch so a filter, sort or page lands the reader back on the list, not
// on the eleven.
func viewParam(r *http.Request) string {
	switch r.URL.Query().Get("v") {
	case "team", "why", "watch":
		return r.URL.Query().Get("v")
	}
	return ""
}

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
	if len(got) != len(s.token) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) == 1
}
