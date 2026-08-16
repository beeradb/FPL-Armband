package analysis

import "testing"

// TestFixtureLoadIsAppliedOnce pins the collision that a comment used to assert
// was impossible.
//
// Two consumers apply the same multiplier from different files: Metrics scales
// Score by FixtureLoad when FixtureLoadInScore() is true, which happens at
// horizon 1; and XIValue scales Score by FixtureLoad for the transfer decision.
// XIValue carried the argument that these cannot meet, because "that path picks
// the eleven through BestXI and never calls this" — true when written, and
// falsified by SimConfig.AnticipateChips, which puts the transfer engine at
// horizon 1 in the gameweek before a chip. A doubling club was valued 4x rather
// than 2x in exactly those weeks.
//
// The guard is a fact carried on the value (PlayerMetrics.loadInScore) rather
// than a condition re-derived by the second consumer, because re-deriving it
// would be the second implementation of one quantity — the bug class this
// package has paid for more than once.
func TestFixtureLoadIsAppliedOnce(t *testing.T) {
	if !fixtureLoadTransfers {
		t.Skip("fixture load is switched off for transfers")
	}
	// A legal fifteen where one player's club plays twice. Scores are flat so the
	// only thing that can move XIValue is the multiplier.
	squad := func(loadInScore bool) []PlayerMetrics {
		var out []PlayerMetrics
		add := func(pos string, n int) {
			for i := 0; i < n; i++ {
				out = append(out, PlayerMetrics{Position: pos, Score: 1, FixtureLoad: 1})
			}
		}
		add("GKP", 2)
		add("DEF", 5)
		add("MID", 5)
		add("FWD", 3)
		// The doubling player, and the only one that differs between arms.
		out[7].FixtureLoad = 2
		out[7].Score = 5
		if loadInScore {
			out[7].Score *= out[7].FixtureLoad
			out[7].loadInScore = true
		}
		return out
	}

	// Metrics has NOT applied it: XIValue must, so the doubler is worth 2x.
	raw := XIValue(squad(false))
	// Metrics HAS applied it: XIValue must not, so the doubler is worth the same
	// 2x rather than 4x. Identical totals is the whole assertion.
	already := XIValue(squad(true))

	if raw != already {
		t.Errorf("XIValue = %.4f when Score carries the load and %.4f when it does "+
			"not; the multiplier is being applied twice on one of these paths, "+
			"which is what squares a double gameweek", already, raw)
	}

	// And the guard must not have disabled the multiplier altogether: a doubling
	// club has to be worth more than a single one somewhere.
	flat := make([]PlayerMetrics, 15)
	copy(flat, squad(false))
	flat[7].FixtureLoad = 1
	if XIValue(flat) >= raw {
		t.Errorf("a doubling club is worth no more than a single fixture "+
			"(%.4f against %.4f) — the load is not being applied at all",
			XIValue(flat), raw)
	}
}
