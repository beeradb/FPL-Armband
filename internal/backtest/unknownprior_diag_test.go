package backtest

import (
	"fmt"
	"testing"
)

// DOES GIVING AN UNKNOWN PLAYER A PRIOR EARN POINTS?
//
//	DIAG=1 EXP=UNKNOWNPRIOR FPL_CELLS=/tmp/up.csv scripts/replay \
//	    -run TestDiagUnknownPrior -v -timeout 4h
//
// # What changed, and why it needs three arms rather than two
//
// `blendRatesCode` returned early before the season started, and a player with
// no prior fell through that branch at **exactly zero expected minutes** — never
// reaching the `shrinkToLeague` the in-season path sends the same case to. Across
// six seasons that was 122 to 284 players a season, all reading 0.0000, so the
// model had no ordering at all over a fifth of the pool it picks an opening
// squad from.
//
// Two things were done about it and they are **not independent**:
//
//   - the prior fix gives such a player his position's league average, which
//     makes the population SCORABLE but not ORDERED — every unknown in a
//     position gets the same number;
//   - `PriceMinutesPrior` then tilts that league term by where he sits in his
//     own position's price distribution, which is what orders them.
//
// Folding them into one arm would measure their sum and neither. This record has
// already paid for that: three sweeps in one session had to be re-run because two
// things moved at once.
//
// # PRE-REGISTERED, and the prediction comes from a DIFFERENT instrument
//
// The ordering benchmark (`TestDiagOwnershipPredictsMinutes`) measured all three
// states against minutes actually played in GW1-10, six seasons, Spearman:
//
//	                     no-history stratum      has-history stratum
//	baseline (zero)      FLAT — undefined        0.565 to 0.640
//	prior fix only       -0.190 to -0.058        unchanged
//	fix + price tilt     +0.253 to +0.474        unchanged
//
// So, before running:
//
//   - **The fix alone should be neutral to slightly NEGATIVE.** It lifts unknowns
//     off zero without ordering them, and on the ordering instrument it was
//     negative in all six seasons. If it comes out clearly positive here, the two
//     instruments disagree and that is the finding rather than the points.
//   - **Fix plus price should beat fix alone.** That is the contrast this exists
//     for, and it is a within-cell difference of differences.
//   - ⚠️ **Nothing here is expected to clear a threshold on its own.** The
//     opening fifteen's gap from ideal is several points a week and this touches
//     part of one squad; HOLD pools over the whole season, so a first-weeks
//     effect is diluted. **A null on the arms with a positive contrast between
//     them is the most likely honest outcome.**
//   - ⚠️ **Uninterpretable if the arms move no players.** `moves` is printed;
//     read it before reading anything else.
//
// # ⚠️ Read HOLD, not POLICY
//
// HOLD is the opening fifteen with no transfer policy in the way, which is what a
// prior builds. POLICY adds the weekly transfer decision, whose noise is far
// larger than anything a pre-season prior can be worth.
func TestDiagUnknownPrior(t *testing.T) {
	requireDiag(t)
	starts := sweepStarts()

	fmt.Printf("\n=== DOES A PRIOR FOR THE UNKNOWN EARN POINTS, AND DOES PRICE ORDER IT?\n")
	fmt.Printf("Baseline reproduces the defect: a player with no prior reads zero\n")
	fmt.Printf("expected minutes before the season starts.\n")
	fmt.Printf("⚠️ Pre-registered from the ORDERING instrument, AMENDED 2026-08-28\n")
	fmt.Printf("before this sweep was ever run, because its basis was withdrawn.\n")
	fmt.Printf("It read: the fix alone is negative on ordering in every season, so\n")
	fmt.Printf("expect it flat-to-negative here. That negative is a POOLING artefact\n")
	fmt.Printf("-- within position the fix's prediction is CONSTANT in every season,\n")
	fmt.Printf("so it has no ordering at all, and the pooled rho was a rank by\n")
	fmt.Printf("position. The amended prediction: the fix alone moves points only\n")
	fmt.Printf("through the LEVEL it assigns, which is over-stated against the known\n")
	fmt.Printf("population by 1.5-1.9x outfield and 4.5x for goalkeepers, so expect\n")
	fmt.Printf("it to be flat-to-negative for THAT reason; and expect the fix+price\n")
	fmt.Printf("contrast to carry whatever positive there is, since price is the only\n")
	fmt.Printf("arm that orders these players at all.\n")
	fmt.Printf("⚠️ Read HOLD at --scale=per_gw, and read the moves column first.\n")

	arms := []policyVariant{
		{label: "baseline: unknowns read zero minutes",
			apply: func(sc *SimConfig) {
				sc.Weights.UnknownPriorShare = 0
				sc.Weights.PriceMinutesPrior = 0
			}},
		{label: "prior fix: league average, unordered",
			apply: func(sc *SimConfig) {
				sc.Weights.UnknownPriorShare = 1
				sc.Weights.PriceMinutesPrior = 0
			}},
	}
	for _, w := range []float64{0.25, 0.5} {
		tilt := w
		arms = append(arms, policyVariant{
			label: fmt.Sprintf("fix + price tilt %.2f", tilt),
			apply: func(sc *SimConfig) {
				sc.Weights.UnknownPriorShare = 1
				sc.Weights.PriceMinutesPrior = tilt
			},
		})
	}
	runPolicySweep(t, arms, starts)

	fmt.Printf("\n⚠️ The contrast that answers the question is (fix + price) MINUS (fix\n")
	fmt.Printf("alone), within a cell. Reading either against the baseline conflates\n")
	fmt.Printf("giving unknowns a prior at all with ordering them once they have one.\n")
	fmt.Printf("⚠️ Two tilt settings, so the answer can be a SHAPE rather than one\n")
	fmt.Printf("value. Taking whichever scores highest is an argmax over correlated\n")
	fmt.Printf("estimates and is not a result.\n")
	fmt.Printf("⚠️ Absolute totals here are NOT comparable with anything recorded\n")
	fmt.Printf("before this change: the baseline arm is the old behaviour, and the\n")
	fmt.Printf("other three are a different scoring era. Paired differences survive.\n")
}
