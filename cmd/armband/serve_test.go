package main

import (
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"armband/internal/config"
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
	for _, bad := range []string{"", a[:31], a + "0", strings.ToUpper(a), "0" + a[1:]} {
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
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/other", nil))
	if w.Code != 404 {
		t.Fatalf("GET /other answered %d, want 404", w.Code)
	}
}

// newActionServer builds a squadServer for the action tests: a real config in
// a temp file, a token, and no engine — the action path never touches the
// engine, and a nil one failing would be the test doing its job.
func newActionServer(t *testing.T) (*squadServer, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	s := &squadServer{token: "tok", cfg: &config.Config{}, cfgPath: path}
	return s, path
}

func postAction(t *testing.T, s *squadServer, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/action", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
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

	// A bad action or a non-numeric code is a client error and changes nothing.
	for _, form := range []url.Values{
		{"t": {"tok"}, "a": {"sell"}, "c": {code}},
		{"t": {"tok"}, "a": {"boot"}, "c": {"notanumber"}},
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
	s := &squadServer{token: "tok", cfg: &config.Config{}, cfgPath: filepath.Join(blocker, "config.json")}
	if w := postAction(t, s, url.Values{"t": {"tok"}, "a": {"boot"}, "c": {"456"}}); w.Code != 500 {
		t.Fatalf("an unsaveable path answered %d, want 500", w.Code)
	}
	if len(s.cfg.Roster.Exclude) != 0 {
		t.Error("a failed save was adopted into the running config")
	}
}
