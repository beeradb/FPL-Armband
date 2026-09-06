package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"armband/internal/analysis"
	"armband/internal/config"
	"armband/internal/fpl"
	"armband/internal/viewmodel"
)

// resultsSessionCookie builds a session cookie carrying only an imported entry id — the
// one thing GET /api/results reads off the session — without going through the whole
// PUT /api/import machinery.
func resultsSessionCookie(t *testing.T, entry int) *http.Cookie {
	t.Helper()
	w := httptest.NewRecorder()
	sess := session{Version: sessionVersion, Entry: entry}
	if err := sess.write(w); err != nil {
		t.Fatal(err)
	}
	return sessionCookie(t, w)
}

// getResults issues GET /api/results?<query>, optionally with a session cookie.
func getResults(t *testing.T, s *squadServer, query string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", routeResults+query, nil)
	req.Host = "127.0.0.1:8080"
	if cookie != nil {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	return w
}

func TestResultsRejectsWrongMethod(t *testing.T) {
	s := fixtureServer(t)
	req := httptest.NewRequest("PUT", routeResults+"?gw=1", nil)
	req.Host = "127.0.0.1:8080"
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT /api/results answered %d, want 405", w.Code)
	}
	if got := w.Header().Get("Allow"); got != "GET" {
		t.Errorf("Allow header = %q, want GET", got)
	}
}

// TestResultsAnswers409WithNoEntryImported pins the design's first rule: with no session
// cookie at all — the same as a reader who has never imported a team — there is nothing
// to show a result for, and that is a fact about the request, not a server error.
func TestResultsAnswers409WithNoEntryImported(t *testing.T) {
	s := fixtureServer(t)
	w := getResults(t, s, "?gw=1", nil)
	if w.Code != http.StatusConflict {
		t.Errorf("GET /api/results with no imported entry answered %d, want 409: %s",
			w.Code, w.Body.String())
	}
}

// TestResultsAnswers400ForABadGameweek covers both halves of the design's single 400
// rule: a gw that does not parse as a positive integer, and one that parses fine but does
// not exist in this bootstrap.
func TestResultsAnswers400ForABadGameweek(t *testing.T) {
	s := fixtureServer(t)
	cookie := resultsSessionCookie(t, 1234567)
	for _, query := range []string{"?gw=abc", "?gw=0", "?gw=-1", "?gw=", "?gw=999"} {
		w := getResults(t, s, query, cookie)
		if w.Code != http.StatusBadRequest {
			t.Errorf("GET /api/results%s answered %d, want 400: %s", query, w.Code, w.Body.String())
		}
	}
}

// TestResultsAnswers409BeforeTheDeadline pins the design's second 409 rule: FPL serves no
// picks until a gameweek's deadline has passed, and the fixture's pinned clock (two days
// before GW1's own deadline) makes every gameweek in the capture exactly that case by
// construction — the same premise TestTheFixtureMatchesWhatProductionBuildsAtGW1 asserts
// for the engine. fetchEntry/fetchPicks are set to fail the test if ever called, so this
// also pins that the deadline gate runs before any network call.
func TestResultsAnswers409BeforeTheDeadline(t *testing.T) {
	s := fixtureServer(t)
	s.fetchEntry = func(ctx context.Context, id int) (*fpl.Entry, error) {
		t.Fatal("apiResults fetched an entry with the gameweek's deadline still open")
		return nil, nil
	}
	s.fetchPicks = func(ctx context.Context, entryID, event int) (*fpl.EntryPicks, error) {
		t.Fatal("apiResults fetched picks with the gameweek's deadline still open")
		return nil, nil
	}
	cookie := resultsSessionCookie(t, 1234567)
	w := getResults(t, s, "?gw=1", cookie)
	if w.Code != http.StatusConflict {
		t.Errorf("GET /api/results?gw=1 before its deadline answered %d, want 409: %s",
			w.Code, w.Body.String())
	}
}

// TestResultsGoodRequestAnswersTheExpectedShape is the end-to-end success path: a session
// with an imported entry, a gameweek whose deadline has passed, a fake entry and a fake
// legal fifteen returned through the fetchEntry/fetchPicks seams (see squadServer's own
// doc comment on why those exist — a test needs no live FPL client). No fetchLive is
// wired, which is the honest degrade s.client == nil already gives houseLiveSources: the
// live overlay stays empty and ResultState stays "", which is itself pinned below via the
// no-store cache header that path takes.
func TestResultsGoodRequestAnswersTheExpectedShape(t *testing.T) {
	s := fixtureServer(t)
	event := s.engine.Boot.Events[0] // GW1, deadline 2026-08-21 17:30 UTC
	s.clock = func() time.Time { return event.DeadlineTime.Add(time.Hour) }

	gk, def, mid, fwd := legalFifteenElements(t, s.engine.Boot)
	picks := fakePicks(gk, def, mid, fwd)
	s.fetchEntry = func(ctx context.Context, id int) (*fpl.Entry, error) {
		if id != 1234567 {
			t.Errorf("fetchEntry asked for id %d, want the session's 1234567", id)
		}
		return &fpl.Entry{ID: id, Name: "Test Team", SummaryOverallPoints: 71}, nil
	}
	s.fetchPicks = func(ctx context.Context, entryID, gw int) (*fpl.EntryPicks, error) {
		if gw != event.ID {
			t.Errorf("fetchPicks asked for gameweek %d, want %d", gw, event.ID)
		}
		return picks, nil
	}
	cookie := resultsSessionCookie(t, 1234567)

	w := getResults(t, s, "?gw=1", cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/results?gw=1 answered %d, want 200: %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store — result_state is not final with no "+
			"live client configured", got)
	}

	var st viewmodel.State
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatalf("the response did not decode: %v", err)
	}
	if st.Results == nil {
		t.Fatal("State.Results is nil, want a value — a fake entry and picks were given")
	}
	if st.Results.OverallPoints != 71 {
		t.Errorf("Results.OverallPoints = %d, want 71", st.Results.OverallPoints)
	}
	if st.Results.ResultEvent != event.ID {
		t.Errorf("Results.ResultEvent = %d, want %d (the requested gw)", st.Results.ResultEvent, event.ID)
	}
	if len(st.Results.XI) != 11 || len(st.Results.Bench) != 4 {
		t.Errorf("Results XI/Bench = %d/%d, want 11/4", len(st.Results.XI), len(st.Results.Bench))
	}
	if st.Results.Formation == "" {
		t.Error("Results.Formation is empty, want the fifteen's shape")
	}
}

// TestResultsAnswers409WhenTheEntryFetchFails pins the "no fallback" rule the design
// states explicitly: an entry this process could not reach must answer with a status, not
// silently render buildSquadPage's optimum captioned as this gameweek's result.
func TestResultsAnswers409WhenTheEntryFetchFails(t *testing.T) {
	s := fixtureServer(t)
	event := s.engine.Boot.Events[0]
	s.clock = func() time.Time { return event.DeadlineTime.Add(time.Hour) }
	s.fetchEntry = func(ctx context.Context, id int) (*fpl.Entry, error) {
		return nil, notFoundErr()
	}
	s.fetchPicks = func(ctx context.Context, entryID, gw int) (*fpl.EntryPicks, error) {
		t.Fatal("apiResults fetched picks after the entry fetch failed")
		return nil, nil
	}
	cookie := resultsSessionCookie(t, 1234567)
	w := getResults(t, s, "?gw=1", cookie)
	if w.Code != http.StatusConflict {
		t.Errorf("GET /api/results with a failed entry fetch answered %d, want 409: %s",
			w.Code, w.Body.String())
	}
}

// TestResultsUsesTheVisitorsOwnEffectiveConfig pins the fix to apiResults' config leak:
// GET /api/results used to build the page from the raw site-owner config (*s.cfg) rather
// than the visiting reader's own effective config (s.effectiveCfgFrom(sess)) — the same
// pattern buildState already follows, see effectiveCfgFrom's own comment in serve.go.
//
// Two channels leaked through the raw config, and this test exercises both against one
// request:
//
//   - Roster.Lock. applyRoster/violatesRoster (squadchoice.go) bind the owner's own
//     lock to every optimiser request built from cfg, including this one. A lock naming
//     a player outside the visitor's actual fifteen made violatesRoster discard his real
//     squad and fall through to e.Optimize — a fabricated "optimal" squad, built with
//     the owner's lock forced into it, captioned as the visitor's own result.
//     effectiveCfgFrom fixes this by running the owner's config through forPlanner
//     first, which strips Roster.Lock entirely (see forPlanner's own Roster handling in
//     session.go, and chipleak_test.go's TestTheSiteServesNoChipPlanOfItsOwn for the
//     sibling fix to the chip-plan leak below).
//
//   - Chips. buildSquadPage assigns cfg.Chips onto the SHARED engine's e.Chips before
//     calling ApplyChipPlan, which can shorten e.Weights.Horizon when a wildcard is
//     planned. The owner's own wildcard has nothing to do with a visitor's result, and
//     State.Horizon — the figure every projection on the page is "over what?" for —
//     must reflect the visitor's own (empty) chip plan, not the owner's.
func TestResultsUsesTheVisitorsOwnEffectiveConfig(t *testing.T) {
	s := fixtureServer(t)
	event := s.engine.Boot.Events[0] // GW1, deadline 2026-08-21 17:30 UTC
	s.clock = func() time.Time { return event.DeadlineTime.Add(time.Hour) }

	gk, def, mid, fwd := legalFifteenElements(t, s.engine.Boot)
	picks := fakePicks(gk, def, mid, fwd)

	inSquad := map[int]bool{}
	for _, group := range [][]fpl.Element{gk, def, mid, fwd} {
		for _, el := range group {
			inSquad[el.Code] = true
		}
	}
	var outsider fpl.Element
	found := false
	for _, el := range s.engine.Boot.Elements {
		if !inSquad[el.Code] {
			outsider = el
			found = true
			break
		}
	}
	if !found {
		t.Fatal("the fixture bootstrap has no element outside the visitor's own fifteen")
	}

	// Clear the fixture's own standing exclude (see fixtureServerNamed's own comment):
	// it names boot.Elements[1], which legalFifteenElements' front-of-bootstrap picking
	// can itself pick as part of the visitor's "own" fifteen, and that coincidence would
	// trigger violatesRoster on its own — indistinguishable from the leak this test
	// isolates. The lock below is this test's only standing correction.
	s.cfg.Roster.Exclude = nil

	// The OWNER's config: a lock naming a player who is not in the visitor's actual
	// squad, and a wildcard planned two gameweeks out. Both live on *s.cfg, the config
	// apiResults used to build the page from directly.
	s.cfg.Roster.Lock = append(s.cfg.Roster.Lock, config.RosterOverride{
		Code: outsider.Code, Name: outsider.WebName,
		Reason: "the site owner's own lock — unrelated to any visitor",
		SetOn:  fixtureNow.Format("2006-01-02"),
	})
	nextGW := 1
	if next := s.engine.Boot.NextEvent(); next != nil {
		nextGW = next.ID
	}
	wildcardGW := nextGW + 2
	s.cfg.Chips = analysis.ChipSchedule{First: analysis.ChipPlan{Wildcard: wildcardGW}}

	fullHorizon := s.engine.Weights.Horizon
	shortened, why := s.engine.EffectiveHorizon(s.cfg.Chips)
	if why == "" || shortened >= fullHorizon {
		t.Skipf("the owner's wildcard at GW%d does not shorten the shared engine's "+
			"horizon %d — nothing here would distinguish the fix from the bug",
			wildcardGW, fullHorizon)
	}

	s.fetchEntry = func(ctx context.Context, id int) (*fpl.Entry, error) {
		return &fpl.Entry{ID: id, Name: "Visitor Team", SummaryOverallPoints: 42}, nil
	}
	s.fetchPicks = func(ctx context.Context, entryID, gw int) (*fpl.EntryPicks, error) {
		return picks, nil
	}
	cookie := resultsSessionCookie(t, 1234567)

	w := getResults(t, s, "?gw=1", cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/results?gw=1 answered %d, want 200: %s", w.Code, w.Body.String())
	}

	var st viewmodel.State
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatalf("the response did not decode: %v", err)
	}
	if st.Results == nil {
		t.Fatal("State.Results is nil, want a value — a fake entry and picks were given")
	}

	// The visitor's real squad, not a substituted optimum: the fifteen returned must be
	// exactly the fifteen fakePicks handed the server, and the owner's locked outsider
	// must not be among them.
	wantIDs := map[int]bool{}
	for _, p := range picks.Picks {
		wantIDs[p.Element] = true
	}
	gotIDs := map[int]bool{}
	for _, p := range st.Results.XI {
		gotIDs[p.ID] = true
	}
	for _, p := range st.Results.Bench {
		gotIDs[p.ID] = true
	}
	if len(gotIDs) != 15 {
		t.Fatalf("the result carries %d distinct players, want 15", len(gotIDs))
	}
	for id := range wantIDs {
		if !gotIDs[id] {
			t.Errorf("the visitor's own player (element %d) is missing from the result "+
				"— the owner's lock discarded his real squad", id)
		}
	}
	if gotIDs[outsider.ID] {
		t.Errorf("the owner's locked player (element %d, %s) leaked into the visitor's "+
			"own result", outsider.ID, outsider.WebName)
	}

	// The visitor's own week-views, not the owner's: with no chip plan of his own, his
	// horizon must be the full configured one, not the span the owner's wildcard
	// truncates it to.
	if st.Horizon != fullHorizon {
		t.Errorf("State.Horizon = %d, want the visitor's own full horizon %d — the "+
			"owner's wildcard at GW%d truncated it to %d (%q)",
			st.Horizon, fullHorizon, wildcardGW, shortened, why)
	}
}
