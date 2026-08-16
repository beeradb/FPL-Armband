package backtest

import (
	"os"
	"testing"
)

// TestDiagXGAggregate sizes the defect rebuildXGAggregates fixes, on real seasons.
//
//	DIAG=1 go test ./internal/backtest -run TestDiagXGAggregate -v -timeout 60m
//
// # What it measures and why in two parts
//
// The **mechanism** table is the cheap half and the one that establishes the defect
// exists: for each pair, the total expected goals a pre-season view carries for the
// prior season, with the aggregate rebuild on and off. Off, a season whose prior has no
// `expected_goals` column reads **zero** — so the opening fifteen is bought with no
// expected goals at all, and every later gameweek blends against a zero prior. That is
// a fact about the payload rather than a measurement, so it needs no standard error.
//
// The **points** table is the expensive half, and it is a paired difference on the two
// metrics the record uses. It is reported per cell rather than pooled, because the
// effect is structurally confined to the pairs whose prior lacks the column — the pairs
// whose prior is 2022-23 or later must come out **byte-identical**, and that invariance
// is a stronger check than any of the moved figures.
//
// # The one that matters beyond the new seasons
//
// The 2021-22 → 2022-23 pair is in the shipped four-season grid, so this is not confined
// to the extended seasons: a figure in the record measured on that cell moves. Read the
// per-cell column, not the mean.
func TestDiagXGAggregate(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)

	// Load with the rebuild OFF first, so both arms come from a parse this test
	// controls. loadSeason's process-global cache would otherwise hand back whichever
	// arm won the race — the hazard the extended-seasons diagnostic documents.
	load := func(name string, agg bool) *Season {
		if agg {
			t.Setenv("FPL_NO_XG_AGGREGATE", "")
		} else {
			t.Setenv("FPL_NO_XG_AGGREGATE", "1")
		}
		s, err := Load(t.Context(), cfg.CacheDir, name)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}

	// The seven-season grid, so the pair that plays 2019-20 off a prior-only 2018-19 is
	// exercised end to end. That pair is the whole point of prior-only loading, and a
	// diagnostic that measured the six it did not need would never have run it.
	pairs := scoringPairNames()

	t.Log("prior      season     prior xG, rebuild off   on        players rebuilt")
	type cell struct{ prior, cur string }
	var moved []cell
	for _, p := range pairs {
		var off, on float64
		var rebuilt int
		for _, agg := range []bool{false, true} {
			s := load(p[0], agg)
			// Summed in element-id order, not map order. Floating-point addition is
			// not associative, so two parses of the same season sum to values that
			// differ in the last bits — enough to make an exact equality check
			// between the two arms report every cell as "moved". That is the
			// map-iteration defect in a new costume: accumulation over a map gives
			// the right *value* and not a reproducible one.
			var xg float64
			for _, id := range sortedPlayerIDs(s) {
				xg += s.Players[id].XG
			}
			if agg {
				on, rebuilt = xg, s.XGRepair.AggFilled
			} else {
				off = xg
			}
		}
		t.Logf("%-10s %-10s %21.1f   %-9.1f %d", p[0], p[1], off, on, rebuilt)
		if on != off {
			moved = append(moved, cell{p[0], p[1]})
		}
		// The invariance check. A prior that already carries a complete aggregate must
		// be untouched: this repair only ever fills a hole, and one cell differing
		// refutes that where confirming an effect costs a replay.
		if !xgRepairs[p[0]].NoAggregate && on != off {
			t.Errorf("%s carries its own expected-goals aggregate and the rebuild "+
				"changed it from %.1f to %.1f", p[0], off, on)
		}
		if xgRepairs[p[0]].NoAggregate && off != 0 {
			t.Errorf("%s has no expected_goals column, so with the rebuild off its "+
				"aggregate should be exactly zero, not %.1f", p[0], off)
		}
	}
	if len(moved) == 0 {
		t.Error("no pair's prior aggregate moved, so this measured nothing — either " +
			"the repair data is absent or the rebuild is not reaching Load")
	}
	t.Logf("cells whose prior xG moves: %v", moved)

	if os.Getenv("EXP") == "" {
		t.Log("set EXP=1 for the paired points comparison (a replay per cell, slow)")
		return
	}

	// The points half: HOLD and POLICY per cell, both arms, at a GW1 entry where the
	// prior carries the most weight. Reported per cell, since the effect cannot reach
	// the pairs whose prior already had an aggregate and pooling would dilute it with
	// guaranteed zeroes.
	//
	// **Read these as "the cell moves", not as a size.** Two reasons, and the second
	// was found by this diagnostic:
	//
	//   - one entry point on one cell is the noisiest way this harness can be read, and
	//     the record's own threshold is tens of points a season;
	//   - `Optimize` is **not deterministic** — see TestDiagOptimizerIsNotDeterministic —
	//     so a pair whose prior did not change can still report a difference. The rows
	//     below therefore carry no assertion; the assertion is on the prior aggregate
	//     above, which is exact.
	t.Log("prior      season     HOLD off   HOLD on    delta    POLICY off  POLICY on   delta")
	for _, p := range pairs {
		var holdOff, holdOn, polOff, polOn int
		for _, agg := range []bool{false, true} {
			prior, cur := load(p[0], agg), load(p[1], agg)
			sc := sweepConfig(cfg, 1, false)
			res, err := Simulate(cur, prior, sc)
			if err != nil {
				t.Fatalf("%s: %v", p[1], err)
			}
			// The Full captaincy rung is HOLD. Taken from HoldCaptaincyWeekly rather
			// than Hold() for the reason the sweep harness gives: one weekly pass
			// produces all three rungs, and two expressions of one quantity end with
			// the measured one not being the one that runs.
			h := sumInts(HoldCaptaincyWeekly(cur, prior, sc, res.OpeningSquad).Full)
			if agg {
				holdOn, polOn = h, res.Points
			} else {
				holdOff, polOff = h, res.Points
			}
		}
		pol := "  (HOLD only)"
		if TransferPathComparable(p[1]) {
			pol = ""
		}
		note := pol
		if !xgRepairs[p[0]].NoAggregate && (holdOn != holdOff || polOn != polOff) {
			// Not an error, and it took a separate investigation to be sure of that.
			// This cell's prior aggregate is provably unchanged — asserted above — so a
			// points difference here cannot be the rebuild. It is Optimize returning a
			// different fifteen from identical inputs.
			note += "  <- prior unchanged; this is optimiser nondeterminism"
		}
		t.Logf("%-10s %-10s %8d   %8d   %+6d   %9d   %9d   %+6d%s",
			p[0], p[1], holdOff, holdOn, holdOn-holdOff,
			polOff, polOn, polOn-polOff, note)
	}
}
