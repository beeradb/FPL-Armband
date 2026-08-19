package main

import (
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

// TestServeAnswers404OffTheTwoRoutes. Every other path must 404 rather than
// fall through to the page — a served page at every URL would be a second,
// undiscovered surface for whatever the page later learns to do.
func TestServeAnswers404OffTheTwoRoutes(t *testing.T) {
	s := &squadServer{}
	req := httptest.NewRequest("GET", "/other", nil)
	req.Host = "127.0.0.1:8080"
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Fatalf("GET /other answered %d, want 404", w.Code)
	}
}

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

	// An off-site ret is refused; the reader lands on the root with the token.
	w = postAction(t, s, url.Values{"t": {"tok"}, "a": {"boot"}, "c": {code},
		"ret": {"//evil.example"}})
	if loc := w.Header().Get("Location"); loc != "/?t=tok" {
		t.Errorf("an off-site ret was honoured: %q", loc)
	}
	// A backslash ret is the same attack: browsers read "/\\" as the authority
	// delimiter, so it must be refused too.
	w = postAction(t, s, url.Values{"t": {"tok"}, "a": {"boot"}, "c": {code},
		"ret": {`/\evil.example`}})
	if loc := w.Header().Get("Location"); loc != "/?t=tok" {
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
