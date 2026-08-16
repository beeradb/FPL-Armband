package backtest

import (
	"testing"

	"armband/internal/analysis"
)

// The two preparation switches, checked where they can be checked in a
// millisecond rather than inferred from a points table after an hour of replay.
//
// `SimConfig.chipCredit` is the whole of the wiring: everything downstream is
// `analysis.SquadState`, which has its own tests. What can go wrong here is the
// amortisation reading a horizon the gate does not charge over, the window being
// off by one at either end, and — the expensive one — an arm that is on when it
// should be off, which would move every figure recorded before these existed.

func prepConfig(bench, captain bool) SimConfig {
	return SimConfig{
		PrepareBenchBoost:    bench,
		PrepareTripleCaptain: captain,
		Chips:                analysis.ChipPlan{BenchBoost: 20, TripleCaptain: 30},
	}
}

// TestChipPreparationIsOffByDefault is the comparability guard. Every chip figure
// in the record was measured by a policy that could not see a chip coming, and a
// switch that defaults on would retroactively change what they mean.
func TestChipPreparationIsOffByDefault(t *testing.T) {
	cfg := SimConfig{Chips: analysis.ChipPlan{BenchBoost: 20, TripleCaptain: 20}}
	for gw := 1; gw <= 38; gw++ {
		if got := cfg.chipCredit(gw, 5); got.Bench != 0 || got.Captain != 0 {
			t.Fatalf("GW%d credits %+v with both switches off", gw, got)
		}
	}
}

// TestTheChipCreditIsOneWeekOfTheHorizon — the gate charges gain x horizon, and
// a chip pays once. So the per-gameweek credit must be the reciprocal of the same
// horizon the gate is about to multiply back, or the chip is credited a multiple
// of what it actually pays, in exactly the weeks the arm is judged on.
func TestTheChipCreditIsOneWeekOfTheHorizon(t *testing.T) {
	cfg := prepConfig(true, true)
	for _, h := range []float64{1, 2, 5, 8} {
		got := cfg.chipCredit(20, h)
		if want := 1 / h; got.Bench != want {
			t.Errorf("horizon %.0f: bench credit %.4f, want %.4f", h, got.Bench, want)
		}
	}
	// A horizon below one is the end of the season, not an instruction to divide
	// by zero.
	if got := cfg.chipCredit(20, 0); got.Bench != 1 {
		t.Errorf("horizon 0: bench credit %.4f, want 1", got.Bench)
	}
}

// TestTheChipCreditWindowIsClosedAtTheNearEnd — a chip in *this* gameweek still
// counts, because a transfer made now plays in it; one past the far edge does not.
func TestTheChipCreditWindowIsClosedAtTheNearEnd(t *testing.T) {
	cfg := prepConfig(true, false)
	cfg.Chips = analysis.ChipPlan{BenchBoost: 20}

	cases := []struct {
		gw   int
		want bool
	}{
		{19, true},  // one week out, inside a horizon of 5
		{20, true},  // the boost week itself
		{21, false}, // already played
		// A decision at GW n covers n..n+H-1, which is H gameweeks including this
		// one. So GW16 is the far edge — 16, 17, 18, 19, 20 — and GW15 covers
		// 15..19 and stops one short of the chip.
		{16, true},
		{15, false},
	}
	for _, c := range cases {
		got := cfg.chipCredit(c.gw, 5).Bench > 0
		if got != c.want {
			t.Errorf("GW%d with the boost at GW20 and a horizon of 5: credited=%v, want %v",
				c.gw, got, c.want)
		}
	}
}

// TestTheTwoChannelsAreSeparate. They reach different players — the boost buys at
// the cheap end of the squad and the triple captain at the expensive end — so an
// arm that switched both would measure their sum and neither.
func TestTheTwoChannelsAreSeparate(t *testing.T) {
	if got := prepConfig(true, false).chipCredit(20, 5); got.Bench <= 0 || got.Captain != 0 {
		t.Errorf("bench-only arm credits %+v", got)
	}
	if got := prepConfig(false, true).chipCredit(30, 5); got.Captain <= 0 || got.Bench != 0 {
		t.Errorf("captain-only arm credits %+v", got)
	}
}

// TestBothChannelsAreLiveOnTheGridTheyAreMeasuredOn turns "the arm ran" into
// arithmetic, and it exists because of a result rather than as routine hygiene.
//
// The triple-captain arm came back **exactly zero in all 36 cells** of
// `TestDiagChipPreparation`. This record's standing rule is that a knob which does
// not arrive returns a byte-identical null, indistinguishable in the output from a
// null meaning the knob does nothing — and it names that as its own recorded
// failure. `TestTheCaptainCreditReachesTheRankedSearches` shows the credit reaches
// the search; this shows the credit was *non-zero on the weeks the sweep replayed*,
// against the real placement rule and the real grid. Together they leave one
// reading: the credit was live and no decision changed.
//
// It asserts on the count rather than on any single cell, because a chip out of
// reach from a late entry is a legitimate zero.
func TestBothChannelsAreLiveOnTheGridTheyAreMeasuredOn(t *testing.T) {
	cfg := loadConfig(t)

	benchLive, tcLive, benchPlanned, tcPlanned := 0, 0, 0, 0
	for _, pair := range loadPairsOrSkip(t, cfg) {
		for _, start := range sweepStarts() {
			plan := anchoredPlan(pair.Cur, start)
			// The cell's real config, so the horizon is the one the sweep ran on
			// rather than a copy of it. A diagnostic must never carry its own copy
			// of the thing it is checking — the previous version hard-coded 5,
			// which agreed with the shipped Weights.Horizon by luck and ignored the
			// end-of-season shortening entirely.
			sc := sweepConfig(cfg, start, false)
			sc.Chips = plan
			sc.PrepareBenchBoost = true
			sc.PrepareTripleCaptain = true
			var bench, tc bool
			for gw := start; gw <= 38; gw++ {
				c := sc.chipCredit(gw, effectiveHorizon(sc.decisionHorizon(), gw))
				bench = bench || c.Bench > 0
				tc = tc || c.Captain > 0
			}
			if plan.BenchBoost > 0 {
				benchPlanned++
				if bench {
					benchLive++
				}
			}
			if plan.TripleCaptain > 0 {
				tcPlanned++
				if tc {
					tcLive++
				}
			}
		}
	}

	t.Logf("bench boost: planned in %d cells, credit live in %d; triple captain: "+
		"planned in %d, live in %d", benchPlanned, benchLive, tcPlanned, tcLive)

	// A planned chip must always produce a live credit somewhere in the season:
	// the placement rule never puts a chip before the entry gameweek, so there is
	// always a decision week within the horizon of it.
	if benchLive != benchPlanned || tcLive != tcPlanned {
		t.Errorf("a planned chip left the credit dead: bench %d/%d, triple captain %d/%d",
			benchLive, benchPlanned, tcLive, tcPlanned)
	}
	// And the grid must actually place them, or this test passes vacuously and the
	// sweep's null has no witness at all.
	if benchPlanned == 0 || tcPlanned == 0 {
		t.Fatalf("the anchored grid placed %d bench boosts and %d triple captains — "+
			"the sweep's null has no witness", benchPlanned, tcPlanned)
	}
}

// TestAWildcardEndsThePreparationWindow — the squad being valued does not survive
// a wildcard, so a credit must not reach past one.
//
// Without this the transfer search spends free transfers buying a bench for a
// fifteen `playWildcard` replaces wholesale, and the preparation arrives in the
// rebuild for nothing. It is not a hypothetical case: wildcard-into-boost is the
// sequence the chip actually lives in, and it is the next arm anyone will run.
func TestAWildcardEndsThePreparationWindow(t *testing.T) {
	cfg := SimConfig{
		PrepareBenchBoost:    true,
		PrepareTripleCaptain: true,
		Chips:                analysis.ChipPlan{Wildcard: 32, BenchBoost: 33},
	}
	// GW29-31 are inside a five-week horizon of the boost, and all of them are on
	// the far side of the wildcard.
	for gw := 29; gw <= 31; gw++ {
		if got := cfg.chipCredit(gw, 5); got.Bench != 0 {
			t.Errorf("GW%d credits the bench at %.4f with a wildcard at GW32 between it "+
				"and the boost at GW33 — the squad being valued will not survive to play it",
				gw, got.Bench)
		}
	}
	// The wildcard week itself is the rebuild, and the week after is the boost:
	// from GW32 onward there is no wildcard left in front, so preparation is real
	// again. (At GW32 the rebuild is what prepares; ordinary transfers still may.)
	if got := cfg.chipCredit(33, 5); got.Bench <= 0 {
		t.Error("the boost's own week credits nothing — the wildcard barrier has " +
			"swallowed the chip itself")
	}

	// A free hit is NOT a barrier: it fields a temporary fifteen for one week and
	// hands the permanent squad straight back, so a chip beyond it is still this
	// squad's to prepare for.
	fh := SimConfig{
		PrepareBenchBoost: true,
		Chips:             analysis.ChipPlan{FreeHit: 32, BenchBoost: 33},
	}
	if got := fh.chipCredit(30, 5); got.Bench <= 0 {
		t.Error("a free hit between the decision and the boost stopped the credit — it " +
			"does not replace the permanent squad")
	}
}

// TestAnUnplayedChipIsNotPrepared — zero means "never played", and a squad must
// not be built toward a chip nobody plays. The whole grid of cells where a chip
// is out of reach depends on this.
func TestAnUnplayedChipIsNotPrepared(t *testing.T) {
	cfg := SimConfig{PrepareBenchBoost: true, PrepareTripleCaptain: true}
	for gw := 1; gw <= 38; gw++ {
		if got := cfg.chipCredit(gw, 5); got.Bench != 0 || got.Captain != 0 {
			t.Fatalf("GW%d credits %+v with no chips planned", gw, got)
		}
	}
}
