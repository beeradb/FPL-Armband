package main

import (
	"fmt"
	"testing"

	"armband/internal/analysis"
	"armband/internal/fpl"
)

// TestSquadBlankOrDoubleWeeksReportsTheSquadsOwnClubs — `armband chips`
// prints league-wide blank/double counts, but before this test nothing told a
// reader whether the squad they actually hold is exposed by one. This pins
// squadBlankOrDoubleWeeks, the function that turns those league-wide counts
// into a per-club answer for one squad, using the SAME fixture count the
// scoring path prices a bench-boost week with (Engine.FixtureCountsIn) rather
// than a second, hand-rolled fixture window.
func TestSquadBlankOrDoubleWeeksReportsTheSquadsOwnClubs(t *testing.T) {
	boot := &fpl.Bootstrap{
		ElementTypes: []fpl.ElementType{
			{ID: 1, SingularNameShort: "GKP"}, {ID: 2, SingularNameShort: "DEF"},
			{ID: 3, SingularNameShort: "MID"}, {ID: 4, SingularNameShort: "FWD"},
		},
	}
	names := map[int]string{1: "A", 2: "B", 3: "C", 4: "D", 5: "E", 6: "F"}
	for id := 1; id <= 6; id++ {
		boot.Teams = append(boot.Teams, fpl.Team{ID: id, Name: names[id], ShortName: names[id]})
	}
	for i := 1; i <= 5; i++ {
		boot.Events = append(boot.Events, fpl.Event{ID: i})
	}

	ev := func(n int) *int { return &n }
	// GW2 is the interesting week: club A (and B) have no fixture at all, so
	// they blank; club C (and D) play each other twice, so they double; club
	// E (and F) play their one ordinary match, so they are unaffected.
	fixtures := []fpl.Fixture{
		{ID: 1, Event: ev(1), TeamH: 1, TeamA: 2},
		{ID: 2, Event: ev(1), TeamH: 3, TeamA: 4},
		{ID: 3, Event: ev(2), TeamH: 3, TeamA: 4},
		{ID: 4, Event: ev(2), TeamH: 4, TeamA: 3},
		{ID: 5, Event: ev(2), TeamH: 5, TeamA: 6},
		{ID: 6, Event: ev(3), TeamH: 1, TeamA: 2},
		{ID: 7, Event: ev(3), TeamH: 3, TeamA: 4},
		{ID: 8, Event: ev(1), TeamH: 5, TeamA: 6},
		{ID: 9, Event: ev(3), TeamH: 5, TeamA: 6},
	}

	e := analysis.NewEngineFull(boot, fixtures, analysis.DefaultWeights(),
		analysis.Congestion{}, analysis.RoleRisk{})

	mk := func(id, club, pos int) fpl.Element {
		return fpl.Element{
			ID: id, Team: club, ElementType: pos, WebName: fmt.Sprintf("P%d", id),
			Status: "a", Minutes: 900, Starts: 10,
		}
	}
	boot.Elements = []fpl.Element{
		mk(10, 1, 2), // holds club A, which blanks GW2
		mk(11, 3, 3), // holds club C, which doubles GW2
		mk(12, 5, 4), // holds club E, which plays normally every week
	}

	var squad []analysis.PlayerMetrics
	for i := range boot.Elements {
		squad = append(squad, e.Metrics(&boot.Elements[i]))
	}

	// The league-wide counts cmdChips itself derives from e.Fixtures.
	counts := map[int]int{}
	for _, f := range fixtures {
		if f.Event != nil {
			counts[*f.Event]++
		}
	}
	gws := []int{1, 2, 3}

	blanks, doubles := squadBlankOrDoubleWeeks(e, squad, gws, counts)

	if got := blanks[2]; len(got) != 1 || got[0] != "P10" {
		t.Errorf("GW2 blanks = %v, want exactly [P10] — club A has no fixture that week", got)
	}
	if got := doubles[2]; len(got) != 1 || got[0] != "P11" {
		t.Errorf("GW2 doubles = %v, want exactly [P11] — club C plays twice that week", got)
	}
	if len(blanks[1]) != 0 || len(doubles[1]) != 0 {
		t.Errorf("GW1 is a normal week for every squad member, got blanks=%v doubles=%v",
			blanks[1], doubles[1])
	}
	if len(blanks[3]) != 0 || len(doubles[3]) != 0 {
		t.Errorf("GW3 is a normal week for every squad member, got blanks=%v doubles=%v",
			blanks[3], doubles[3])
	}
}
