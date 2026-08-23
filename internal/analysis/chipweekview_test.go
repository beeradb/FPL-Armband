package analysis

import (
	"testing"

	"armband/internal/fpl"
)

// chipViewTestEngine builds an eight-club pool -- ten players per club, three
// per position group -- big enough for Optimize to fill a legal fifteen under
// the three-per-club cap. It mirrors the harness
// TestAWildcardIsNotBuiltOnOneWeeksBlanks already uses for the same reason.
func chipViewTestEngine(t *testing.T) *Engine {
	t.Helper()
	e := loadEngine(t, nil, nil)
	e.Weights.Horizon = 5
	id := 0
	for club := 1; club <= 8; club++ {
		for _, pos := range []int{1, 1, 2, 2, 2, 3, 3, 3, 4, 4} {
			id++
			e.Boot.Elements = append(e.Boot.Elements, fpl.Element{
				ID: id, Team: club, ElementType: pos, WebName: "P", NowCost: 45,
				Status: "a", Minutes: 2500, Starts: 30, TotalPoints: 100,
			})
		}
	}
	budget := 1000
	e.SquadValue, e.Bank = &budget, new(int)
	return e
}

// TestAChipRebuildHonoursExclusions pins FINDING 1 closed: the rebuild block
// used to build a fresh OptimizeRequest from scratch, so a wildcard week on
// the rail was never given the operator's own locks and exclusions. This
// fails on unmodified `main`, where WeekViews (and so ChipWeekView's
// predecessor) ignores req entirely.
func TestAChipRebuildHonoursExclusions(t *testing.T) {
	e := chipViewTestEngine(t)
	// Make one player unmissable on merit alone, so the only way he could be
	// absent from a wildcard rebuild is the exclusion actually being honoured.
	best := &e.Boot.Elements[0]
	best.TotalPoints = 100000
	best.ExpectedGoalsPer90 = fpl.Num(5)
	best.ExpectedAssistsPer90 = fpl.Num(5)

	req := OptimizeRequest{ExcludeIDs: []int{best.ID}}
	v := e.ChipWeekView(nil, 1, "Wildcard", req)
	if !v.Rebuilt {
		t.Fatal("wildcard week did not rebuild the squad at all")
	}
	for _, p := range v.Squad {
		if p.ID == best.ID {
			t.Fatalf("excluded player %d appears in the wildcard rebuild; "+
				"ChipWeekView is not honouring the caller's req.ExcludeIDs", best.ID)
		}
	}
}

// TestChipWeekViewIgnoresTheConfiguredPlan asks for a wildcard in a gameweek
// the configured chip_plan does not name (e.Chips is left empty here, so
// chipAt would answer "" for every week). ChipWeekView must still rebuild --
// it answers "what would this chip buy in gw", not "what does the plan say
// about gw", which is chipAt/WeekViews's question and not this one.
func TestChipWeekViewIgnoresTheConfiguredPlan(t *testing.T) {
	e := chipViewTestEngine(t)
	e.Chips = ChipSchedule{} // no chip planned anywhere

	v := e.ChipWeekView(nil, 3, "Wildcard", OptimizeRequest{})
	if !v.Rebuilt {
		t.Error("ChipWeekView did not rebuild a wildcard for a week the plan " +
			"does not name; it must answer the hypothetical regardless of chip_plan")
	}
	if v.Chip != "Wildcard" {
		t.Errorf("WeekView.Chip = %q, want %q", v.Chip, "Wildcard")
	}
}

// TestChipWeekViewRefusesAGameweekBehindTheDeadline: a gameweek every one of
// whose fixtures has already finished is one nobody could still chip into.
// ChipWeekView takes no clock, so this is the one signal it can check on the
// fixture data it already holds -- and it must decline the rebuild rather
// than publish a fifteen for a chip that could not have been played.
func TestChipWeekViewRefusesAGameweekBehindTheDeadline(t *testing.T) {
	e := chipViewTestEngine(t)
	// Close gameweek 1: every fixture in it is finished.
	for i := range e.Fixtures {
		if e.Fixtures[i].Event != nil && *e.Fixtures[i].Event == 1 {
			e.Fixtures[i].Finished = true
		}
	}

	squad := []PlayerMetrics{{ID: 1}, {ID: 2}}
	v := e.ChipWeekView(squad, 1, "Wildcard", OptimizeRequest{})
	if v.Rebuilt {
		t.Error("ChipWeekView rebuilt a squad for gameweek 1, whose fixtures " +
			"have all finished -- nobody could still play a wildcard into it")
	}
}
