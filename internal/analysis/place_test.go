package analysis

import (
	"testing"

	"armband/internal/fpl"
)

// TestAPlacementGoesInTheSetWhoseWindowHoldsIt.
//
// FPL grants two sets of chips, the second from gameweek 20. A caller that knows only "the
// reader chose gameweek 25" cannot tell which set that is without the feed, and filing it in
// the wrong one is invisible: the plan is legal, the engine acts on it, and the squad is
// built around a chip the reader will not have. The planner did exactly that — every
// placement went into the first set — until this.
func TestAPlacementGoesInTheSetWhoseWindowHoldsIt(t *testing.T) {
	boot := &fpl.Bootstrap{Chips: []fpl.Chip{
		{Name: "wildcard", StartEvent: 2, StopEvent: 19},
		{Name: "freehit", StartEvent: 2, StopEvent: 19},
		{Name: "bboost", StartEvent: 1, StopEvent: 19},
		{Name: "3xc", StartEvent: 1, StopEvent: 19},
		{Name: "wildcard", StartEvent: 20, StopEvent: 38},
		{Name: "freehit", StartEvent: 20, StopEvent: 38},
		{Name: "bboost", StartEvent: 20, StopEvent: 38},
		{Name: "3xc", StartEvent: 20, StopEvent: 38},
	}}

	var s ChipSchedule
	s, ok := s.Place(boot, "bboost", 5)
	if !ok {
		t.Fatal("a bench boost in gameweek 5 was refused")
	}
	if s.First.BenchBoost != 5 {
		t.Errorf("the first set's bench boost is GW%d, want 5", s.First.BenchBoost)
	}
	if s.Second.BenchBoost != 0 {
		t.Errorf("a gameweek-5 placement reached the SECOND set (GW%d)", s.Second.BenchBoost)
	}

	// The one that was wrong.
	s, ok = s.Place(boot, "wildcard", 25)
	if !ok {
		t.Fatal("a wildcard in gameweek 25 was refused, and the feed allows it")
	}
	if s.Second.Wildcard != 25 {
		t.Errorf("the second set's wildcard is GW%d, want 25", s.Second.Wildcard)
	}
	if s.First.Wildcard != 0 {
		t.Errorf("a gameweek-25 wildcard was filed in the FIRST set (GW%d). The plan is "+
			"legal and the engine acts on it, so nothing reports this — the squad is simply "+
			"built around a chip the reader will not have.", s.First.Wildcard)
	}
	// And the earlier placement survived, because a schedule holds one of each per set.
	if s.First.BenchBoost != 5 {
		t.Errorf("placing a wildcard cleared the bench boost (now GW%d)", s.First.BenchBoost)
	}
}

// TestAPlacementOutsideEveryWindowChangesNothing.
//
// The endpoint validates placements, so this is the second line rather than the first. It
// matters because the failure it prevents is silent and durable: a stored session written
// before a rule changed would otherwise put a chip into a plan the engine then builds around.
func TestAPlacementOutsideEveryWindowChangesNothing(t *testing.T) {
	boot := &fpl.Bootstrap{Chips: []fpl.Chip{
		{Name: "wildcard", StartEvent: 2, StopEvent: 19},
	}}

	for _, tc := range []struct {
		name string
		key  string
		gw   int
	}{
		{"before the window opens", "wildcard", 1},
		{"after it closes", "wildcard", 20},
		{"a chip the feed does not carry", "bboost", 5},
		{"a chip nobody has heard of", "tripleeverything", 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var s ChipSchedule
			got, ok := s.Place(boot, tc.key, tc.gw)
			if ok {
				t.Errorf("accepted %s in gameweek %d", tc.key, tc.gw)
			}
			if got != s {
				t.Errorf("a refused placement changed the schedule: %+v", got)
			}
		})
	}
}
