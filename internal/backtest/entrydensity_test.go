package backtest

// Is the entry-gameweek axis really worth densifying?
//
//	DIAG=1 FPL_SWEEP_STARTS="1,2,3,4,6,11,16,17,18,19,21,26" \
//	  FPL_CELLS=/tmp/density.csv \
//	  go test ./internal/backtest -run TestDiagEntryDensity -count=1 -v -timeout 180m
//
// # The claim under test
//
// AGENTS.md records the entry-point axis as the one noise remedy its own
// measurements support on HOLD, and publishes a falsifiable prediction from
// Var(mean) = (sd_season^2 + sd_resid^2/G)/S with G entry points and S = 4
// seasons: the season-clustered SE on HOLD should fall 0.515 -> 0.364 -> 0.257 ->
// 0.182 at G = 6, 12, 24, 48, because sd_season is measured at zero there and the
// whole of the spread is within-season path noise.
//
// That arithmetic assumes the G residuals within a season are **independent**, and
// entry points are strictly nested: SimConfig carries StartGW and no EndGW, so
// every window runs to GW38 and an entry at GW2 shares 37 of its 38 gameweeks with
// an entry at GW1. At the shipped spacing of 5 the correlation is evidently
// tolerable. At spacing 1 or 2 it may not be — and if adjacent cells are
// near-duplicates then the 1/G shrinkage is fictional and densifying manufactures
// confidence exactly the way the budget-jitter axis would have. AGENTS.md's own
// rule: the value of an extra axis is extra *independent* samples.
//
// So the quantity wanted is not a season total. It is **how fast the correlation
// between two entry points' paired differences falls off with the gap between
// them**, which is why this block exists separately from TestDiagProjection: here
// the grid is the object of study rather than the instrument.
//
// # Why this grid
//
// FPL_SWEEP_STARTS is deliberately not defaulted (see sweepStarts). The grid this
// was measured on keeps the shipped six and adds two tight clusters:
//
//	1, 2, 3, 4, 6, 11, 16, 17, 18, 19, 21, 26
//
// which gives 6 pairs at spacing 1, 4 at spacing 2, 2 at spacing 3 and 5 at
// spacing 5, each times four seasons — so the short spacings are replicated rather
// than each resting on one pair. Two clusters at different points in the season,
// because AGENTS.md finds MinutesHalfLife's entire HOLD signal at the GW26 entry
// and ±0.16 at GW1, so the correlation structure may itself differ early against
// late and one cluster could not show that.
//
// **This grid over-samples short spacings on purpose, so its own clustered SE is
// not an estimate of "the SE at G = 12".** An evenly-spread twelve would do
// better, containing fewer near-duplicate pairs. The extrapolation to an evenly
// spread grid is done in stats/entry_density.R, from the fitted
// correlation-versus-spacing curve — and it is a fit, stated as one.
//
// # Why MinutesWeight, and why two arms
//
// MinutesWeight 1.25 against 1.00 is a **positive control**: it is characterised
// in AGENTS.md at −0.709 to −0.717 pts/gw on HOLD with CR2 t = −5.95, so the
// effect is known to be real and roughly known in size. That is what makes a
// change in the standard error interpretable — a metric or a grid can always be
// made quieter by measuring less, and the only defence is to watch a known effect
// while the noise moves. It is the same selection criterion the captaincy rungs
// were judged on and failed.
//
// Two arms rather than the four in TestDiagProjection's MINW block, because 48
// cells per arm is the cost here and the correlation structure needs one
// comparison, not four. POLICY is emitted as usual but is not the question:
// AGENTS.md measures POLICY's path axis at 88% of its irreducible floor, so
// densification cannot help there whatever this finds.

import (
	"fmt"
	"testing"
)

func TestDiagEntryDensity(t *testing.T) {
	requireDiag(t)
	starts := sweepStarts()
	fmt.Printf("\n=== entry-point density: MinutesWeight 1.25 vs 1.00 (positive control, "+
		"1.25 no longer ships) at %d starts. Metric: HOLD.\n", len(starts))
	if len(starts) <= 6 {
		fmt.Printf("NOTE: running at the shipped grid — %d cells per arm, and no pair "+
			"closer than five gameweeks. Set FPL_SWEEP_STARTS to densify.\n",
			len(starts)*len(sweepPairNames()))
	}

	var v []policyVariant
	for _, x := range []float64{1.25, 1.0} {
		label := fmt.Sprintf("exponent %.2f", x)
		if x == 1.0 {
			label += " (ships)"
		}
		v = append(v, policyVariant{label: label, apply: func(sc *SimConfig) {
			sc.WeeklyXI = true
			sc.Weights.MinutesWeight = x
		}})
	}
	runPolicySweep(t, v, starts)
}
