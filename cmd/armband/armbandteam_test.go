package main

import (
	"testing"
	"time"

	"armband/internal/fpl"
)

// fakeEntryPicks builds a picks response for fifteen of the fixture's own elements — not
// necessarily a legal fifteen (picksToFixed does not check budget or club limits, only
// translation), with the given elements at positions 1..15, position 1 captained and
// position 2 vice-captained, matching a real FPL response's shape.
func fakeEntryPicks(elements []fpl.Element) *fpl.EntryPicks {
	if len(elements) != 15 {
		panic("fakeEntryPicks needs exactly 15 elements")
	}
	picks := &fpl.EntryPicks{}
	for i, el := range elements {
		picks.Picks = append(picks.Picks, fpl.Pick{
			Element:       el.ID,
			Position:      i + 1,
			Multiplier:    1,
			IsCaptain:     i == 0,
			IsViceCaptain: i == 1,
		})
	}
	return picks
}

// TestPicksToFixedTranslatesARealSquad is the regression pin for the bug this fixes: the
// house team page must show config.EntryID's REAL fifteen, not the model's own optimum,
// and this is the function that turns FPL's picks response into what buildSquadPage's
// Fixed/Arrange actually consume. See armbandTeamState's own comment for the live
// incident (Enzo Fernández shown in a slot the real account had Szoboszlai) this pins
// against recurring.
func TestPicksToFixedTranslatesARealSquad(t *testing.T) {
	s := fixtureServer(t)
	elements := s.engine.Boot.Elements[:15]
	picks := fakeEntryPicks(elements)

	fixed, arr := picksToFixed(s.engine.Boot, picks)

	if len(fixed) != 15 {
		t.Fatalf("got %d codes, want 15", len(fixed))
	}
	for i, el := range elements {
		if fixed[i] != el.Code {
			t.Errorf("fixed[%d] = %d, want %s's code %d", i, fixed[i], el.WebName, el.Code)
		}
	}

	wantXI := make([]int, 11)
	for i := 0; i < 11; i++ {
		wantXI[i] = elements[i].Code
	}
	if len(arr.XI) != 11 {
		t.Fatalf("XI has %d players, want 11", len(arr.XI))
	}
	for i := range wantXI {
		if arr.XI[i] != wantXI[i] {
			t.Errorf("XI[%d] = %d, want %d", i, arr.XI[i], wantXI[i])
		}
	}

	wantBench := []int{elements[11].Code, elements[12].Code, elements[13].Code, elements[14].Code}
	if len(arr.Bench) != 4 {
		t.Fatalf("bench has %d players, want 4", len(arr.Bench))
	}
	for i := range wantBench {
		if arr.Bench[i] != wantBench[i] {
			t.Errorf("bench[%d] = %d, want %d", i, arr.Bench[i], wantBench[i])
		}
	}

	if arr.Captain != elements[0].Code {
		t.Errorf("captain = %d, want %s's code %d", arr.Captain, elements[0].WebName, elements[0].Code)
	}
	if arr.Vice != elements[1].Code {
		t.Errorf("vice = %d, want %s's code %d", arr.Vice, elements[1].WebName, elements[1].Code)
	}
}

// TestPicksToFixedSortsByPositionRegardlessOfResponseOrder — FPL's own picks array is
// already position-ordered, but this must not silently rely on that: an out-of-order
// response must still land the right player in the XI versus the bench.
func TestPicksToFixedSortsByPositionRegardlessOfResponseOrder(t *testing.T) {
	s := fixtureServer(t)
	elements := s.engine.Boot.Elements[:15]
	picks := fakeEntryPicks(elements)
	// Reverse the response order; the Position field alone still says who starts.
	for i, j := 0, len(picks.Picks)-1; i < j; i, j = i+1, j-1 {
		picks.Picks[i], picks.Picks[j] = picks.Picks[j], picks.Picks[i]
	}

	_, arr := picksToFixed(s.engine.Boot, picks)
	if len(arr.XI) != 11 || arr.XI[0] != elements[0].Code {
		t.Errorf("XI[0] = %v, want position 1's player %s (%d) regardless of response order",
			arr.XI, elements[0].WebName, elements[0].Code)
	}
}

// TestPicksToFixedRefusesAnUnresolvedElement — a pick that cannot be mapped to this
// season's element list must fall through to (nil, arrangement{}), the same "let
// buildSquadPage fall back to its own optimum" answer a stale reader-imported team gets,
// rather than silently drawing fourteen players and calling it a team.
func TestPicksToFixedRefusesAnUnresolvedElement(t *testing.T) {
	s := fixtureServer(t)
	elements := append([]fpl.Element(nil), s.engine.Boot.Elements[:15]...)
	picks := fakeEntryPicks(elements)
	picks.Picks[14].Element = -1 // no such element

	fixed, arr := picksToFixed(s.engine.Boot, picks)
	if fixed != nil || arr.Captain != 0 || len(arr.XI) != 0 {
		t.Errorf("got fixed=%v arr=%+v, want the honest empty fallback", fixed, arr)
	}
}

// TestPicksToFixedRefusesAShortSquad — FPL always answers with fifteen; this pins the
// fallback for a response that does not, the same guard importTeam applies to an import.
func TestPicksToFixedRefusesAShortSquad(t *testing.T) {
	s := fixtureServer(t)
	picks := &fpl.EntryPicks{Picks: []fpl.Pick{
		{Element: s.engine.Boot.Elements[0].ID, Position: 1},
	}}

	fixed, arr := picksToFixed(s.engine.Boot, picks)
	if fixed != nil || len(arr.XI) != 0 {
		t.Errorf("got fixed=%v arr=%+v, want the honest empty fallback for a 1-player response", fixed, arr)
	}
}

// TestHouseRealPicksIsInertWithNoEntryOrEvent — the house team page must not spend a
// fetch when there is no FPL client, when the operator has not configured
// config.EntryID, or before any gameweek has closed. Matches houseTeamSources' own
// honest-absence contract. A real (but here unused, since every case below returns
// before it would be touched) *fpl.Client is used for the entry/event cases so the guard
// clauses under test are the entryID/event checks, not "is client nil".
func TestHouseRealPicksIsInertWithNoEntryOrEvent(t *testing.T) {
	s := fixtureServer(t)
	event := s.engine.Boot.Events[0]

	if fixed, arr := houseRealPicks(t.Context(), nil, s.engine.Boot, 2785902, &event); fixed != nil || len(arr.XI) != 0 {
		t.Errorf("got fixed=%v arr=%+v with a nil client, want the honest empty fallback", fixed, arr)
	}

	client := fpl.New(t.TempDir(), 24*time.Hour, 24*time.Hour)
	if fixed, arr := houseRealPicks(t.Context(), client, s.engine.Boot, 0, &event); fixed != nil || len(arr.XI) != 0 {
		t.Errorf("got fixed=%v arr=%+v with no entry configured, want the honest empty fallback", fixed, arr)
	}
	if fixed, arr := houseRealPicks(t.Context(), client, s.engine.Boot, 2785902, nil); fixed != nil || len(arr.XI) != 0 {
		t.Errorf("got fixed=%v arr=%+v with no closed event yet, want the honest empty fallback", fixed, arr)
	}
}

// TestFixtureMatchStatusThreeWay pins the 2026-08-22 defect: a match that has ended is
// not "in progress". FPL's own /api/fixtures/?event=1, read live 2026-08-22, showed five
// of six fixtures Started=true, FinishedProvisional=true, Finished=false — played out,
// score locked in, bonus not yet applied — and the pre-fix two-way switch (Finished vs
// Started) reported every one of them "live", putting a live dot and a "provisional
// bonus" asterisk on matches that had already finished. The "live, genuinely in
// progress" case below is the one real fixture from that read (BRE v TOT) that stayed
// genuinely in progress.
func TestFixtureMatchStatusThreeWay(t *testing.T) {
	cases := []struct {
		name string
		f    fpl.Fixture
		want string
	}{
		{"fulltime, unsettled", fpl.Fixture{Started: true, FinishedProvisional: true, Finished: false}, "fulltime"},
		{"finished, settled", fpl.Fixture{Started: true, FinishedProvisional: true, Finished: true}, "finished"},
		{"live, genuinely in progress", fpl.Fixture{Started: true, FinishedProvisional: false, Finished: false}, "live"},
		{"not yet kicked off", fpl.Fixture{Started: false, FinishedProvisional: false, Finished: false}, "scheduled"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := fixtureMatchStatus(c.f); got != c.want {
				t.Errorf("fixtureMatchStatus(%+v) = %q, want %q", c.f, got, c.want)
			}
		})
	}
}
