package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"armband/internal/analysis"
	"armband/internal/config"
	"armband/internal/fpl"
)

// TestServeRefusesANonLoopbackAddr pins the outer perimeter of the write path.
//
// The served page can write config.json, so a listener bound to a network
// interface hands the mutation path to every host that can reach the port,
// behind nothing but a random per-startup token that was never meant to carry
// a perimeter alone. The refusal is the cheap half of the threat model: a
// non-loopback bind is always a decision, so it fails here rather than being
// weighed later.
func TestServeRefusesANonLoopbackAddr(t *testing.T) {
	for _, ok := range []string{"127.0.0.1:8080", "localhost:9999", "[::1]:8080", "127.0.0.1:0"} {
		if err := validateServeAddr(ok); err != nil {
			t.Errorf("loopback %q was refused: %v", ok, err)
		}
	}
	for _, bad := range []string{"0.0.0.0:8080", ":8080", "192.168.1.5:8080", "[::]:8080", "8080"} {
		if err := validateServeAddr(bad); err == nil {
			t.Errorf("non-loopback %q was accepted; it would expose the config write "+
				"path to the network", bad)
		}
	}
}

// TestServeTokenIsPerStartupAndCheckedExactly pins the two facts the token's
// whole job rests on: a fresh server gets a fresh token, and a submitted token
// must match in full — any prefix or suffix difference is a reject.
func TestServeTokenIsPerStartupAndCheckedExactly(t *testing.T) {
	a, err := newServeToken()
	if err != nil {
		t.Fatal(err)
	}
	b, err := newServeToken()
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("two startups drew the same token; the write gate would be reusable")
	}
	if len(a) != 32 { // 16 bytes, hex
		t.Fatalf("token is %d chars, want 32", len(a))
	}

	s := &squadServer{token: a}
	if !s.tokenOK(a) {
		t.Fatal("the correct token was rejected")
	}
	// One char forced to differ: "0"+a[1:] collides with the token itself
	// whenever the token happens to start with a 0.
	mutated := "f" + a[1:]
	if a[0] == 'f' {
		mutated = "0" + a[1:]
	}
	for _, bad := range []string{"", a[:31], a + "0", strings.ToUpper(a), mutated} {
		if s.tokenOK(bad) {
			t.Errorf("token %q was accepted; only the exact token may pass", bad)
		}
	}
}

// The 404 assertion this file used to carry is now TestUnknownRoutesStill404 in
// webroutes_test.go. The reason it exists is unchanged — every unknown path must
// 404 rather than fall through to a handler, because a document served at every
// URL is a second, undiscovered surface for whatever the application later
// learns to do — but it now covers the whole route table rather than the two
// routes that existed when it was written.

// newActionServer builds a squadServer for the action tests: a real config in
// a temp file, a token, and a two-element bootstrap so the codes the tests
// post resolve — the handler refuses a code the bootstrap does not contain,
// because the page only ever posts codes it read from the bootstrap and a
// miss means a stale form.
func newActionServer(t *testing.T) (*squadServer, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	s := &squadServer{
		token:   "tok",
		cfg:     &config.Config{},
		cfgPath: path,
		// The config-write tests run in persist mode: the default session
		// store never touches the file, which is exactly what they assert
		// against.
		persist: true,
		engine: &analysis.Engine{Boot: &fpl.Bootstrap{Elements: []fpl.Element{
			{Code: 456, ID: 1, WebName: "Booted"},
			{Code: 999, ID: 2, WebName: "Other"},
		}}},
	}
	return s, path
}

func postAction(t *testing.T, s *squadServer, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/action", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// httptest.NewRequest stamps Host as example.com, which the handler's
	// loopback gate refuses — the tests speak as a loopback browser.
	req.Host = "127.0.0.1:8080"
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	return w
}

// TestTheActionGateRequiresTheToken pins the whole point of the token: a POST
// without it — which any web page the user has open in their browser can fire
// at localhost — must change nothing, on disk or in memory.
func TestTheActionGateRequiresTheToken(t *testing.T) {
	s, path := newActionServer(t)
	for _, form := range []url.Values{
		{"a": {"boot"}, "c": {"456"}},
		{"t": {"wrong"}, "a": {"boot"}, "c": {"456"}},
		{"a": {"boot"}, "c": {"456"}},
	} {
		if w := postAction(t, s, form); w.Code != 403 {
			t.Errorf("POST %v answered %d, want 403", form, w.Code)
		}
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("a token-less POST wrote the config file")
	}
	if len(s.cfg.Roster.Exclude) != 0 {
		t.Error("a token-less POST changed the in-memory roster")
	}
}

// TestTheActionsWriteAndLiftOverridesByPermanentCode. Boot and lock write a
// page-sourced override keyed by the PERMANENT code, unlock and unboot lift
// exactly one list's entry, the change persists before the redirect fires,
// and the redirect answers with the path the reader acted from.
func TestTheActionsWriteAndLiftOverridesByPermanentCode(t *testing.T) {
	s, path := newActionServer(t)
	const code = "456"

	w := postAction(t, s, url.Values{"t": {"tok"}, "a": {"boot"}, "c": {code},
		"ret": {"/?sort=price&p=2"}})
	if w.Code != 303 {
		t.Fatalf("boot answered %d, want 303", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/?sort=price&p=2" {
		t.Errorf("redirect is %q; the reader loses the view they acted from", loc)
	}
	if len(s.cfg.Roster.Exclude) != 1 || s.cfg.Roster.Exclude[0].Code != 456 {
		t.Fatalf("boot did not exclude by permanent code: %+v", s.cfg.Roster.Exclude)
	}
	o := s.cfg.Roster.Exclude[0]
	if o.Reason != "booted from the squad page" || o.SetOn == "" || o.LastChecked == "" {
		t.Errorf("page override carries the wrong provenance: %+v", o)
	}
	saved, err := config.Load(path)
	if err != nil {
		t.Fatalf("the saved config does not load: %v", err)
	}
	if len(saved.Roster.Exclude) != 1 || saved.Roster.Exclude[0].Code != 456 {
		t.Error("the boot was not persisted before the redirect")
	}

	// Lock the same player: he moves from exclude to lock, never both.
	if w := postAction(t, s, url.Values{"t": {"tok"}, "a": {"lock"}, "c": {code}}); w.Code != 303 {
		t.Fatalf("lock answered %d", w.Code)
	}
	if len(s.cfg.Roster.Exclude) != 0 || len(s.cfg.Roster.Lock) != 1 {
		t.Fatalf("lock left the player on both lists: %+v", s.cfg.Roster)
	}

	// Unlock lifts the lock alone.
	if w := postAction(t, s, url.Values{"t": {"tok"}, "a": {"unlock"}, "c": {code}}); w.Code != 303 {
		t.Fatalf("unlock answered %d", w.Code)
	}
	if len(s.cfg.Roster.Lock) != 0 {
		t.Error("unlock left the lock in place")
	}

	// Re-boot, then unboot from the excluded card.
	_ = postAction(t, s, url.Values{"t": {"tok"}, "a": {"boot"}, "c": {code}})
	if w := postAction(t, s, url.Values{"t": {"tok"}, "a": {"unboot"}, "c": {code}}); w.Code != 303 {
		t.Fatalf("unboot answered %d", w.Code)
	}
	if len(s.cfg.Roster.Exclude) != 0 {
		t.Error("unboot left the exclusion in place")
	}

	// A bad action, a non-numeric code, or a code the bootstrap does not
	// contain is a client error and changes nothing.
	for _, form := range []url.Values{
		{"t": {"tok"}, "a": {"sell"}, "c": {code}},
		{"t": {"tok"}, "a": {"boot"}, "c": {"notanumber"}},
		{"t": {"tok"}, "a": {"boot"}, "c": {"123456"}},
	} {
		if w := postAction(t, s, form); w.Code != 400 {
			t.Errorf("POST %v answered %d, want 400", form, w.Code)
		}
	}

	// An off-site ret is refused; the reader lands on the application. The
	// fallback used to be the root with the token on it, which is now the landing
	// page -- a reader who had just locked a player would have been bounced out to
	// the marketing hero.
	w = postAction(t, s, url.Values{"t": {"tok"}, "a": {"boot"}, "c": {code},
		"ret": {"//evil.example"}})
	if loc := w.Header().Get("Location"); loc != routeApp {
		t.Errorf("an off-site ret was honoured: %q", loc)
	}
	// A backslash ret is the same attack: browsers read "/\\" as the authority
	// delimiter, so it must be refused too.
	w = postAction(t, s, url.Values{"t": {"tok"}, "a": {"boot"}, "c": {code},
		"ret": {`/\evil.example`}})
	if loc := w.Header().Get("Location"); loc != routeApp {
		t.Errorf("a backslash ret was honoured: %q", loc)
	}
}

// TestServeAnswersByLoopbackHostOnly pins the other half of the loopback
// bind. A DNS-rebound browser arrives at this socket with a foreign Host
// header and reads the answer same-origin — the origin, from the browser's
// point of view, is the foreign name — so the Host itself must be refused, or
// the token gate is readable by whichever page arranged the rebinding.
func TestServeAnswersByLoopbackHostOnly(t *testing.T) {
	s := &squadServer{}
	for _, host := range []string{"evil.example", "evil.example:8080", "127.0.0.1.evil.example"} {
		req := httptest.NewRequest("GET", "/nope", nil)
		req.Host = host
		w := httptest.NewRecorder()
		s.ServeHTTP(w, req)
		if w.Code != 403 {
			t.Errorf("Host %q answered %d, want 403", host, w.Code)
		}
	}
	for _, host := range []string{"127.0.0.1:8080", "localhost:8080", "[::1]:8080", "LOCALHOST"} {
		req := httptest.NewRequest("GET", "/nope", nil)
		req.Host = host
		w := httptest.NewRecorder()
		s.ServeHTTP(w, req)
		if w.Code != 404 {
			t.Errorf("loopback Host %q answered %d, want the 404 for /nope", host, w.Code)
		}
	}
}

// TestTheActionClampsTheBodyBeforeTheTokenCheck. The body parse is the one
// thing an unauthenticated caller can make arbitrarily expensive, and it runs
// under the mutex — so a huge body must be refused without ever being parsed
// in full.
func TestTheActionClampsTheBodyBeforeTheTokenCheck(t *testing.T) {
	s, _ := newActionServer(t)
	huge := strings.Repeat("x", 1<<20)
	w := postAction(t, s, url.Values{"t": {"tok"}, "a": {"boot"}, "c": {"456"}, "junk": {huge}})
	if w.Code != 403 && w.Code != 400 {
		t.Fatalf("an oversized body answered %d, want a refusal", w.Code)
	}
	if len(s.cfg.Roster.Exclude) != 0 {
		t.Error("an oversized body was parsed and acted on")
	}
}

// TestWatchQueryParsesAndDefaults. The watchlist parameters are a view, so
// anything unparseable falls back to the opening state rather than erroring —
// but a VALID parameter must arrive exactly as sent, or the sort links and
// the filters the reader set would silently do nothing.
func TestWatchQueryParsesAndDefaults(t *testing.T) {
	q := watchQuery(httptest.NewRequest("GET", "/?sort=score&dir=asc&q=sal&pos=MID&team=ars&p=2", nil))
	if q.Sort != "score" || q.Desc {
		t.Errorf("sort %q desc %v, want score ascending", q.Sort, q.Desc)
	}
	if q.Q != "sal" || q.Pos != "MID" || q.Team != "ARS" {
		t.Errorf("filters %+v, want sal/MID/ARS", q)
	}
	if q.Page != 2 {
		t.Errorf("page %d, want 2", q.Page)
	}

	q = watchQuery(httptest.NewRequest("GET", "/?sort=bogus&pos=XX&team=%20liv%20", nil))
	if q.Sort != "price" || !q.Desc {
		t.Errorf("an unknown sort fell back to %q desc=%v, want the default price descending",
			q.Sort, q.Desc)
	}
	if q.Pos != "" {
		t.Errorf("an unknown position survived as %q", q.Pos)
	}
	if q.Team != "LIV" {
		t.Errorf("a padded team name arrived as %q, want LIV", q.Team)
	}

	q = watchQuery(httptest.NewRequest("GET", "/?sort=name", nil))
	if q.Sort != "name" || q.Desc {
		t.Errorf("a directionless name sort is %q desc=%v; names open ascending",
			q.Sort, q.Desc)
	}
}

// TestTheSessionStoreWritesACookieNotTheConfig pins the default: the page's
// overrides live in a browser-session cookie, and config.json stays a
// default. Persisting is the opt-in, not the baseline.
func TestTheSessionStoreWritesACookieNotTheConfig(t *testing.T) {
	s, path := newActionServer(t)
	s.persist = false

	// Boot in session mode: a cookie comes back, the config file is never
	// written.
	w := postAction(t, s, url.Values{"t": {"tok"}, "a": {"boot"}, "c": {"456"}})
	if w.Code != 303 {
		t.Fatalf("boot answered %d, want 303", w.Code)
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != sessionCookieName {
		t.Fatalf("the session store is not in the response cookies: %+v", cookies)
	}
	if cookies[0].HttpOnly != true || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Errorf("the session cookie is not hardened: %+v", cookies[0])
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("a session-mode action wrote config.json")
	}
	if len(s.cfg.Roster.Exclude) != 0 {
		t.Error("a session-mode action changed the in-memory config")
	}

	// The cookie decodes to the booted code.
	raw, err := base64.StdEncoding.DecodeString(cookies[0].Value)
	if err != nil {
		t.Fatalf("the cookie value is not base64: %v", err)
	}
	var sess session
	if err := json.Unmarshal(raw, &sess); err != nil {
		t.Fatalf("the cookie value is not session JSON: %v", err)
	}
	if len(sess.Exclude) != 1 || sess.Exclude[0] != 456 {
		t.Errorf("the cookie holds %+v, want the booted code 456", sess)
	}

	// Lock the same player: he moves from exclude to lock in the cookie, and
	// the next action carries the running session along (the request cookie).
	req := httptest.NewRequest("POST", "/action", strings.NewReader(
		url.Values{"t": {"tok"}, "a": {"lock"}, "c": {"456"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = "127.0.0.1:8080"
	req.AddCookie(cookies[0])
	w = httptest.NewRecorder()
	s.ServeHTTP(w, req)
	cookies = w.Result().Cookies()
	raw, _ = base64.StdEncoding.DecodeString(cookies[0].Value)
	// A FRESH struct. json.Unmarshal leaves fields absent from the JSON untouched, so
	// decoding into the previous value would keep the old Exclude list -- omitempty drops
	// the now-empty one from the wire, and the stale value survives to be asserted on.
	var afterLock session
	if err := json.Unmarshal(raw, &afterLock); err != nil {
		t.Fatalf("the cookie value is not session JSON: %v", err)
	}
	if len(afterLock.Lock) != 1 || len(afterLock.Exclude) != 0 {
		t.Errorf("the cookie after lock holds %+v, want the code locked only", afterLock)
	}

	// Unlock empties the store, and the response clears the cookie rather
	// than carrying an empty one.
	req = httptest.NewRequest("POST", "/action", strings.NewReader(
		url.Values{"t": {"tok"}, "a": {"unlock"}, "c": {"456"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = "127.0.0.1:8080"
	req.AddCookie(cookies[0])
	w = httptest.NewRecorder()
	s.ServeHTTP(w, req)
	cookies = w.Result().Cookies()
	if len(cookies) != 1 || cookies[0].MaxAge >= 0 {
		t.Errorf("an empty session should clear the cookie, got %+v", cookies)
	}
}

// TestTheSessionOverridesRideOnTopOfTheConfig. The effective config is the
// real one plus the session: a session boot clears a config lock for the
// page (never the file), and a session lock survives beside untouched
// config lists.
func TestTheSessionOverridesRideOnTopOfTheConfig(t *testing.T) {
	s, _ := newActionServer(t)
	s.persist = false
	s.cfg.Roster.Lock = []config.RosterOverride{{Code: 999, Name: "Other"}}

	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "127.0.0.1:8080"
	raw, _ := json.Marshal(session{Version: sessionVersion, Exclude: []int{999}, Lock: []int{456}})
	req.AddCookie(&http.Cookie{Name: sessionCookieName,
		Value: base64.StdEncoding.EncodeToString(raw)})

	cfg := s.effectiveCfg(req)
	if len(cfg.Roster.Exclude) != 1 || cfg.Roster.Exclude[0].Code != 999 {
		t.Errorf("a session boot did not clear the config lock for the page: %+v",
			cfg.Roster.Exclude)
	}
	if len(cfg.Roster.Lock) != 1 || cfg.Roster.Lock[0].Code != 456 {
		t.Errorf("a session lock did not join the effective config: %+v", cfg.Roster.Lock)
	}
	if len(s.cfg.Roster.Lock) != 1 || s.cfg.Roster.Lock[0].Code != 999 {
		t.Error("the session store mutated the real config")
	}
}

// The enhanced-answer test that stood here is gone with the thing it tested.
//
// /action could answer with a freshly rendered page for the page's own script to morph
// into place, and the finding it pinned was that the render had to read the READER's
// filters and sort out of the POSTed ret rather than the action URL. There is no
// server-rendered page any more -- the application re-fetches /api/state -- so the
// handler always answers a 303 and there is no second request to mis-read.
//
// The open-redirect half of that finding survives, in safeRetPath, and is still tested
// by TestTheActionsWriteAndLiftOverridesByPermanentCode.

// TestTheActionSavesBeforeItAdopts. A save failure must leave the running
// config untouched — adopting first would show an override on the page that
// every later run silently lacks, which is the page lying about the state of
// the world.
func TestTheActionSavesBeforeItAdopts(t *testing.T) {
	// A FILE where the config's directory should be: Save's MkdirAll cannot
	// turn it into a directory, so the save fails for certain.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	s := &squadServer{
		token: "tok", cfg: &config.Config{}, cfgPath: filepath.Join(blocker, "config.json"),
		persist: true,
		engine: &analysis.Engine{Boot: &fpl.Bootstrap{Elements: []fpl.Element{
			{Code: 456, ID: 1, WebName: "Booted"},
		}}},
	}
	if w := postAction(t, s, url.Values{"t": {"tok"}, "a": {"boot"}, "c": {"456"}}); w.Code != 500 {
		t.Fatalf("an unsaveable path answered %d, want 500", w.Code)
	}
	if len(s.cfg.Roster.Exclude) != 0 {
		t.Error("a failed save was adopted into the running config")
	}
}
