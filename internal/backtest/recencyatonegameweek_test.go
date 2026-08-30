package backtest

import (
	"math"
	"testing"

	"armband/internal/analysis"
)

// The recency index is a no-op at one gameweek played, and is not at two.
//
// # Why this is asserted rather than described
//
// `cmd/armband asof` scores a captured moment, and a capture holds the bootstrap
// and the fixtures and nothing else — there is no per-player match history in it
// to build a recency index from. That would be a silent divergence from the live
// path, which wires one whenever a gameweek has been played, so `asof` refuses
// above one gameweek played and runs at or below it.
//
// That cutoff is only honest if the index really does nothing at one gameweek,
// and "the recency-weighted mean over a single gameweek equals the flat
// season-to-date mean" is the kind of arithmetic claim that is true until a
// denominator convention changes underneath it — `newRecentIndexWith` already
// divides per *match* rather than per gameweek, which is exactly such a change
// and would have broken this silently had it landed the other way round.
//
// It is also what lets `asof.go` sit in `unwiredBaseline` without weakening
// `TestEveryScoringEngineGetsRecency`. An entry in that map is a claim that a
// missing index cannot change a number; this is the claim, executed. The
// `webroutes_test.go` entry sets the precedent — its premise is asserted by
// `TestTheFixtureMatchesWhatProductionBuildsAtGW1` rather than left in a comment.
//
// The second half matters as much as the first: a guard that refuses from two
// gameweeks onward is only worth its cost if two gameweeks is where the model
// actually moves. Asserting the divergence keeps this from becoming a rule
// nobody can retire, and would fail loudly if recency were ever switched off.
func TestRecencyIsANoOpAtOneGameweekPlayed(t *testing.T) {
	cfg := loadConfig(t)

	var checkedNoOp, checkedMoves int
	for _, pair := range sweepPairNames() {
		prior := loadSeason(t, cfg, pair[0])
		cur := loadSeason(t, cfg, pair[1])
		sc := sweepConfig(cfg, 1, false)

		// At one gameweek played: identical, to the bit.
		wired, boot := EngineAt(cur, prior, 1, sc)
		bare, _ := EngineAt(cur, prior, 1, sc)
		bare.Recent = nil
		if wired.Recent == nil {
			t.Fatalf("%s: EngineAt built no recency index at through=1, so this test "+
				"is comparing an engine against itself and proves nothing", pair[1])
		}
		var differ int
		for _, el := range boot.Elements {
			if math.Abs(wired.Metrics(&el).ExpectedMinutes-bare.Metrics(&el).ExpectedMinutes) > 1e-9 {
				differ++
			}
		}
		if differ != 0 {
			t.Errorf("%s: %d of %d players differ on ExpectedMinutes at ONE gameweek "+
				"played. The recency index is no longer a no-op there, so `armband "+
				"asof` is scoring a different model than the live path on exactly the "+
				"captures it was built for. Either rebuild the index from the capture "+
				"or lower asof's refusal to GameweeksPlayed() > 0.",
				pair[1], differ, len(boot.Elements))
		}
		checkedNoOp++

		// At two: it moves, which is why the refusal above one exists at all.
		w2, boot2 := EngineAt(cur, prior, 2, sc)
		b2, _ := EngineAt(cur, prior, 2, sc)
		b2.Recent = nil
		differ = 0
		for _, el := range boot2.Elements {
			if math.Abs(w2.Metrics(&el).ExpectedMinutes-b2.Metrics(&el).ExpectedMinutes) > 1e-9 {
				differ++
			}
		}
		if differ == 0 {
			t.Errorf("%s: the recency index changes nothing at TWO gameweeks played "+
				"either. asof's refusal above one gameweek is then costing coverage "+
				"for no reason — or recency has been switched off and several other "+
				"guards are now vacuous.", pair[1])
		}
		checkedMoves++
	}

	if checkedNoOp == 0 || checkedMoves == 0 {
		t.Fatal("no season pairs were checked; the sweep list is empty and this " +
			"test passed without measuring anything")
	}
	t.Logf("checked %d season pairs: recency is a no-op at 1 gameweek, and moves at 2",
		checkedNoOp)
}

// Guard against the optimiser, not just the minutes field, at the cutoff asof
// actually runs at. ExpectedMinutes is the headline but the squad is the output,
// and EngineAt's own comment records a case where reading one as the whole of the
// other produced a wrong diagnosis.
func TestRecencyDoesNotMoveTheSquadAtOneGameweekPlayed(t *testing.T) {
	cfg := loadConfig(t)

	for _, pair := range sweepPairNames() {
		prior := loadSeason(t, cfg, pair[0])
		cur := loadSeason(t, cfg, pair[1])
		sc := sweepConfig(cfg, 1, false)

		wired, _ := EngineAt(cur, prior, 1, sc)
		bare, _ := EngineAt(cur, prior, 1, sc)
		bare.Recent = nil

		a, err := wired.Optimize(analysis.OptimizeRequest{Budget: 1000})
		if err != nil {
			t.Fatalf("%s: optimising the wired engine: %v", pair[1], err)
		}
		b, err := bare.Optimize(analysis.OptimizeRequest{Budget: 1000})
		if err != nil {
			t.Fatalf("%s: optimising the unwired engine: %v", pair[1], err)
		}

		held := map[int]bool{}
		for _, p := range a.StartingXI {
			held[p.ID] = true
		}
		var moved int
		for _, p := range b.StartingXI {
			if !held[p.ID] {
				moved++
			}
		}
		if moved != 0 || math.Abs(a.ExpectedPoints-b.ExpectedPoints) > 1e-9 {
			t.Errorf("%s: dropping the recency index at ONE gameweek played moved %d "+
				"of the starting XI and %+.4f expected points. asof runs here.",
				pair[1], moved, b.ExpectedPoints-a.ExpectedPoints)
		}
	}
}
