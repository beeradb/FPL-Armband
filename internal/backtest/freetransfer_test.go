package backtest

// The flat `free_transfer_value` ladder — measured and unresolved.
//
//	EXP=FREEVALUE FPL_SWEEP_SEASONS=extended FPL_CELLS=/tmp/freevalue.csv \
//	    scripts/replay -run TestDiagFreeTransferValue -v -timeout 3h
//
// `free_transfer_value` is what a free transfer is charged, in points across the
// decision horizon, and it reaches the simulation as `SimConfig.FreeCost`. It
// ships at 2.0, and until this block ran it had never been varied in any banked
// sweep — every `stats/snapshots/*/cells/*.provenance.csv` stamped it at 2 as an
// ambient constant, so the level was an untested prior question rather than a
// re-tune. It is now measured and **unresolved**: nothing clears its own
// threshold and the ladder has no shape. See
// stats/findings/2026-08-17-free-transfer-value-ladder.md.
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
	"strings"
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
// ⚠️ **It does NOT isolate the funded pair, and an earlier version of this
// comment claimed it did.** `useHit` is `free == 0` in `decide`, so a hit is
// only ever taken after the week's free transfers are spent — which makes
// **every hit week a multi-move week by construction**. This column is the
// funded pair, the multi-free-move week and the hit channel added together, and
// attributing a movement in it to any one of them needs `hits` beside it, which
// the census prints for exactly that reason. `MaxHitsPerWeek` ships at 1, so
// `hits` is also a count of hit *weeks* and the subtraction is exact at shipped
// config.
//
// It is collected because the charge acts through more than one channel and a
// points column cannot see which. What licenses the two-mechanism reading of the
// ladder is the *arithmetic* — see
// TestTheFreeTransferChargeIsInertOnSinglesBelowTheKink — not this count.
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
	trips, moves, multiWeeks, hits, cells int
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
			// ⚠️ Every count comes off `res.Moves`, never `res.Transfers`.
			// `Simulate`'s wildcard branch does `res.Transfers += n` with no
			// append, so a ratio taken across the two would draw numerator and
			// denominator from different populations. Inert today — `sweepConfig`
			// plans no chips, and the two were verified equal on this grid — and
			// latent the moment this block is run with one.
			//
			// ⚠️ The grid table `runPolicySweep` prints above carries its own
			// `moves` column and that one IS `res.Transfers`: two counts under one
			// word in one printout, equal only while no chip is played.
			// `roundTrips` has the same boundary — it cannot see a player
			// wildcarded out and transferred back in.
			observe: func(_ seasonPair, _ int, res *SimResult) {
				c.trips += roundTrips(res.Moves)
				c.moves += len(res.Moves)
				c.multiWeeks += multiMoveWeeks(res.Moves)
				c.hits += res.Hits
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
	fmt.Printf("%-10s %8s %8s %10s %8s %6s %8s %6s\n",
		"value", "moves", "trips", "trips/move", "multi-wk", "hits", "residual", "cells")
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
		fmt.Printf("%-10.1f %8d %8d %10.4f %8d %6d %8d %6d\n",
			fc, c.moves, c.trips, share, c.multiWeeks, c.hits,
			c.multiWeeks-c.hits, c.cells)
	}
	fmt.Printf("\nA move count that does not differ across the ladder means the constant\n")
	fmt.Printf("never reached the decision — a comparison that did not run, not a tie.\n")
	fmt.Printf("⚠️ `multi-wk` does NOT isolate the funded pair: `useHit` is `free == 0`,\n")
	fmt.Printf("so every hit week is a multi-move week. `residual` nets the hit channel\n")
	fmt.Printf("out, exact while MaxHitsPerWeek is 1. Attribute on `residual`, not `multi-wk`.\n")
	fmt.Printf("⚠️ `trips/move` is a ratio of pooled totals over cells of 38 to 13\n")
	fmt.Printf("gameweeks, so the long cells dominate it. Both arms share the cells.\n")
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
// The one exception is the end of the season, where `effectiveHorizon` shortens.
// ⚠️ **It starts at GW35, not GW36/GW37**, and getting that wrong is easy: the
// binding condition is not when a rung's OWN bar clears `MinGain` (`r/H > 0.4`)
// but when it differs from SHIPPED's, and shipped's own bar `2/H` rises above
// `MinGain` the moment `H < 5`. `effectiveHorizon(5, gw)` is `min(5, 39-gw)`, so
// `H = 4` at GW35 and both low rungs are live from there — four end-of-season
// weeks per cell, not two and three. The liveness half below pins that boundary
// rather than merely asserting one exists.
//
// ⚠️ **Three switches make the identity false and none of them ships**, so this
// test refuses to run under them rather than asserting something untrue:
// `budgetWeight` puts a non-zero `Money` in `value()`, the option-value taper
// makes the charge a per-week quantity rather than `FreeCost`, and the unified
// search does not reach the singles branch at all.
func TestTheFreeTransferChargeIsInertOnSinglesBelowTheKink(t *testing.T) {
	cfg := loadConfig(t)
	shipped := cfg.Review.FreeTransferValue
	minGain := cfg.Review.MinGainForTransfer
	// The data state the identity holds in, asserted rather than assumed. A
	// prose caveat cannot fail; this can.
	if budgetWeight > 0 {
		t.Fatalf("budgetWeight is %v: Money enters value(), so the bar is "+
			"max(MinGain, (FreeCost-Money)/H) and this test's identity is false",
			budgetWeight)
	}
	if unifiedTransfers {
		t.Fatal("the unified search is on: it does not reach the singles branch, " +
			"so there is no bar of this shape to pin")
	}
	sc := SimConfig{
		MinGain: minGain, MinGainHit: cfg.Review.MinGainForHit, Weights: cfg.Weights,
	}
	if sc.TaperFreeTransferValue {
		t.Fatal("the option-value taper is on: the charge is FreeCost*factor per " +
			"week, so a single constant does not sit on the kink")
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
	// A gain grid straddling every bar in play, from zero to above the largest.
	//
	// ⚠️ The ceiling must clear `shipped/1` — the bar at the shortest horizon —
	// and not merely the bar at the shipped one. A grid stopping at 1.2 cannot
	// separate a charge of 1.5 from the shipped 2.0 at `H = 1`, where their bars
	// are 1.5 and 2.0, so the boundary assertion below reported a disconnection
	// that was really a grid too narrow to see.
	ceiling := shipped/1.0 + 0.5
	var gains []float64
	for g := 0.0; g <= ceiling+1e-9; g += 0.02 {
		gains = append(gains, g)
	}

	// The positive control. If MinGain ever rose above the grid's ceiling every
	// call below would return false and the inertness loop would pass vacuously,
	// which is the shape of guard this project has been bitten by.
	accepts, refusals := 0, 0
	for _, g := range gains {
		if single(g, shipped, full) {
			accepts++
		} else {
			refusals++
		}
	}
	if accepts == 0 || refusals == 0 {
		t.Fatalf("the gain grid does not straddle the bar at the shipped charge "+
			"(%d accepts, %d refusals): the inertness loop below would pass on a "+
			"constant answer and prove nothing", accepts, refusals)
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
	// nothing": shorten the horizon and the low rungs bind.
	//
	// It pins WHERE they start binding rather than merely that they do. A floor
	// ("something woke somewhere") would pass on a build whose boundary had moved
	// by two gameweeks, which is exactly the error this test's own comment made
	// before review caught it.
	for _, rung := range freeValueRungs {
		if rung >= shipped {
			continue
		}
		for h := 1.0; h <= full; h++ {
			differs := false
			for _, g := range gains {
				if single(g, shipped, h) != single(g, rung, h) {
					differs = true
					break
				}
			}
			switch {
			case h == full && differs:
				t.Fatalf("charge %v differs from the shipped %v at the full horizon "+
					"%v: the singles channel is not pinned below the kink after all",
					rung, shipped, h)
			case h < full && !differs:
				t.Fatalf("charge %v is indistinguishable from the shipped %v at "+
					"horizon %v: shipped's own bar is %v there against min_gain %v, "+
					"so they must differ — FreeCost is not reaching the singles "+
					"branch", rung, shipped, h, shipped/h, minGain)
			}
		}
	}
}

// TestTheSinglesProposalCarriesNoAlternativeOrStrictFlag is the source half of
// the guard above.
//
// The kink identity holds only because `decide` builds the singles proposal with
// `Alternative` zero, `Strict` false and no hit — and the test above hard-codes
// all three into its own literal rather than calling `decide`. So a change
// giving the singles branch `Strict: true` (tidying it up against the funded
// pair, which `gate.go` openly invites) would turn `>= 0` into `> 0`, break the
// inertness at ties, and leave that test passing unchanged while every low-rung
// attribution in the finding silently became wrong.
//
// A source scan rather than a behavioural test, following this package's own
// precedent in TestTheHitCeilingIsReadByTheFundedPairBranch: the property is
// about how the call site is written, and there is no input that reveals it.
func TestTheSinglesProposalCarriesNoAlternativeOrStrictFlag(t *testing.T) {
	src, err := os.ReadFile("simulate.go")
	if err != nil {
		t.Fatal(err)
	}
	// The singles proposal, from its comment through the switch that consumes it.
	const anchor = "// One move, so the package is the move. The alternative is doing nothing,"
	i := strings.Index(string(src), anchor)
	if i < 0 {
		t.Fatalf("cannot find the singles proposal in simulate.go: this guard is "+
			"anchored on %q and the anchor has moved", anchor)
	}
	rest := string(src)[i:]
	end := strings.Index(rest, "default:")
	if end < 0 {
		t.Fatal("cannot find the end of the singles switch in simulate.go")
	}
	block := rest[:end]
	for _, banned := range []string{"Alternative:", "Strict:"} {
		if strings.Contains(block, banned) {
			t.Fatalf("the singles proposal now sets %s. The free-single bar is "+
				"max(MinGain, FreeCost/H) only while Alternative is zero and Strict "+
				"is false; setting either changes it, and "+
				"TestTheFreeTransferChargeIsInertOnSinglesBelowTheKink would go on "+
				"passing against its own hard-coded literal. Re-derive the bar, "+
				"re-read stats/findings/2026-08-17-free-transfer-value-ladder.md, "+
				"then update both.", banned)
		}
	}
	if !strings.Contains(block, "withBar(cfg.MinGain)") {
		t.Fatal("the singles branch no longer gates on withBar(cfg.MinGain): the " +
			"first half of the free-single bar is not min_gain any more, so the " +
			"kink identity does not hold")
	}
}
