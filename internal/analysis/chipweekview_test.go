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
	// A closed gameweek is never eligible to rebuild in the first place -- it
	// is not the same zero-Rebuilt state as an eligible rebuild that failed,
	// and RebuildFailed must not blur the two together.
	if v.RebuildFailed {
		t.Error("ChipWeekView marked a closed gameweek as a FAILED rebuild; " +
			"it was never eligible to rebuild at all, so RebuildFailed should " +
			"stay false here")
	}
}

// TestAFailedRebuildIsDistinguishableFromAConfirmedNoChangeRebuild pins the
// defect this exists to fix: before RebuildFailed existed, a wildcard/free-hit
// rebuild that COULD NOT RUN (Optimize errored) and one that ran and
// confirmed the caller's own squad was already optimal both left Squad equal
// to the squad passed in and Rebuilt false -- indistinguishable downstream.
// buildChipTeam (internal/viewmodel/build.go) diffs Squad against the
// account's real fifteen to report Changes/Out, so a FAILED rebuild rendered
// as "0 changes, nothing transferred out": a confident answer, no error, no
// caveat, served publicly and cached for 300s by /api/wildcard
// (cmd/armband/chipteams.go). This asserts the two states now read
// differently on WeekView itself, which is where buildChipTeam reads them
// from.
func TestAFailedRebuildIsDistinguishableFromAConfirmedNoChangeRebuild(t *testing.T) {
	squad := []PlayerMetrics{{ID: 1}, {ID: 2}}

	t.Run("failed rebuild", func(t *testing.T) {
		e := chipViewTestEngine(t)
		// A budget too small to buy even one of this pool's £4.5m players (all
		// NowCost: 45) forces Optimize to fail on infeasibility -- a stand-in
		// for any Optimize error, since ChipWeekView's error path does not
		// discriminate by cause (see AGENTS.md: "a design defect in the error
		// path... misfires identically for any cause of Optimize failure").
		tiny := 1
		e.SquadValue, e.Bank = &tiny, new(int)

		v := e.ChipWeekView(squad, 1, "Wildcard", OptimizeRequest{})
		if v.Rebuilt {
			t.Fatal("Optimize should have failed against a £0.1m budget, but " +
				"v.Rebuilt is true")
		}
		if !v.RebuildFailed {
			t.Fatal("a wildcard was eligible (open gameweek, chip named) but " +
				"Optimize could not complete it, so RebuildFailed must be true")
		}
		if v.Caveat == "" {
			t.Error("a failed rebuild must say so in Caveat -- silence here is " +
				"exactly the defect: the page would show the squad with no " +
				"warning at all")
		}
		if len(v.Squad) != len(squad) || v.Squad[0].ID != squad[0].ID {
			t.Errorf("a failed rebuild must leave Squad as the squad passed in, "+
				"got %+v", v.Squad)
		}
	})

	t.Run("confirmed no-change rebuild", func(t *testing.T) {
		e := chipViewTestEngine(t)
		// A generous budget lets Optimize actually run and succeed. Whatever
		// fifteen it returns, success is signalled by Rebuilt alone --
		// RebuildFailed must be false here even though nothing about a
		// two-player placeholder squad looks "changed" by eye.
		v := e.ChipWeekView(squad, 1, "Wildcard", OptimizeRequest{})
		if !v.Rebuilt {
			t.Fatal("Optimize should have succeeded against the ample default " +
				"budget chipViewTestEngine sets, but v.Rebuilt is false")
		}
		if v.RebuildFailed {
			t.Error("a successful rebuild must never also read as a failed " +
				"one -- the two are meant to be mutually exclusive")
		}
	})
}
