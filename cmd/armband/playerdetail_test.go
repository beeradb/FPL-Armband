package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"armband/internal/fpl"
	"armband/internal/viewmodel"
)

// fixtureSummary is a small, fixed ElementSummary: one past season and two played fixtures
// this season, on a team id the committed capture also carries.
func fixtureSummary(teamID int) *fpl.ElementSummary {
	return &fpl.ElementSummary{
		HistoryPast: []fpl.PastSeason{
			// Oldest first, as FPL actually returns it -- BuildPlayerDetail must read the
			// LAST entry, not the first.
			{SeasonName: "2023/24", TotalPoints: 90, Minutes: 1800, Starts: 20,
				GoalsScored: 3, Assists: 2, CleanSheets: 4, Bonus: 5,
				ExpectedGoals: 2.5, ExpectedAssists: 1.5,
				StartCost: 60, EndCost: 65},
			{SeasonName: "2024/25", TotalPoints: 186, Minutes: 2843, Starts: 32,
				GoalsScored: 12, Assists: 14, CleanSheets: 9, Bonus: 22,
				ExpectedGoals: 9.8, ExpectedAssists: 11.2,
				StartCost: 90, EndCost: 102},
		},
		History: []fpl.HistoryEntry{
			{Round: 1, OpponentTeam: teamID, WasHome: true, Minutes: 62, Starts: 1,
				TotalPoints: 2, GoalsScored: 0, Assists: 0, CleanSheets: 0, Bonus: 0, BPS: 6,
				ExpectedGoals: 0.09, ExpectedAssists: 0.11, Value: 100},
			{Round: 2, OpponentTeam: teamID, WasHome: false, Minutes: 84, Starts: 1,
				TotalPoints: 8, GoalsScored: 0, Assists: 1, CleanSheets: 1, Bonus: 1, BPS: 18,
				ExpectedGoals: 0.18, ExpectedAssists: 0.55, Value: 101},
		},
	}
}

// withFetch wires a fixtureServer to answer /api/player from fixtureSummary rather than a
// live client, and records every element id it was asked for.
func withFetch(t *testing.T) (*squadServer, *[]int) {
	t.Helper()
	s := fixtureServer(t)
	var asked []int
	teamID := s.engine.Boot.Elements[0].Team
	s.fetchSummary = func(_ context.Context, id int) (*fpl.ElementSummary, error) {
		asked = append(asked, id)
		return fixtureSummary(teamID), nil
	}
	return s, &asked
}

func playerPath(code int) string { return prefixPlayer + strconv.Itoa(code) }

// TestPlayerDetailIsKeyedOnCodeNotID is the trap this route exists to avoid: the client
// addresses a player by permanent CODE, and the route must translate that to the
// season-scoped element id itself rather than being handed one, or a code sent after
// element ids have been reassigned would resolve to the wrong footballer -- or to none at
// all.
func TestPlayerDetailIsKeyedOnCodeNotID(t *testing.T) {
	s, asked := withFetch(t)
	el := s.engine.Boot.Elements[0]

	w := get(t, s, playerPath(el.Code))
	if w.Code != 200 {
		t.Fatalf("GET %s answered %d: %s", playerPath(el.Code), w.Code, w.Body.String())
	}
	if len(*asked) != 1 || (*asked)[0] != el.ID {
		t.Fatalf("elementSummary was asked for %v, want exactly [%d] (the ELEMENT ID for "+
			"code %d) -- the route must translate code to id, not pass the code through",
			*asked, el.ID, el.Code)
	}

	// This only asserts something when id and code actually differ, which the committed
	// capture's elements generally do -- a code sent as though it were the id must not
	// resolve to a player.
	if el.ID != el.Code {
		*asked = nil
		w2 := get(t, s, playerPath(el.ID))
		if w2.Code == 200 {
			t.Errorf("GET %s (the element ID, not the code) answered 200 -- codes and ids "+
				"must not be interchangeable on this route", playerPath(el.ID))
		}
	}
}

// TestPlayerDetailTranslatesHistory checks the shape of the document end to end: last
// season is the LAST entry in history_past (oldest-first order respected), points_per_90 is
// computed rather than left to the client, and the gameweek log resolves the opponent's team
// id to a short name using the bootstrap rather than leaking a bare id.
func TestPlayerDetailTranslatesHistory(t *testing.T) {
	s, _ := withFetch(t)
	el := s.engine.Boot.Elements[0]

	w := get(t, s, playerPath(el.Code))
	if w.Code != 200 {
		t.Fatalf("GET answered %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct == "" || w.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Content-Type %q, Cache-Control %q -- want a JSON document explicitly "+
			"marked not to cache, the same as /api/state", ct, w.Header().Get("Cache-Control"))
	}

	var d viewmodel.PlayerDetail
	if err := json.Unmarshal(w.Body.Bytes(), &d); err != nil {
		t.Fatalf("decoding the document: %v\n%s", err, w.Body.String())
	}

	if d.LastSeason == nil {
		t.Fatal("LastSeason is nil, want the most recent of two seasons")
	}
	if d.LastSeason.Season != "2024/25" {
		t.Errorf("LastSeason.Season = %q, want the LAST entry (2024/25) -- history_past is "+
			"oldest-first and the first entry (2023/24) must not be read as last season",
			d.LastSeason.Season)
	}
	if d.LastSeason.Points != 186 || d.LastSeason.Minutes != 2843 {
		t.Errorf("LastSeason = %+v, want the 2024/25 row's own figures", d.LastSeason)
	}
	if d.LastSeason.PriceStart != 9.0 || d.LastSeason.PriceEnd != 10.2 {
		t.Errorf("LastSeason price = %.1f -> %.1f, want 9.0 -> 10.2 -- FPL's tenths (90, 102) "+
			"converted to millions via fpl.TenthsToMillions", d.LastSeason.PriceStart, d.LastSeason.PriceEnd)
	}
	wantPer90 := 186.0 / (2843.0 / 90.0)
	if diff := d.LastSeason.PointsPer90 - wantPer90; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("PointsPer90 = %v, want %v computed server-side from points and minutes",
			d.LastSeason.PointsPer90, wantPer90)
	}

	if len(d.Gameweeks) != 2 {
		t.Fatalf("Gameweeks has %d entries, want 2", len(d.Gameweeks))
	}
	var wantShort string
	for _, tm := range s.engine.Boot.Teams {
		if tm.ID == el.Team {
			wantShort = tm.ShortName
		}
	}
	if d.Gameweeks[0].Opponent != wantShort {
		t.Errorf("Gameweeks[0].Opponent = %q, want the resolved short name %q -- the client "+
			"must never be handed a bare team id", d.Gameweeks[0].Opponent, wantShort)
	}
	if d.Gameweeks[1].Points != 8 || !d.Gameweeks[1].Started {
		t.Errorf("Gameweeks[1] = %+v, did not round-trip the fixture", d.Gameweeks[1])
	}
	if d.Gameweeks[1].Price != 10.1 {
		t.Errorf("Gameweeks[1].Price = %.1f, want 10.1 -- FPL's tenths (101) converted to "+
			"millions", d.Gameweeks[1].Price)
	}
}

// TestPlayerDetailOmitsCleanSheetsOffThePitch checks the position-gated field: a clean sheet
// is not a midfielder's or forward's stat, so LastSeason.CleanSheets must be entirely absent
// from the document for one rather than printed as a bare, misleading zero -- and present,
// even at zero, for a defender or goalkeeper.
func TestPlayerDetailOmitsCleanSheetsOffThePitch(t *testing.T) {
	s, _ := withFetch(t)

	var def, mid *fpl.Element
	for i := range s.engine.Boot.Elements {
		el := &s.engine.Boot.Elements[i]
		switch s.engine.Boot.PositionShort(el.ElementType) {
		case "DEF", "GKP":
			if def == nil {
				def = el
			}
		case "MID", "FWD":
			if mid == nil {
				mid = el
			}
		}
	}
	if def == nil || mid == nil {
		t.Fatal("the committed capture has no defender/goalkeeper and outfield attacker to compare")
	}

	for _, tc := range []struct {
		name    string
		el      *fpl.Element
		wantNil bool
	}{
		{"defender or keeper", def, false},
		{"midfielder or forward", mid, true},
	} {
		w := get(t, s, playerPath(tc.el.Code))
		if w.Code != 200 {
			t.Fatalf("%s: GET answered %d", tc.name, w.Code)
		}
		var d viewmodel.PlayerDetail
		if err := json.Unmarshal(w.Body.Bytes(), &d); err != nil {
			t.Fatalf("%s: decoding: %v", tc.name, err)
		}
		if d.LastSeason == nil {
			t.Fatalf("%s: LastSeason is nil", tc.name)
		}
		gotNil := d.LastSeason.CleanSheets == nil
		if gotNil != tc.wantNil {
			t.Errorf("%s: CleanSheets nil=%v, want nil=%v", tc.name, gotNil, tc.wantNil)
		}
		// The raw JSON is what the client actually sees, so the "must not even print a
		// zero" half of the claim is checked on the wire form directly.
		if tc.wantNil && bytes.Contains(w.Body.Bytes(), []byte(`"clean_sheets"`)) {
			t.Errorf("%s: the wire document names clean_sheets at all; it must be omitted, "+
				"not sent as 0 or null", tc.name)
		}
	}
}

// TestPlayerDetailRefusesAnUnknownCode checks the 404 path: a code the bootstrap does not
// carry must not silently resolve to some other player's history.
func TestPlayerDetailRefusesAnUnknownCode(t *testing.T) {
	s, asked := withFetch(t)
	w := get(t, s, playerPath(999999999))
	if w.Code != 404 {
		t.Errorf("GET an unknown code answered %d, want 404", w.Code)
	}
	if len(*asked) != 0 {
		t.Errorf("elementSummary was called for an unknown code: %v", *asked)
	}
}

// TestPlayerDetailRefusesGarbage checks the route parses its own path segment rather than
// handing a non-numeric suffix to Atoi and trusting the zero value it produces on error.
// TestPlayerDetailRejectsOtherMethods checks the route states its method explicitly, like
// every other read route on this server, rather than silently accepting whatever verb a
// caller sends.
func TestPlayerDetailRejectsOtherMethods(t *testing.T) {
	s := fixtureServer(t)
	el := s.engine.Boot.Elements[0]
	req := httptest.NewRequest("POST", playerPath(el.Code), nil)
	req.Host = "127.0.0.1:8080"
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST %s answered %d, want 405", playerPath(el.Code), w.Code)
	}
	if got := w.Header().Get("Allow"); got != "GET" {
		t.Errorf("Allow header %q, want GET", got)
	}
}

func TestPlayerDetailRefusesGarbage(t *testing.T) {
	s := fixtureServer(t)
	for _, path := range []string{prefixPlayer, prefixPlayer + "abc", prefixPlayer + "-1", prefixPlayer + "0"} {
		if w := get(t, s, path); w.Code != 400 && w.Code != 404 {
			t.Errorf("GET %s answered %d, want 400 or 404", path, w.Code)
		}
	}
}

// TestPlayerDetailReportsAnUpstreamFailure checks that a failed fetch answers an error
// rather than a 200 with an empty or half-built document the client would render as an
// absence.
func TestPlayerDetailReportsAnUpstreamFailure(t *testing.T) {
	s := fixtureServer(t)
	s.fetchSummary = func(context.Context, int) (*fpl.ElementSummary, error) {
		return nil, fmt.Errorf("upstream unavailable")
	}
	el := s.engine.Boot.Elements[0]
	w := get(t, s, playerPath(el.Code))
	if w.Code < 500 {
		t.Errorf("GET answered %d on a failed fetch, want a server error", w.Code)
	}
}
