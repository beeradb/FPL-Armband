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

// IS THE HARM IN THE DEAD RUBBER SPECIFICALLY?
//
//	DIAG=1 FPL_CELLS=/tmp/priortrim.csv scripts/replay \
//	    -run TestDiagPriorMinutesTrim -v -timeout 4h
//
// # The question, and why it is not the one already answered
//
// `TestDiagPriorMinutesRecency` measured weighting last season's closing weeks
// and found it loses MONOTONICALLY: every half-life worse than the flat prior,
// the loss shrinking toward zero as the half-life lengthens. The reading was
// that the summer resets roles, so May is a smaller sample rather than a
// privileged one.
//
// That test cannot separate a rival explanation, because it weights the most
// corrupted week most heavily. **GW38 is measurably rotated — about 32% of
// established regulars benched or rotated, against ~25% mid-season — while GW36
// and GW37 are NOT (21-26%, at or below the GW20-30 level).** So "recency is
// wrong" and "recency is right but the last week is poison" both predict the
// observed loss, and the trim is what tells them apart.
//
// # A 2x2, because the mechanism names the pair
//
// Half-life and trim act on the same weeks, so they are crossed rather than
// swept separately: a difference of differences within a cell cancels the path
// divergence a single difference carries. Two half-lives are enough — the
// sharpest, where GW38 carries the most weight and the recorded loss is
// largest, and the shipped-adjacent 4.
//
// # PRE-REGISTERED, before running
//
//   - **If the dead rubber is the problem**, trimming helps MORE at half-life 2
//     than at 4, because that is where GW38's weight is concentrated. The
//     interaction is the test, not either main effect.
//   - **If recency across the summer is simply wrong**, trimming moves little at
//     either half-life and both trimmed arms stay below the flat prior.
//   - ⚠️ **The prior evidence predicts the second.** The gradient toward flat is
//     monotone, so the trim must REVERSE a consistent trend rather than nudge a
//     null. And the half-life already anchors on each player's LAST APPEARANCE
//     rather than on GW38, so a player who stopped in March is anchored at
//     March and the dead rubber is already partly blunted. This is run because
//     unmeasured is worse than measured-and-lost, not because it is expected to
//     win.
//   - **What would make this uninterpretable:** the trimmed arms moving zero
//     players at GW1. Check the moves column before reading anything.
func TestDiagPriorMinutesTrim(t *testing.T) {
	requireDiag(t)
	starts := sweepStarts()

	fmt.Printf("\n=== IS THE HARM IN THE DEAD RUBBER, OR IN SUMMER RECENCY ITSELF?\n")
	fmt.Printf("Baseline is the shipped FLAT prior. The four arms cross two half-lives\n")
	fmt.Printf("with trimmed and untrimmed, so the INTERACTION is readable.\n")
	fmt.Printf("⚠️ Pre-registered: if GW38 is the problem, the trim helps MORE at\n")
	fmt.Printf("half-life 2 than at 4. If summer recency is simply wrong, it helps at\n")
	fmt.Printf("neither and both trimmed arms stay below flat.\n")
	fmt.Printf("⚠️ Read HOLD at --scale=per_gw. HOLD is the opening fifteen with no\n")
	fmt.Printf("transfer policy in the way, which is what a prior builds.\n")
	fmt.Printf("⚠️ The trim keeps the gradient: GW37 still outweighs GW1. It removes the\n")
	fmt.Printf("dead rubber, it does not flatten the prior — that is a different arm and\n")
	fmt.Printf("it is the baseline.\n")

	arms := []policyVariant{
		{label: "flat prior (shipped)",
			apply: func(sc *SimConfig) { sc.PriorMinutesHalfLife = 0 }},
	}
	for _, hl := range []float64{2, 4} {
		for _, trim := range []int{0, 37} {
			h, tr := hl, trim
			label := fmt.Sprintf("half-life %.0f, untrimmed", h)
			if tr > 0 {
				label = fmt.Sprintf("half-life %.0f, trimmed after GW%d", h, tr)
			}
			arms = append(arms, policyVariant{
				label: label,
				apply: func(sc *SimConfig) {
					sc.PriorMinutesHalfLife = h
					sc.PriorTrimAfterGW = tr
				},
			})
		}
	}
	runPolicySweep(t, arms, starts)

	fmt.Printf("\n⚠️ The contrast that answers the question is the DIFFERENCE OF\n")
	fmt.Printf("DIFFERENCES: (hl2 trimmed - hl2 untrimmed) against (hl4 trimmed - hl4\n")
	fmt.Printf("untrimmed). Reading either trimmed arm against flat on its own conflates\n")
	fmt.Printf("the trim with the half-life it was applied to.\n")
	fmt.Printf("⚠️ Four arms against one baseline: correct for multiplicity. A single\n")
	fmt.Printf("arm clearing its own bar out of four is what Holm exists for.\n")
}
