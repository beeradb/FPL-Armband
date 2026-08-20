package analysis

import (
	"testing"

	"armband/internal/fpl"
)

// TestPlayableChipsReadsTheFeedsOwnWindows.
//
// The planner offered a wildcard in gameweek 1, which the game refuses. The first fix wrote
// the rule out by hand — bench boost and triple captain, gameweek 1 — which was right and
// was a second copy of something the bootstrap publishes per chip. This asserts the answer
// tracks the feed rather than any belief about the rules, so a boundary FPL moves moves here
// with it.
func TestPlayableChipsReadsTheFeedsOwnWindows(t *testing.T) {
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

	for _, tc := range []struct {
		gw   int
		want []string
	}{
		// The bug: a wildcard and a free hit were offered here.
		{1, []string{"bboost", "3xc"}},
		{2, []string{"wildcard", "freehit", "bboost", "3xc"}},
		{19, []string{"wildcard", "freehit", "bboost", "3xc"}},
		// The second set, which the hand-written rule knew nothing about.
		{20, []string{"wildcard", "freehit", "bboost", "3xc"}},
		{38, []string{"wildcard", "freehit", "bboost", "3xc"}},
		// Outside every window. Nothing is offered, rather than everything.
		{39, nil},
		{0, nil},
	} {
		got := PlayableChips(boot, tc.gw)
		if len(got) != len(tc.want) {
			t.Errorf("gameweek %d offers %v, want %v", tc.gw, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("gameweek %d offers %v, want %v", tc.gw, got, tc.want)
				break
			}
		}
	}
}

// TestPlayableChipsFollowsAMovedBoundary is the point of reading the feed.
//
// If FPL opened the wildcard in gameweek 1, the hand-written rule would have gone on
// refusing it and nobody would have noticed until someone read the payload.
func TestPlayableChipsFollowsAMovedBoundary(t *testing.T) {
	boot := &fpl.Bootstrap{Chips: []fpl.Chip{
		{Name: "wildcard", StartEvent: 1, StopEvent: 19},
		{Name: "bboost", StartEvent: 3, StopEvent: 19},
	}}
	got := PlayableChips(boot, 1)
	if len(got) != 1 || got[0] != "wildcard" {
		t.Errorf("gameweek 1 offers %v; the feed opened the wildcard there and closed the "+
			"bench boost, and the answer must follow the feed", got)
	}
}

// TestPlayableChipsSurvivesAnAbsentBootstrap.
//
// The view model builds from a page that may have been assembled without one, and a nil
// dereference on the render path takes the whole document out rather than one rail.
func TestPlayableChipsSurvivesAnAbsentBootstrap(t *testing.T) {
	if got := PlayableChips(nil, 1); got != nil {
		t.Errorf("a nil bootstrap offered %v", got)
	}
}

// TestChipWindowStatusAtCountsSpentChipsOutsideTheRail is the regression test
// for app.js:625's defect: `(4 - GWS.filter(g => g.chip).length) + ' of 4
// left'` only ever looks at the rail, current gameweek and upcoming, so a
// chip already played earlier in the window is never subtracted and the
// count overstates what the reader has left. ChipWindowStatusAt must see it
// regardless of which gameweek is asked about.
func TestChipWindowStatusAtCountsSpentChipsOutsideTheRail(t *testing.T) {
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

	e := &Engine{Boot: boot}
	// Played back at GW5 — long behind the rail by GW15, where "current +
	// upcoming" starts.
	e.Chips.First.BenchBoost = 5

	if got, want := e.ChipWindowStatusAt(15), (ChipWindowStatus{EndsGW: 19, Size: 4, Remaining: 3}); got != want {
		t.Errorf("GW15 (chip spent behind it): status = %+v, want %+v", got, want)
	}
	// Asked about from before the chip was played: still unspent then.
	if got, want := e.ChipWindowStatusAt(3), (ChipWindowStatus{EndsGW: 19, Size: 4, Remaining: 4}); got != want {
		t.Errorf("GW3 (chip not yet played): status = %+v, want %+v", got, want)
	}
	// A chip PLANNED but not yet reached is not spent either.
	e.Chips.First.Wildcard = 18
	if got, want := e.ChipWindowStatusAt(15), (ChipWindowStatus{EndsGW: 19, Size: 4, Remaining: 3}); got != want {
		t.Errorf("GW15 (wildcard planned ahead, bboost spent behind): status = %+v, want %+v", got, want)
	}
	// The second window is untouched by anything planned in the first.
	if got, want := e.ChipWindowStatusAt(25), (ChipWindowStatus{EndsGW: 38, Size: 4, Remaining: 4}); got != want {
		t.Errorf("GW25 (second window): status = %+v, want %+v", got, want)
	}
	// Outside every window: the zero value, not a stale window carried over.
	if got := e.ChipWindowStatusAt(39); got != (ChipWindowStatus{}) {
		t.Errorf("GW39 (outside every window): status = %+v, want the zero value", got)
	}
}
