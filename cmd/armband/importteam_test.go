package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"armband/internal/fpl"
	"armband/internal/viewmodel"
)

// openImportWindow rewrites the fixture's events so importWindow reports open, with the
// given current/next gameweek pair. fixtureServer's committed capture is pre-season (GW1
// not yet played), which is exactly the CLOSED state — useful for the closed-window tests
// below, and the reason every other test that needs an OPEN window calls this first.
func openImportWindow(s *squadServer, cur, next int) {
	events := append([]fpl.Event(nil), s.engine.Boot.Events...)
	for i := range events {
		events[i].IsCurrent = events[i].ID == cur
		events[i].IsNext = events[i].ID == next
	}
	s.engine.Boot.Events = events
}

// notFoundErr is what EntryUncached/PicksUncached return on a 404, for tests that inject
// s.fetchEntry/s.fetchPicks directly rather than standing up a fake HTTP server.
func notFoundErr() error {
	return fmt.Errorf("GET /entry/x/: status 404: %w", fpl.ErrNotFound)
}

// legalFifteenElements picks a real, legal fifteen out of the fixture's own bootstrap —
// two goalkeepers, five defenders, five midfielders, three forwards, no more than three
// from one club — so the fake Picks response importTeam is handed decodes into a squad
// validateSession actually accepts.
func legalFifteenElements(t *testing.T, boot *fpl.Bootstrap) (gk, def, mid, fwd []fpl.Element) {
	t.Helper()
	clubCount := map[int]int{}
	pick := func(elementType, need int) []fpl.Element {
		var out []fpl.Element
		for _, el := range boot.Elements {
			if el.ElementType != elementType {
				continue
			}
			if clubCount[el.Team] >= 3 {
				continue
			}
			out = append(out, el)
			clubCount[el.Team]++
			if len(out) == need {
				break
			}
		}
		if len(out) != need {
			t.Fatalf("could not find %d players of element_type %d respecting the "+
				"3-per-club limit in the fixture capture", need, elementType)
		}
		return out
	}
	gk = pick(1, 2)
	def = pick(2, 5)
	mid = pick(3, 5)
	fwd = pick(4, 3)
	return
}

// fakePicks builds an EntryPicks response for a legal fifteen: a 4-4-2 in the XI (one
// goalkeeper, four defenders, four midfielders, two forwards), the second goalkeeper and
// one of each outfield position on the bench, a captain and a vice-captain in the XI.
func fakePicks(gk, def, mid, fwd []fpl.Element) *fpl.EntryPicks {
	xi := []fpl.Element{gk[0], def[0], def[1], def[2], def[3],
		mid[0], mid[1], mid[2], mid[3], fwd[0], fwd[1]}
	bench := []fpl.Element{gk[1], def[4], mid[4], fwd[2]}

	var picks []fpl.Pick
	pos := 1
	for _, el := range xi {
		picks = append(picks, fpl.Pick{Element: el.ID, Position: pos, Multiplier: 1})
		pos++
	}
	// Two arbitrary-but-distinct XI slots wear the armband — position 2 and 3, i.e. the
	// first two outfield defenders, chosen only for being two different XI players.
	picks[1].IsCaptain = true
	picks[1].Multiplier = 2
	picks[2].IsViceCaptain = true
	for _, el := range bench {
		picks = append(picks, fpl.Pick{Element: el.ID, Position: pos})
		pos++
	}
	return &fpl.EntryPicks{Picks: picks}
}

func putImport(t *testing.T, s *squadServer, entry any, cookie *http.Cookie, crossOriginNoToken bool) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]any{"entry": entry})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("PUT", routeImport, bytes.NewReader(body))
	req.Host = "127.0.0.1:8080"
	req.Header.Set("Content-Type", "application/json")
	if crossOriginNoToken {
		req.Header.Set("Sec-Fetch-Site", "cross-site")
	} else {
		req.Header.Set("X-Armband-Token", s.token)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	return w
}

func TestImportRejectsWrongMethod(t *testing.T) {
	s := fixtureServer(t)
	req := httptest.NewRequest("GET", routeImport, nil)
	req.Host = "127.0.0.1:8080"
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /api/import answered %d, want 405", w.Code)
	}
}

func TestImportRejectsCrossOriginWithoutAToken(t *testing.T) {
	s := fixtureServer(t)
	openImportWindow(s, 1, 2)
	w := putImport(t, s, "1234567", nil, true)
	if w.Code != http.StatusForbidden {
		t.Errorf("cross-origin PUT /api/import with no token answered %d, want 403", w.Code)
	}
}

func TestImportRefusesWhenTheGateIsClosed(t *testing.T) {
	s := fixtureServer(t)
	openImportWindow(s, 1, 2)
	s.signups = &recordingStore{}
	w := putImport(t, s, "1234567", nil, false)
	if w.Code != http.StatusForbidden {
		t.Errorf("PUT /api/import with the preview gate closed answered %d, want 403", w.Code)
	}
}

// TestImportRefusesBeforeGW1HasBeenPlayed pins that a closed import window is checked
// BEFORE any parsing or network call: fetchEntry is set to fail the test if it is ever
// called at all, so this fails loudly if the handler ever reaches the network path with
// the window closed.
func TestImportRefusesBeforeGW1HasBeenPlayed(t *testing.T) {
	s := fixtureServer(t) // pre-season fixture: importWindow is closed by construction
	s.fetchEntry = func(ctx context.Context, id int) (*fpl.Entry, error) {
		t.Fatal("importTeam fetched an entry with the import window closed")
		return nil, nil
	}
	w := putImport(t, s, "1234567", nil, false)
	if w.Code != http.StatusConflict {
		t.Errorf("PUT /api/import with the window closed answered %d, want 409", w.Code)
	}
}

func TestImportRejectsAMalformedEntryValue(t *testing.T) {
	s := fixtureServer(t)
	openImportWindow(s, 1, 2)
	for _, bad := range []any{"not-a-number", "", "0", "-5", nil} {
		w := putImport(t, s, bad, nil, false)
		if w.Code != http.StatusBadRequest {
			t.Errorf("PUT /api/import with entry=%v answered %d, want 400", bad, w.Code)
		}
	}
}

func TestImportAnswers404WhenFPLHasNoSuchEntry(t *testing.T) {
	s := fixtureServer(t)
	openImportWindow(s, 1, 2)
	s.fetchEntry = func(ctx context.Context, id int) (*fpl.Entry, error) {
		return nil, notFoundErr()
	}
	w := putImport(t, s, "9999999", nil, false)
	if w.Code != http.StatusNotFound {
		t.Errorf("PUT /api/import for an unknown entry answered %d, want 404: %s",
			w.Code, w.Body.String())
	}
}

func TestImportAnswers409WhenTheEntryHasNoSquadForTheGameweek(t *testing.T) {
	s := fixtureServer(t)
	openImportWindow(s, 1, 2)
	s.fetchEntry = func(ctx context.Context, id int) (*fpl.Entry, error) {
		return &fpl.Entry{ID: id}, nil
	}
	s.fetchPicks = func(ctx context.Context, entryID, event int) (*fpl.EntryPicks, error) {
		return nil, notFoundErr()
	}
	w := putImport(t, s, "1234567", nil, false)
	if w.Code != http.StatusConflict {
		t.Errorf("PUT /api/import with no picks for the gameweek answered %d, want 409: %s",
			w.Code, w.Body.String())
	}
}

func TestImportAnswers503WhenFPLIsUnreachable(t *testing.T) {
	s := fixtureServer(t)
	openImportWindow(s, 1, 2)
	s.fetchEntry = func(ctx context.Context, id int) (*fpl.Entry, error) {
		return nil, errors.New("dial tcp: connection refused")
	}
	w := putImport(t, s, "1234567", nil, false)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("PUT /api/import with FPL unreachable answered %d, want 503", w.Code)
	}
}

// TestImportAtGoodStandingProducesASessionKeyedOnCodes is the end-to-end success path: a
// legal fake squad comes back from FPL, keyed on season-scoped element ids, and the stored
// session must hold PERMANENT CODES — never element ids — with the XI, bench, captain and
// vice matching the fake picks.
func TestImportAtGoodStandingProducesASessionKeyedOnCodes(t *testing.T) {
	s := fixtureServer(t)
	openImportWindow(s, 1, 2)

	gk, def, mid, fwd := legalFifteenElements(t, s.engine.Boot)
	picks := fakePicks(gk, def, mid, fwd)
	s.fetchEntry = func(ctx context.Context, id int) (*fpl.Entry, error) {
		return &fpl.Entry{ID: id, Name: "Test Team"}, nil
	}
	s.fetchPicks = func(ctx context.Context, entryID, event int) (*fpl.EntryPicks, error) {
		if event != 1 {
			t.Errorf("fetchPicks asked for gameweek %d, want 1 (the just-played one)", event)
		}
		return picks, nil
	}

	w := putImport(t, s, "1234567", nil, false)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT /api/import answered %d: %s", w.Code, w.Body.String())
	}

	var st viewmodel.State
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatalf("the response did not decode: %v", err)
	}
	if len(st.Squad.Players) != 15 {
		t.Errorf("the imported squad has %d players, want 15", len(st.Squad.Players))
	}
	if len(st.Squad.XI) != 11 || len(st.Squad.Bench) != 4 {
		t.Errorf("XI/bench = %d/%d, want 11/4", len(st.Squad.XI), len(st.Squad.Bench))
	}
	if st.Squad.Captain == 0 || st.Squad.Vice == 0 {
		t.Error("no captain or vice-captain came through the import")
	}
	if !st.Import.Open {
		t.Error("State.Import.Open is false with the window forced open")
	}
	if st.Import.Entry != 1234567 {
		t.Errorf("State.Import.Entry = %d, want 1234567", st.Import.Entry)
	}

	// The session cookie itself must carry CODES, not element ids — element ids are
	// reassigned every summer, and this codebase keys everything durable on the code.
	cookie := sessionCookie(t, w)
	readReq := httptest.NewRequest("GET", "/", nil)
	readReq.AddCookie(cookie)
	got := readSession(readReq)
	if len(got.Squad) != 15 {
		t.Fatalf("stored session has %d squad entries, want 15", len(got.Squad))
	}
	wantCodes := map[int]bool{}
	for _, group := range [][]fpl.Element{gk, def, mid, fwd} {
		for _, el := range group {
			wantCodes[el.Code] = true
		}
	}
	for _, code := range got.Squad {
		if !wantCodes[code] {
			t.Errorf("stored session carries code %d, which is not a code in the fake squad "+
				"— it looks like an ELEMENT ID leaked into the code-keyed store", code)
		}
	}
	if got.Entry != 1234567 {
		t.Errorf("stored session.Entry = %d, want 1234567", got.Entry)
	}
}

// TestImportRefusesAPartialFifteen pins that an unresolvable pick refuses the WHOLE
// import rather than storing fourteen players silently.
func TestImportRefusesAPartialFifteen(t *testing.T) {
	s := fixtureServer(t)
	openImportWindow(s, 1, 2)
	gk, def, mid, fwd := legalFifteenElements(t, s.engine.Boot)
	picks := fakePicks(gk, def, mid, fwd)
	// Corrupt one pick's element id so it resolves to no player at all.
	picks.Picks[0].Element = -999999

	s.fetchEntry = func(ctx context.Context, id int) (*fpl.Entry, error) {
		return &fpl.Entry{ID: id}, nil
	}
	s.fetchPicks = func(ctx context.Context, entryID, event int) (*fpl.EntryPicks, error) {
		return picks, nil
	}
	w := putImport(t, s, "1234567", nil, false)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("PUT /api/import with an unresolvable pick answered %d, want 500: %s",
			w.Code, w.Body.String())
	}
}

// TestImportNeverTouchesConfig pins the v1 boundary stated in importTeam's own doc
// comment: this route must never call config.Save or otherwise persist to the config
// file, regardless of -persist or cfgPath. The budget stays the hypothetical default.
func TestImportNeverTouchesConfig(t *testing.T) {
	s := fixtureServer(t)
	openImportWindow(s, 1, 2)

	cfgPath := filepath.Join(t.TempDir(), "config.json")
	marker := []byte(`{"marker":"untouched"}`)
	if err := os.WriteFile(cfgPath, marker, 0o644); err != nil {
		t.Fatal(err)
	}
	s.cfgPath = cfgPath
	s.persist = true

	gk, def, mid, fwd := legalFifteenElements(t, s.engine.Boot)
	picks := fakePicks(gk, def, mid, fwd)
	s.fetchEntry = func(ctx context.Context, id int) (*fpl.Entry, error) {
		return &fpl.Entry{ID: id}, nil
	}
	s.fetchPicks = func(ctx context.Context, entryID, event int) (*fpl.EntryPicks, error) {
		return picks, nil
	}

	w := putImport(t, s, "1234567", nil, false)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT /api/import answered %d: %s", w.Code, w.Body.String())
	}

	got, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, marker) {
		t.Errorf("config file changed from %q to %q — the import route must never touch "+
			"config.json", marker, got)
	}
}
