package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"armband/internal/viewmodel"
)

// getChipTeams issues GET /api/wildcard, optionally with a session cookie.
func getChipTeams(t *testing.T, s *squadServer, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", routeWildcardState, nil)
	req.Host = "127.0.0.1:8080"
	if cookie != nil {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	return w
}

// TestChipTeamsRejectsWrongMethod pins the 405 half of the route's contract.
func TestChipTeamsRejectsWrongMethod(t *testing.T) {
	s := fixtureServer(t)
	s.wildcardEnabled = true // this whole file is /api/wildcard's own contract
	req := httptest.NewRequest("POST", routeWildcardState, nil)
	req.Host = "127.0.0.1:8080"
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST %s answered %d, want 405", routeWildcardState, w.Code)
	}
	if got := w.Header().Get("Allow"); got != "GET" {
		t.Errorf("Allow header = %q, want GET", got)
	}
}

// TestChipTeamsAnswers409WithNoOpenGameweek pins the "season over" state: with
// the clock pinned past every gameweek's own deadline, nextOpenEvent finds
// nothing to project, and that is a fact about the world rather than a
// server error.
func TestChipTeamsAnswers409WithNoOpenGameweek(t *testing.T) {
	s := fixtureServer(t)
	s.wildcardEnabled = true // this whole file is /api/wildcard's own contract
	var latest time.Time
	for _, e := range s.engine.Boot.Events {
		if e.DeadlineTime.After(latest) {
			latest = e.DeadlineTime
		}
	}
	s.clock = func() time.Time { return latest.Add(time.Hour) }

	w := getChipTeams(t, s, nil)
	if w.Code != http.StatusConflict {
		t.Errorf("GET %s with no open gameweek answered %d, want 409: %s",
			routeWildcardState, w.Code, w.Body.String())
	}
}

// TestChipTeamsAnswers500OnABuildFailure forces buildSquadPage's own
// AssemblyBudget call to fail -- a configured entry whose season has started
// but carries no priced squad -- and pins that the route answers a fixed
// 500 sentence rather than a stack trace or a wrong squad.
func TestChipTeamsAnswers500OnABuildFailure(t *testing.T) {
	s := fixtureServer(t)
	s.wildcardEnabled = true // this whole file is /api/wildcard's own contract
	s.cfg.EntryID = 2785902
	s.engine.Entry = 2785902
	if len(s.engine.Fixtures) == 0 {
		t.Fatal("fixture engine has no fixtures to force SeasonHasStarted on")
	}
	s.engine.Fixtures[0].Started = true

	w := getChipTeams(t, s, nil)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("GET %s with an unpriceable entry answered %d, want 500: %s",
			routeWildcardState, w.Code, w.Body.String())
	}
}

// TestChipTeamsGoodRequestAnswersTheExpectedShape is the end-to-end success
// path against the committed capture, whose bootstrap opens the wildcard and
// free hit at gameweek 2 (see analysis.PlayableChips) -- so at the fixture's
// pinned clock (before GW1's own deadline) BOTH chips are state 2, and the
// document must say so rather than draw a fifteen the competition would
// refuse.
func TestChipTeamsGoodRequestAnswersTheExpectedShape(t *testing.T) {
	s := fixtureServer(t)
	s.wildcardEnabled = true // this whole file is /api/wildcard's own contract

	w := getChipTeams(t, s, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s answered %d, want 200: %s", routeWildcardState, w.Code, w.Body.String())
	}
	if got := w.Header().Get("Cache-Control"); got != "public, max-age=300" {
		t.Errorf("Cache-Control = %q, want %q", got, "public, max-age=300")
	}

	var st viewmodel.State
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatalf("the response did not decode: %v", err)
	}
	if st.ChipTeams == nil {
		t.Fatal("State.ChipTeams is nil, want a value")
	}
	ct := st.ChipTeams
	if ct.Event != s.engine.Boot.Events[0].ID {
		t.Errorf("ChipTeams.Event = %d, want gameweek 1", ct.Event)
	}
	if ct.Wildcard != nil || ct.WildcardUnavailable == "" {
		t.Errorf("gameweek 1: wildcard = %+v, unavailable = %q — want nil team and a stated absence",
			ct.Wildcard, ct.WildcardUnavailable)
	}
	if ct.FreeHit != nil || ct.FreeHitUnavailable == "" {
		t.Errorf("gameweek 1: free hit = %+v, unavailable = %q — want nil team and a stated absence",
			ct.FreeHit, ct.FreeHitUnavailable)
	}
}

// TestChipTeamsIsNotSessionScoped pins §4.1's contract: this document reads
// no session at all, so two different readers' cookies must answer
// byte-identical documents -- the same test shape TestResultsIsSessionScoped
// (the opposite assertion) would use for /api/results.
func TestChipTeamsIsNotSessionScoped(t *testing.T) {
	s := fixtureServer(t)
	s.wildcardEnabled = true // this whole file is /api/wildcard's own contract
	cookieA := resultsSessionCookie(t, 111)
	cookieB := resultsSessionCookie(t, 222)

	wa := getChipTeams(t, s, cookieA)
	wb := getChipTeams(t, s, cookieB)
	if wa.Code != http.StatusOK || wb.Code != http.StatusOK {
		t.Fatalf("got statuses %d/%d, want 200/200: %s / %s",
			wa.Code, wb.Code, wa.Body.String(), wb.Body.String())
	}
	if !bytes.Equal(wa.Body.Bytes(), wb.Body.Bytes()) {
		t.Error("GET /api/wildcard answered two different documents for two different " +
			"session cookies -- this route must not read the session at all")
	}
}

// TestPersistingACorrectionInvalidatesTheChipCache pins §5.4's one
// invalidation point: persistCorrections replaces s.cfg under -persist, and
// without invalidating the cache the wildcard page would keep recommending a
// player the operator just blocked until the next deadline changes the
// cache's key.
func TestPersistingACorrectionInvalidatesTheChipCache(t *testing.T) {
	s := fixtureServer(t)
	s.wildcardEnabled = true // this whole file is /api/wildcard's own contract
	s.persist = true
	s.cfgPath = filepath.Join(t.TempDir(), "config.json")

	// Prime the cache as if a build had already run.
	s.chips = &chipCache{event: s.engine.Boot.Events[0].ID}

	in := session{Version: sessionVersion, Lock: []int{s.engine.Boot.Elements[2].Code}}
	if _, err := s.persistCorrections(in); err != nil {
		t.Fatalf("persistCorrections: %v", err)
	}
	if s.chips != nil {
		t.Error("persistCorrections did not invalidate the chip cache")
	}
}

// TestNextOpenEventMatchesTheRailsCurrent pins nextOpenEvent's own warning: it
// restates the same rule viewmodel.buildGameweeks computes inline for
// Gameweek.Current, and the two must never drift apart.
func TestNextOpenEventMatchesTheRailsCurrent(t *testing.T) {
	s := fixtureServer(t)
	s.wildcardEnabled = true // this whole file is /api/wildcard's own contract
	req := httptest.NewRequest("GET", routeArmbandTeamState, nil)
	req.Host = "127.0.0.1:8080"
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s answered %d, want 200: %s", routeArmbandTeamState, w.Code, w.Body.String())
	}

	var st viewmodel.State
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatalf("the response did not decode: %v", err)
	}
	var railCurrent int
	found := false
	for _, gw := range st.Gameweeks {
		if gw.Current {
			railCurrent, found = gw.Number, true
			break
		}
	}
	if !found {
		t.Fatal("no gameweek on the rail is marked Current")
	}

	want := nextOpenEvent(s.engine.Boot, s.now())
	if want == nil {
		t.Fatal("nextOpenEvent returned nil, want a gameweek")
	}
	if railCurrent != want.ID {
		t.Errorf("rail's Current gameweek = %d, nextOpenEvent = %d, want equal", railCurrent, want.ID)
	}
}

// TestChipTeamsDoesNotPublishOurOwnChipPlan pins a privacy boundary rather than a
// behaviour: /api/wildcard is ungated, so anything on it is public.
//
// It used to carry plan_wildcard_gw and plan_free_hit_gw, straight from
// cfg.Chips — the gameweeks this account intends to play its wildcard and free
// hit. Observed live on 2026-08-24 returning 6 and 16 to anyone who asked. No
// page ever rendered them, so it was strategy published for nothing.
//
// The distinction worth keeping: PLAYED chips may stay. FPL publishes those
// itself on the entry's own history, so repeating them reveals nothing. What a
// manager INTENDS is not public anywhere else, and this is the surface that
// would have made it so.
func TestChipTeamsDoesNotPublishOurOwnChipPlan(t *testing.T) {
	s := fixtureServer(t)
	s.wildcardEnabled = true

	w := getChipTeams(t, s, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200", routeWildcardState, w.Code)
	}

	var raw map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decoding the response: %v", err)
	}
	body := w.Body.String()

	for _, leak := range []string{"plan_wildcard_gw", "plan_free_hit_gw"} {
		if bytes.Contains(w.Body.Bytes(), []byte(leak)) {
			t.Errorf("%s is on the public wildcard payload.\n"+
				"That is which gameweek this account plans to play the chip in, on an "+
				"ungated route, and no page renders it. See this test's comment.\n"+
				"body: %.400s", leak, body)
		}
	}
}
