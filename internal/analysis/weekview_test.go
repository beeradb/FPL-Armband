package analysis

import (
	"strings"
	"testing"

	"armband/internal/fpl"
)

// TestChipAtNeedsARealGameweek pins the trap in the obvious implementation: an
// unplanned chip is 0, so a bare switch on the gameweek would match every chip
// at once for gameweek 0 and, worse, report "Wildcard" for any week if the plan
// were empty.
func TestChipAtNeedsARealGameweek(t *testing.T) {
	e := &Engine{}
	if got := e.chipAt(0); got != "" {
		t.Errorf("chipAt(0) = %q with no plan set, want empty", got)
	}
	if got := e.chipAt(7); got != "" {
		t.Errorf("chipAt(7) = %q with no plan set, want empty", got)
	}

	e.Chips = one(ChipPlan{Wildcard: 6, FreeHit: 16, BenchBoost: 8, TripleCaptain: 9})
	for _, c := range []struct {
		gw   int
		want string
	}{{6, "Wildcard"}, {16, "Free Hit"}, {8, "Bench Boost"}, {9, "Triple Captain"}, {7, ""}} {
		if got := e.chipAt(c.gw); got != c.want {
			t.Errorf("chipAt(%d) = %q, want %q", c.gw, got, c.want)
		}
	}
}

// TestRebuildCaveatMeasuresTheBlendRatherThanTheGameweek is the reason the
// warning is computed rather than hardcoded to "GW1 or 2".
//
// The claim it makes is checkable — with BlendMinutesK at 5, two gameweeks give
// this season 2/(2+5) = 29% of the weight on minutes — and it has to stop
// nagging once the current season carries most of the estimate, or it becomes
// noise that trains the reader to skip it.
func TestRebuildCaveatMeasuresTheBlendRatherThanTheGameweek(t *testing.T) {
	e := &Engine{}
	e.Weights.BlendMinutesK = 5

	e.Boot = bootWithFinished(0)
	got := e.rebuildCaveat()
	if !strings.Contains(got, "No gameweek has been played") {
		t.Errorf("pre-season caveat = %q, want the no-football wording", got)
	}

	e.Boot = bootWithFinished(2)
	got = e.rebuildCaveat()
	if !strings.Contains(got, "29%") {
		t.Errorf("caveat at 2 played = %q, want it to quote the 29%% blend share so a "+
			"reader can check the claim rather than take it on trust", got)
	}

	// Once this season is half the estimate there is nothing left to warn about.
	e.Boot = bootWithFinished(5)
	if got := e.rebuildCaveat(); got != "" {
		t.Errorf("caveat at 5 played = %q, want empty — the blend is 50/50 by then and "+
			"a warning that never stops is one nobody reads", got)
	}
	e.Boot = bootWithFinished(20)
	if got := e.rebuildCaveat(); got != "" {
		t.Errorf("caveat at 20 played = %q, want empty", got)
	}
}

// TestBlanksReadTheWeeksOwnSquad guards the bug the wildcard case introduces: a
// rebuilt week is scored on a different fifteen, so asking the squad you own who
// blanks would name players who are not in the team being shown.
func TestBlanksReadTheWeeksOwnSquad(t *testing.T) {
	owned := []PlayerMetrics{{ID: 1, Name: "kept"}, {ID: 2, Name: "sold"}}
	rebuilt := []PlayerMetrics{{ID: 1, Name: "kept"}, {ID: 3, Name: "bought"}}

	// Only the bought player has no fixture this week.
	w := WeekView{Opponents: map[int][]FixtureBrief{
		1: {{Event: 6, Opponent: "ARS"}},
		3: nil,
	}}

	blanks := w.Blanks(rebuilt)
	if len(blanks) != 1 || blanks[0].ID != 3 {
		t.Fatalf("blanks against the rebuilt squad = %+v, want just the bought player", blanks)
	}
	// And against the owned squad it would name a player who is not playing this
	// week at all — which is why the caller must pass WeekView.Squad.
	if got := w.Blanks(owned); len(got) != 1 || got[0].ID != 2 {
		t.Fatalf("sanity: blanks against the owned squad = %+v", got)
	}
}

// bootWithFinished is a bootstrap whose only meaningful content is how many
// gameweeks have finished, which is all GameweeksPlayed reads.
// TestRebuildCaveatDoesNotDenyFootballThatHasBeenPlayed pins the seventh
// instance of the live-gameweek-gap family (see inLiveGameweekGap's comment).
//
// GameweeksPlayed counts FINISHED events, and FPL does not mark a gameweek
// finished until bonus is locked, hours after the last whistle. In that span the
// counter reads 0 while every match has in fact been played — and the aggregates
// already carry those minutes, so the fifteen has visibly moved on them. Saying
// "No gameweek has been played" there is false twice: about the football, and
// about what the picks rest on.
//
// Observed live on 2026-08-24 on fplarmband.com/wildcard, with all ten GW1
// matches played and the squad changing on that data underneath the sentence
// denying it existed.
func TestRebuildCaveatDoesNotDenyFootballThatHasBeenPlayed(t *testing.T) {
	e := &Engine{}
	e.Weights.BlendMinutesK = 5

	// The gap: no event finished, but a fixture has kicked off.
	e.Boot = bootWithFinished(0)
	e.Fixtures = []fpl.Fixture{{Started: true}}

	got := e.rebuildCaveat()
	if strings.Contains(got, "No gameweek has been played") {
		t.Errorf("caveat during the live gameweek gap = %q\n"+
			"want it NOT to claim no gameweek has been played — ten matches had been "+
			"when this was observed live", got)
	}
	if !strings.Contains(got, "not final") {
		t.Errorf("caveat during the gap = %q, want it to say the gameweek is under way "+
			"but not final", got)
	}

	// Genuinely pre-season — nothing kicked off — still gets the original wording.
	e.Fixtures = []fpl.Fixture{{Started: false}}
	if got := e.rebuildCaveat(); !strings.Contains(got, "No gameweek has been played") {
		t.Errorf("pre-season caveat = %q, want the original no-football wording", got)
	}
}

func bootWithFinished(n int) *fpl.Bootstrap {
	b := &fpl.Bootstrap{}
	for i := 1; i <= 38; i++ {
		b.Events = append(b.Events, fpl.Event{ID: i, Finished: i <= n})
	}
	return b
}
