package backtest

import (
	"fmt"
	"testing"
)

// DOES WEIGHTING LAST SEASON'S CLOSING MINUTES BUILD A BETTER OPENING SQUAD?
//
//	DIAG=1 FPL_CELLS=<path> go test ./internal/backtest \
//	    -run TestDiagPriorMinutesRecency -v -count=1 -timeout 3h
//
// # Why this lever and why now
//
// The drift trajectory measured something the record had not: **the opening
// fifteen is uniquely bad.** It carries roughly 5-7 points a week of gap against
// the point-in-time optimum, where any IN-SEASON rebuild carries about 2.5. It is
// the only squad built with no in-season data at all, so whatever is wrong with
// it is wrong with the prior.
//
// A probe then asked which shipped knob can even reach that squad. Almost none
// can: `league_shrink_k`, `blend_rate_k`, `blend_minutes_k`, `prior_half_life`,
// `rate_half_life` and `minutes_half_life` all move **zero** players at GW1 in
// all six seasons — they either govern in-season data that does not exist yet or
// blend across prior seasons the replay does not load. `PriorRateHalfLife` is
// inert too.
//
// ⚠️ **`PriorMinutesHalfLife` moves 4 to 11 of the 15, every season, and
// `AGENTS.md` carries no entry for it.** A large, live, never-measured lever
// aimed at the exact squad measured to be worst.
//
// # What it should fix, in the record's own words
//
// `newPriorIndexRecent` exists because the prior season is a flat total, so "a
// player who lost his place in March counts the same as one who won it, and a
// player who broke through in February is averaged down by the autumn he spent on
// the bench". The standing finding is that **minutes reward sharp recency because
// it removes a BIAS — a dropped player reading as an ever-present** — while rates
// punish it. This arm moves only the minutes half.
//
// # Metric: HOLD, and that is the point
//
// HOLD is the opening fifteen's realised points with no transfer policy in the
// way, so it isolates exactly what a prior builds. ⚠️ **Judged on realised
// points, never on prediction error.** `leagueshrink_test.go` records why in
// blood: recency on rates predicted better out of sample and LOST points at every
// setting, because the optimiser consumes an ordering and lives in the tail of
// the estimate distribution. A better predictor is not a better policy.
//
// ⚠️ **The half-life counts back from the last gameweek the player APPEARED in**,
// not from GW38, so a player who stopped playing in March is anchored at March.
// That already blunts the dead-rubber problem this might otherwise inherit —
// rotation among regulars measures ~32% at GW38 against ~25% mid-season, so GW38
// is genuinely noisier, but GW36-37 are not, and a trim was not built here.
func TestDiagPriorMinutesRecency(t *testing.T) {
	requireDiag(t)
	starts := sweepStarts()

	fmt.Printf("\n=== PRIOR MINUTES RECENCY: does the opening squad get better?\n")
	fmt.Printf("Baseline is the shipped FLAT prior — last season's total minutes, so a\n")
	fmt.Printf("player who lost his place in March counts the same as one who won it.\n")
	fmt.Printf("Arms weight last season's CLOSING weeks by the named half-life.\n")
	fmt.Printf("⚠️ Read HOLD, not POLICY: HOLD is the opening fifteen's realised points\n")
	fmt.Printf("with no transfer policy in the way, which is what a prior builds.\n")
	fmt.Printf("⚠️ This moves 4-11 of 15 players at GW1, so it is NOT inert — but check\n")
	fmt.Printf("the moves column anyway before reading any null.\n")

	arms := []policyVariant{
		{label: "flat prior (shipped)",
			apply: func(sc *SimConfig) { sc.PriorMinutesHalfLife = 0 }},
	}
	for _, hl := range []float64{2, 4, 8, 12} {
		h := hl
		arms = append(arms, policyVariant{
			label: fmt.Sprintf("prior minutes half-life %.0f", h),
			apply: func(sc *SimConfig) { sc.PriorMinutesHalfLife = h },
		})
	}
	runPolicySweep(t, arms, starts)

	fmt.Printf("\n⚠️ A gain here is a claim about the PRIOR, so it should be largest at\n")
	fmt.Printf("early entry points and fade as in-season data accumulates. If it is flat\n")
	fmt.Printf("across entry gameweeks, something other than the prior moved.\n")
	fmt.Printf("⚠️ `--scale=per_gw`: HOLD is a rate — the squad scores every week — not\n")
	fmt.Printf("an event count.\n")
}
