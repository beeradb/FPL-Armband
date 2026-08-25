package backtest

// Which chip carries the anchoring effect: wildcard, bench boost, free hit,
// triple captain, or all four together?
//
//	DIAG=1 FPL_CELLS=<path> go test ./internal/backtest -run TestDiagAnchoredChipDecomposition -v -timeout 45m
//
// TestDiagAnchoredChips bundles bench boost, free hit and triple captain into
// one "anchored" plan and compares it against one "control" plan at fixed
// offsets. That answers whether anchoring is worth anything; it says nothing
// about which chip is doing the work, which is what this file asks.
//
// # The first version of this file was wrong, and this is the repair
//
// It reused the bundled test's `matchedChips`, which intersects playability
// across ALL FIVE of that test's variants (control, full sight, 2/4/6gw lag) at
// once. That is correct for the bundled test — every arm there must play the
// same chip SET — but wrong here: each block in this file compares only TWO
// arms, and the five-way intersection is far stricter than the pair actually
// being measured needs. Triple captain came back at a diluted, near-zero effect
// (−0.029 pts/gw) because 16 of 36 cells had it un-matched under the five-way
// rule and contributed a zero-diff by construction, not because timing did not
// matter for it. Caught by the user asking why a season-scale mean could be
// that small, not by anything in this file.
//
// The fix, `pairMatchedChips`, intersects only the two plans in the block that
// actually gets compared. A chip is isolated if it is played in BOTH the
// control and the anchored plan for that cell — the minimum requirement for a
// timing comparison to mean anything, and no stronger than that.
//
// # Wildcard, added in the repair
//
// The original version excluded wildcard entirely, on the reasoning that it has
// no natural fixed-offset control to contrast against. That is still true, so
// one had to be invented: `wildcardControlOffset` places it late (offset 8,
// between bench boost's 6 and free hit's 10) rather than near GW4, which the
// 2026-08-23 retraction identified as its own confound — an early wildcard is
// valuable for reasons that have nothing to do with fixture anchoring (it
// corrects a squad's opening mistakes), and mixing that in would re-run the
// exact defect that test's second repair removed.
//
// The anchored side reuses `sightedWeeks(...).BenchBoost` — the "biggest double
// within lag" rule — rather than inventing a new placement heuristic. A
// wildcard rebuilds all fifteen, and a double gameweek is exactly when a full
// squad's depth is worth most, so the same selection rule that already governs
// bench boost's placement is the natural one for this chip too.
//
// This is a genuinely new design choice, not a return to something measured
// before, and it has not been independently reviewed.
//
// # Sight length
//
// Fixed at 4 gameweeks for every chip, matching TestDiagAnchoredChips's own bar
// ("if it survives at four it is strategy"). Not swept again here.

import (
	"fmt"
	"os"
	"testing"

	"armband/internal/analysis"
)

// pairMatchedChips returns which chips are played in BOTH a and b — the
// minimum requirement for comparing when a chip is played rather than whether
// it is played at all. Narrower than the bundled test's matchedChips on
// purpose: that function intersects across five variants because every arm
// there must carry the same chip set; a block in this file only ever compares
// two.
func pairMatchedChips(a, b analysis.ChipPlan) [4]bool {
	var out [4]bool
	sa, sb := chipSlots(&a), chipSlots(&b)
	for i := range out {
		out[i] = *sa[i] != 0 && *sb[i] != 0
	}
	return out
}

// isolatedPair builds a control/anchored plan pair for one chip, masked by
// pairMatchedChips computed between the two UNMASKED plans — so masking one
// side cannot itself change what counts as "matched".
func isolatedPair(i int, control, anchored func(cur *Season, start int) analysis.ChipPlan) (
	controlOnly, anchoredOnly func(cur *Season, start int) analysis.ChipPlan) {
	only := func(cur *Season, start int) [4]bool {
		m := pairMatchedChips(control(cur, start), anchored(cur, start))
		var out [4]bool
		out[i] = m[i]
		return out
	}
	controlOnly = func(cur *Season, start int) analysis.ChipPlan {
		return mask(control(cur, start), only(cur, start))
	}
	anchoredOnly = func(cur *Season, start int) analysis.ChipPlan {
		return mask(anchored(cur, start), only(cur, start))
	}
	return
}

// wildcardControlOffset places the control wildcard late — between bench
// boost's fixed offset (6) and free hit's (10) — and deliberately not near
// GW4. See the file header for why an early wildcard is its own confound.
const wildcardControlOffset = 8

func wildcardControl(cur *Season, start int) analysis.ChipPlan {
	var p analysis.ChipPlan
	gw := start + wildcardControlOffset
	if gw > 38 {
		gw = 38
	}
	if gw <= start {
		return p
	}
	p.Wildcard = gw
	return p
}

// wildcardAnchored places the wildcard on the biggest double within lag
// gameweeks of sight — the same rule sightedWeeks already uses for bench
// boost, reused rather than reinvented. See the file header.
func wildcardAnchored(lag int) func(cur *Season, start int) analysis.ChipPlan {
	return func(cur *Season, start int) analysis.ChipPlan {
		var p analysis.ChipPlan
		p.Wildcard = sightedWeeks(cur, start, lag).BenchBoost
		return p
	}
}

// decompChips are the four chips this test isolates, by their chipSlots
// index, each with its own control/anchored plan pair.
func decompChips() []struct {
	index             int
	name              string
	control, anchored func(cur *Season, start int) analysis.ChipPlan
} {
	fixedLagControl := controlWeeks
	fixedLagAnchored := func(cur *Season, start int) analysis.ChipPlan { return sightedWeeks(cur, start, 4) }
	return []struct {
		index             int
		name              string
		control, anchored func(cur *Season, start int) analysis.ChipPlan
	}{
		{0, "wildcard", wildcardControl, wildcardAnchored(4)},
		{1, "bench boost", fixedLagControl, fixedLagAnchored},
		{2, "free hit", fixedLagControl, fixedLagAnchored},
		{3, "triple captain", fixedLagControl, fixedLagAnchored},
	}
}

// tcMatchupAnchored places the triple captain in the gameweek whose best
// available player projects highest — "captain your best man against the worst
// side", which is what managers actually do with this chip in the first half of
// a season, when there are no doubles to aim at.
//
// It is deliberately NOT double-aware beyond what the projection already
// carries, and that is only true because the planning engine is built at
// HORIZON 1 — see the ChipPlannerXP branch in Simulate. At horizon 1 the score
// carries `FixtureLoad`, so a doubling club's best player projects at roughly
// twice a single fixture and a double week wins this comparison on its own
// merits, rather than by a separate rule that would make the two halves of the
// strategy incommensurable.
//
// Verified before running: at the shipped horizon of 5 the projection took FOUR
// distinct values across a whole season and no double was visible at all; at
// horizon 1 it takes fourteen, with the doubles (12.0, 13.6, 14.2) standing
// clear of the ordinary weeks (~5-7) and the blanks (4.4, 5.0) below them.
//
// `capXP` carries no entry for a gameweek nobody plays, so a blank week can
// never be chosen.
func tcMatchupAnchored(cur *Season, start int, capXP map[int]float64) analysis.ChipPlan {
	var p analysis.ChipPlan
	bestGW, bestXP := 0, 0.0
	for gw := start + 1; gw <= 38; gw++ {
		xp, ok := capXP[gw]
		if !ok {
			continue
		}
		// Strictly-better resolves ties to the EARLIEST week, matching
		// sightedWeeks' own tie rule so the two anchoring families cannot
		// disagree about what "the best week" means.
		if xp > bestXP {
			bestGW, bestXP = gw, xp
		}
	}
	p.TripleCaptain = bestGW
	return p
}

// tcMatchupControl is the same fixed offset the other blocks use, isolated to
// the triple captain, expressed in the ChipPlannerXP signature so both arms of
// this comparison are wired through the identical path — the difference between
// the arms is the RULE, not which planner field they happen to use.
func tcMatchupControl(cur *Season, start int, capXP map[int]float64) analysis.ChipPlan {
	var p analysis.ChipPlan
	// Matched to the anchored arm: if that arm cannot place the chip in this
	// cell, neither does this one. Otherwise a cell where only one arm plays a
	// chip contributes a chip-vs-no-chip difference into a mean labelled
	// "timing" — the defect `pairMatchedChips` exists to prevent in the sibling
	// blocks, which this comparison was asserting rather than enforcing.
	if tcMatchupAnchored(cur, start, capXP).TripleCaptain == 0 {
		return p
	}
	p.TripleCaptain = controlWeeks(cur, start).TripleCaptain
	return p
}

// TestDiagTripleCaptainMatchup asks whether the triple captain is worth timing
// on OPPONENT QUALITY, which TestDiagAnchoredChipDecomposition never tested.
//
//	DIAG=1 FPL_CELLS=<path> go test ./internal/backtest -run TestDiagTripleCaptainMatchup -v -timeout 45m
//
// That test placed the chip on the best remaining DOUBLE and found nothing
// (-1.1 points a season, below its own ~5.6 detection threshold). But a
// doubles-only rule is only the second-half half of real practice, and it left
// the chip unplayed in 16 of 36 cells for want of a second double. This is the
// other half.
//
// Both arms play exactly one chip, the triple captain, in every cell where
// either can place it — so this is a pure timing contrast with no chip-set
// difference between arms.
func TestDiagTripleCaptainMatchup(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	starts := sweepStarts()

	fmt.Printf("\n=== triple captain: is it worth timing on OPPONENT QUALITY?\n")
	fmt.Printf("Control places it at a fixed offset from entry. The anchored arm\n")
	fmt.Printf("places it in the gameweek whose best available player projects\n")
	fmt.Printf("highest, from an engine built at the entry cutoff. Metric: POLICY.\n")

	// ⚠️ WeeklyXI is PINNED, and without it this comparison could not work.
	// `runPolicySweep` builds cells at WeeklyXI false, which leaves the weekly
	// view at the shipped horizon of 5 — where `FixtureLoadInScore` is false and
	// the captain is chosen by an engine that cannot see a double gameweek. The
	// rule times the chip on FixtureLoad (that is what horizon 1 buys) and the
	// simulation would then armband whoever a load-blind engine ranked first, so
	// the chip would be timed on a signal it could not cash. Both arms set it, so
	// the contrast stays clean either way — but the anchored arm is the one that
	// needs it.
	arms := []policyVariant{
		{label: "triple captain, fixed offset (control)",
			apply: func(sc *SimConfig) {
				sc.WeeklyXI = true
				sc.ChipPlannerXP = tcMatchupControl
			}},
		{label: "triple captain, best projected matchup",
			apply: func(sc *SimConfig) {
				sc.WeeklyXI = true
				sc.ChipPlannerXP = tcMatchupAnchored
			}},
	}
	runPolicySweep(t, arms, starts)

	fmt.Printf("\nRead against TestDiagAnchoredChipDecomposition's triple-captain\n")
	fmt.Printf("block (-0.029 pts/gw, doubles-only rule, unresolved). A materially\n")
	fmt.Printf("larger effect here would say the chip IS worth timing and the\n")
	fmt.Printf("earlier null was about the RULE rather than about the chip.\n")
}

func TestDiagAnchoredChipDecomposition(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	starts := sweepStarts()

	fmt.Printf("\n=== which chip carries the anchoring effect? Metric: POLICY.\n")
	fmt.Printf("Four separate paired comparisons, one chip isolated per block,\n")
	fmt.Printf("matched pairwise (control vs anchored for THAT chip only), not\n")
	fmt.Printf("across the whole five-variant family. 4 gameweeks of sight.\n")

	for _, c := range decompChips() {
		fmt.Printf("\n--- %s alone ---\n", c.name)
		control, anchored := isolatedPair(c.index, c.control, c.anchored)
		arms := []policyVariant{
			{label: c.name + ", fixed offset (control)",
				apply: func(sc *SimConfig) { sc.ChipPlanner = control }},
			{label: c.name + ", anchored (4gw sight)",
				apply: func(sc *SimConfig) { sc.ChipPlanner = anchored }},
		}
		runPolicySweep(t, arms, starts)
	}

	fmt.Printf("\nRead each chip's block against the bundled result in\n")
	fmt.Printf("TestDiagAnchoredChips (+1.645 pts/gw at 4gw sight, bench boost +\n")
	fmt.Printf("free hit + triple captain together, no wildcard). Wildcard has no\n")
	fmt.Printf("bundled figure to compare against — this is its first measurement.\n")
}
