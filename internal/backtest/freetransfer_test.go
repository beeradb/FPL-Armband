package backtest

// The flat `free_transfer_value` ladder — the level this constant has never been
// swept at.
//
//	EXP=FREEVALUE FPL_SWEEP_SEASONS=extended FPL_CELLS=/tmp/freevalue.csv \
//	    scripts/replay -run TestDiagFreeTransferValue -v -timeout 3h
//
// `free_transfer_value` is what a free transfer is charged, in points across the
// decision horizon, and it reaches the simulation as `SimConfig.FreeCost`. It
// ships at 2.0 and **has never been varied in any banked sweep**: every
// `stats/snapshots/*/cells/*.provenance.csv` stamps it at 2 as an ambient
// constant, so the level is an untested prior question rather than a re-tune.
//
// Two things want it established. The option-value taper multiplies this same
// constant by a decay-and-congestion factor, so it moves the *mean* charge as
// well as the charge's shape — a taper result is not attributable against a
// level nobody has measured. And the one recorded figure for the constant
// (charging the full four-point hit value dropped transfers from 73 to 39 and
// scored below charging nothing) comes from single-path GW1 replays that predate
// cell banking, paired differences and three fixes worth over a hundred points a
// season. It is a direction with no threshold.
//
// # Why this runs through runPolicySweep
//
// It used to carry its own cell loop and its own `extract` closure, which is
// this package's signature bug at one remove: a second expression of the paired
// difference, emitting no cells, no provenance and no `setting` column, so
// nothing it produced could be banked or read by `stats/sweep_inference.R`. The
// shared harness gives the arms a declared-before-the-first-cell provenance
// sidecar, the per-cell CSV, the invariance and liveness guards, and both
// xPoints instruments for free.
//
// # Which metric
//
// POLICY. This constant is *about* transfers, which is the only case AGENTS.md
// admits for the noisy metric; `policy_xpoints` is read beside it and never
// instead of it.
//
// HOLD is an invariance check rather than a result. `FreeCost` is read only
// inside the weekly transfer decision and HOLD makes no transfers, so HOLD must
// be byte-identical across the ladder. That is a **confinement** — a code fact,
// so re-running it can only fail — and the check with power is its mirror,
// below.
//
// # Liveness
//
// `moves` and `hits` are integer counts observed without noise, and they are the
// arrival evidence: if they do not differ across the ladder, the constant never
// reached the decision and a flat points column is a comparison that never ran
// rather than a tie. Round-trips are counted beside them because the recorded
// claim under test — "a volume brake, not an anti-churn device" — is about the
// *proportion* of moves that are round-trips, which no points column can see.
//
// Pre-registered in
// stats/findings/2026-08-17-free-transfer-value-ladder-PREREGISTRATION.md,
// committed before the run.

import (
	"fmt"
	"math"
	"os"
	"sort"
	"testing"
)

// roundTrips counts players sold and later bought back within one replay.
func roundTrips(moves []Move) int {
	sold := map[int]bool{}
	trips := 0
	for _, m := range moves {
		if m.OutID != 0 {
			sold[m.OutID] = true
		}
		if m.InID != 0 && sold[m.InID] {
			trips++
			sold[m.InID] = false // count each round-trip once
		}
	}
	return trips
}

// multiMoveWeeks counts gameweeks in which the policy spent more than one
// transfer.
//
// It separates the two channels the charge acts through, which is the whole
// reason it is collected. Below the shipped value the single-swap bar does not
// move at all — see TestTheFreeTransferChargeIsInertOnSinglesBelowTheKink — so
// every difference the low rungs can produce arrives through the funded pair,
// whose bar is the pair's value against the best single's. A ladder whose low
// half moves only this column is varying a different mechanism from its high
// half, and a monotone points column across the whole ladder would be two
// readings rather than one shape.
func multiMoveWeeks(moves []Move) int {
	byGW := map[int]int{}
	for _, m := range moves {
		byGW[m.GW]++
	}
	n := 0
	for _, c := range byGW {
		if c > 1 {
			n++
		}
	}
	return n
}

// churn is the round-trip, move and channel census one arm accumulated over the
// grid.
//
// Counts only. No mean, no SE and no threshold: this is a description of what
// the policy did, and turning it into a paired difference would invent a second
// inference path beside the one in stats/sweep_inference.R.
type churn struct {
	trips, moves, pairWeeks, cells int
}

// freeValueRungs is the ladder's non-shipped arms, in one place.
//
// The diagnostic and the bracketing guard below both read it. Writing the rungs
// out twice — once swept, once asserted — is this package's signature failure at
// small scale: the diagnostic's ladder could go one-sided and a guard carrying
// its own copy would still pass.
var freeValueRungs = []float64{1.0, 1.5, 3.0, 4.0}

func TestDiagFreeTransferValue(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)
	shipped := cfg.Review.FreeTransferValue

	// 2.0 first: variants[0] is the baseline every paired difference is taken
	// against, and the pre-registration fixes it as the shipped value. The rest
	// bracket it on both sides, because a one-sided ladder cannot tell an
	// interior optimum from a slope — the defect the BLENDLO block exists to
	// repair on a different constant.
	ladder := append([]float64{shipped}, freeValueRungs...)
	seen := map[float64]bool{}
	for _, fc := range ladder {
		if seen[fc] {
			t.Fatalf("free_transfer_value %.1f appears twice in the ladder: the "+
				"shipped value is %.1f and must not be repeated as a rung",
				fc, shipped)
		}
		seen[fc] = true
	}

	census := map[float64]*churn{}
	var v []policyVariant
	for _, fc := range ladder {
		label := fmt.Sprintf("free_value=%.1f", fc)
		if fc == shipped {
			label += " (ships)"
		}
		c := &churn{}
		census[fc] = c
		v = append(v, policyVariant{
			label: label,
			apply: func(sc *SimConfig) { sc.FreeCost = fc },
			// Read back off the applied config rather than declared, so the
			// column cannot describe a setting the cell did not have. This
			// family is a genuine ordered ladder in one scalar, which is what
			// licenses stats/schedule_screen.R to read a slope across it.
			setting: func(sc SimConfig) float64 { return sc.FreeCost },
			// The churn census. It may not touch the simulation — the season is
			// already played by the time this runs.
			// ⚠️ Both readings come off `res.Moves`, never `res.Transfers`.
			// `res.Transfers` also counts a wildcard's replacements, which never
			// enter `res.Moves`, so a ratio taken across the two would have its
			// numerator and denominator from different populations. Inert today —
			// `sweepConfig` plans no chips — and latent the moment this block is
			// run with one.
			observe: func(_ seasonPair, _ int, res *SimResult) {
				c.trips += roundTrips(res.Moves)
				c.moves += len(res.Moves)
				c.pairWeeks += multiMoveWeeks(res.Moves)
				c.cells++
			},
		})
	}

	fmt.Printf("\n=== FREEVALUE. free_transfer_value, flat ladder. Metric: POLICY.\n")
	fmt.Printf("Shipped %.1f. HOLD must not move — FreeCost is read only inside the\n", shipped)
	fmt.Printf("weekly transfer decision, so a moved HOLD means the constant leaked.\n")
	runPolicySweep(t, v, sweepStarts())

	// The churn census, printed after the grid. Proportions rather than counts
	// are what the recorded claim is about: raising the charge is expected to cut
	// moves, and the question is whether the round-trip *share* moves with it.
	fmt.Printf("\n--- churn census (counts; no threshold is claimed for these) ---\n")
	fmt.Printf("%-10s %8s %8s %10s %10s %8s\n",
		"value", "moves", "trips", "trips/move", "pair weeks", "cells")
	keys := make([]float64, 0, len(census))
	for fc := range census {
		keys = append(keys, fc)
	}
	sort.Float64s(keys)
	for _, fc := range keys {
		c := census[fc]
		share := 0.0
		if c.moves > 0 {
			share = float64(c.trips) / float64(c.moves)
		}
		fmt.Printf("%-10.1f %8d %8d %10.4f %10d %8d\n",
			fc, c.moves, c.trips, share, c.pairWeeks, c.cells)
	}
	fmt.Printf("\nA move count that does not differ across the ladder means the constant\n")
	fmt.Printf("never reached the decision — a comparison that did not run, not a tie.\n")
	fmt.Printf("`pair weeks` separates the two channels: below the shipped value the\n")
	fmt.Printf("single-swap bar is pinned by min_gain and only the funded pair can move.\n")
}

// TestTheFreeTransferLadderBracketsTheShippedValue pins the design defect the
// ladder exists to avoid, without running a cell.
//
// A ladder whose lowest rung IS its baseline cannot distinguish an interior
// optimum from an unbounded one, which is precisely what left `BlendRateK`'s
// banked run unreadable and cost a second sweep. The guard is cheap and the
// mistake is easy to reintroduce by deleting an arm that looks redundant.
func TestTheFreeTransferLadderBracketsTheShippedValue(t *testing.T) {
	cfg := loadConfig(t)
	shipped := cfg.Review.FreeTransferValue
	if shipped <= 0 {
		t.Fatalf("free_transfer_value is %v: the ladder below brackets a positive "+
			"shipped value", shipped)
	}
	// Read off the diagnostic's own rungs rather than a second copy of them. A
	// guard asserting a *value* must not import its subject; a guard asserting a
	// *property* must, or the property can stop holding of the thing that runs
	// while the guard goes on passing.
	below, above := 0, 0
	for _, fc := range freeValueRungs {
		switch {
		case fc < shipped:
			below++
		case fc > shipped:
			above++
		default:
			t.Fatalf("rung %v equals the shipped value: the baseline is not a rung", fc)
		}
	}
	if below < 1 || above < 1 {
		t.Fatalf("the ladder has %d rungs below the shipped %v and %d above: a "+
			"one-sided ladder cannot tell an interior optimum from a slope",
			below, shipped, above)
	}
}

// TestTheFreeTransferChargeReachesTheDecision is the liveness guard as a test
// rather than as a remembered fact.
//
// A gate constant that never arrives returns a byte-identical null, which looks
// exactly like a null meaning the knob does nothing — this package's most
// expensive failure mode and the reason the sweep above counts moves. Two
// proposals identical but for the charge must be decided differently at charges
// far enough apart, or the ladder cannot express anything.
func TestTheFreeTransferChargeReachesTheDecision(t *testing.T) {
	// A gain that clears the floor and pays for a charge of 2 across a horizon
	// of 5, but not a charge of 4: 0.5 x 5 = 2.5.
	p := transferProposal{
		Moves: []Move{{}}, Gain: 0.5, Horizon: 5, GainBar: 0.4, FreeCost: 2.0,
	}
	// MinGainHit is never read here — `Hits` is zero on every proposal below —
	// so it is set to the shipped value rather than to a convenient one, so the
	// fixture cannot be misread as a claim that 0.4 is the hit bar.
	cheap := SimConfig{
		MinGain:    0.4,
		MinGainHit: loadConfig(t).Review.MinGainForHit,
		Weights:    loadConfig(t).Weights,
	}
	if !gateDecision(cheap, nil, p) {
		t.Fatalf("a gain of %v over %v gameweeks does not clear a charge of %v: "+
			"the fixture this guard is built on no longer discriminates",
			p.Gain, p.Horizon, p.FreeCost)
	}
	dear := p
	dear.FreeCost = 4.0
	if gateDecision(cheap, nil, dear) {
		t.Fatal("raising free_transfer_value from 2.0 to 4.0 accepted the same " +
			"proposal: the charge does not reach the gate, so a flat ladder " +
			"would be a comparison that never ran rather than a null")
	}
}

// TestTheFreeTransferChargeIsInertOnSinglesBelowTheKink pins the confound that
// governs how this ladder may be read, as arithmetic rather than as an argument.
//
// A free single is accepted iff `Gain >= MinGain` **and** `Gain*H - FreeCost >= 0`
// (`transferProposal.value` with one move, no hit, no money, against an
// Alternative of zero). So its effective bar is `max(MinGain, FreeCost/H)`, and
// at the shipped `DecisionHorizon` of 5 with `MinGain` 0.4 the shipped charge of
// **2.0 sits exactly on the kink**: `2.0/5 = 0.4`.
//
// That is the mirror image of the recorded `min_gain` result — inert at or below
// 0.4 for the same reason from the other side — and it has a consequence for any
// ladder spanning 2.0. **Below the shipped value the single-swap channel cannot
// move at all**, so the low rungs act only through the funded pair, while the
// high rungs act through both. A monotone points column across the whole ladder
// is therefore two readings and not one shape, and an interior optimum at 2.0 is
// confounded with `MinGain x DecisionHorizon`.
//
// The one exception is the end of the season, where `effectiveHorizon` shortens:
// at H = 2 a charge of 1.0 demands 0.5 and does bind. That is GW37 onward for
// 1.0 and GW36 onward for 1.5, which is why the test walks the horizon.
func TestTheFreeTransferChargeIsInertOnSinglesBelowTheKink(t *testing.T) {
	cfg := loadConfig(t)
	shipped := cfg.Review.FreeTransferValue
	minGain := cfg.Review.MinGainForTransfer
	sc := SimConfig{
		MinGain: minGain, MinGainHit: cfg.Review.MinGainForHit, Weights: cfg.Weights,
	}
	full := float64(sc.decisionHorizon())

	// The kink itself: the shipped charge is exactly MinGain across the shipped
	// horizon. If this stops holding the two constants have drifted apart and
	// every reading below is void.
	if got := shipped / full; math.Abs(got-minGain) > 1e-9 {
		t.Fatalf("free_transfer_value %v over horizon %v is %v, and min_gain is %v: "+
			"the shipped charge no longer sits on the kink, so the inertness this "+
			"test pins no longer follows", shipped, full, got, minGain)
	}

	single := func(gain, charge, horizon float64) bool {
		return gateDecision(sc, nil, transferProposal{
			Moves: []Move{{}}, Gain: gain, Horizon: horizon,
			GainBar: minGain, FreeCost: charge,
		})
	}
	// A gain grid straddling the bar from well below to well above it.
	var gains []float64
	for g := 0.0; g <= 1.2000001; g += 0.02 {
		gains = append(gains, g)
	}

	for _, rung := range freeValueRungs {
		if rung >= shipped {
			continue
		}
		for _, g := range gains {
			if want, got := single(g, shipped, full), single(g, rung, full); want != got {
				t.Fatalf("at gain %.2f over the shipped horizon %v, charge %v decides "+
					"%v and the shipped %v decides %v: the single-swap channel is "+
					"supposed to be pinned by min_gain below the kink",
					g, full, rung, got, shipped, want)
			}
		}
	}

	// And the mirror, which is what stops this reading as "the charge does
	// nothing": shorten the horizon and the low rungs do bind. Without this the
	// test would pass on a build where FreeCost had been disconnected entirely.
	woke := 0
	for _, rung := range freeValueRungs {
		if rung >= shipped {
			continue
		}
		for h := full - 1; h >= 1; h-- {
			for _, g := range gains {
				if single(g, shipped, h) != single(g, rung, h) {
					woke++
					break
				}
			}
		}
	}
	if woke == 0 {
		t.Fatal("no sub-shipped charge changes a single-swap decision at any " +
			"shortened horizon: FreeCost is not reaching the singles branch at " +
			"all, which is a disconnection rather than the kink this test pins")
	}
}
