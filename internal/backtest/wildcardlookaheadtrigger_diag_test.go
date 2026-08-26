package backtest

import (
	"fmt"
	"os"
	"testing"
)

// wildcardLookaheadBars brackets the MEASURED distribution of the reading rather
// than guessing from the neighbouring bars, which are in different units.
//
// `TestDiagWildcardLookaheadValue` reads, over 36 cells at entry+5 with one free
// transfer: min 24.89, median 63.54, p90 101.04, max 110.14, mean 68.87. A bar
// above the max never fires and one below the min fires in week one; both read as
// an inert rule and neither is a result. This ladder spans the middle, where the
// rule can discriminate.
//
// ⚠️ Those readings are taken at ONE point five gameweeks past entry with one
// free transfer. The live rule reads every eligible week with a varying
// allowance, so the in-sim distribution is wider on both sides — the ladder
// brackets the shape, not the exact quantiles.
var wildcardLookaheadBars = []float64{40, 55, 70, 85, 100}

// DOES PRICING THE WILDCARD ACROSS A RUN OF GAMEWEEKS BEAT READING ONE?
//
//	DIAG=1 FPL_CELLS=<path> go test ./internal/backtest \
//	    -run TestDiagWildcardLookaheadTrigger -v -count=1 -timeout 90m
//
// Three readings of one decision, against a control that plays no wildcard
// trigger at all, so each arm reads against NOT HAVING the rule rather than
// against the other rules:
//
//  1. **the shipped cost rule** — `4 x max(0, changes - free)` against a bar of
//     12, a one-off hit price over a plain count of all fifteen;
//  2. **the single-week drift rule** — points on the eleven, ⚠️ read on the
//     horizon-5 engine where `FixtureLoadInScore()` is FALSE, so despite looking
//     like the fixture-aware measure it is a five-week average blind to the
//     doubles and blanks inside its own window;
//  3. **the lookahead rule** — `xiDriftSeries` re-reading the same two elevens
//     one gameweek at a time on a horizon-1 engine, where fixture load IS in the
//     score, summed and added to the hit cost repairing would take now.
//
// # ⚠️ Two things to read before the table
//
// **The third reading correlates 0.884 with the second** on the bracketing cells.
// It is not a different ranking of weeks; it is a similar ranking in different
// units. So a difference between arms 2 and 3 is a difference in WHERE THE BAR
// BITES at least as much as in what is being measured, and a null here is the
// likelier outcome. Say so rather than reporting a null as "fixture awareness
// does not help".
//
// **This is NOT the peak rule this line of work set out to build.** That rule
// would fire only when now is the best week to play. It cannot be built on
// `wildcardValueOverNext`: its value is non-increasing in k by construction, so
// `PeakAt` is identically zero and the gate would never refuse anything —
// confirmed at 0 of 36 cells on real data. Waiting can only pay if the squad you
// would rebuild INTO improves while you wait, which needs `Optimize` re-run at
// every lookahead week. See TestWildcardValueOverNextPricesTheLookahead.
func TestDiagWildcardLookaheadTrigger(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	starts := sweepStarts()

	fmt.Printf("\n=== the wildcard priced across a RUN of gameweeks, against one week\n")
	fmt.Printf("Confined to GW1-%d, where there is no double to anchor to and the\n", ChipResetGW-1)
	fmt.Printf("rule is a condition on the SQUAD rather than on the calendar.\n")
	fmt.Printf("Every arm sets its bar EXPLICITLY: sweepConfig does not map\n")
	fmt.Printf("config.OptionValue into SimConfig, so an arm that omits it runs at a\n")
	fmt.Printf("bar of zero and fires on anything positive. Metric: POLICY.\n")
	fmt.Printf("⚠️ Read `--scale=per_path`: a chip is an event count.\n")
	fmt.Printf("⚠️ The wc_trig_* columns say how often each rule FIRED. An arm that\n")
	fmt.Printf("never fired is inert, not neutral, and reads identically to the\n")
	fmt.Printf("control — check the fire count before reading a null.\n")

	arms := []policyVariant{
		{label: "no wildcard trigger (control)",
			apply: func(sc *SimConfig) { sc.WildcardTrigger = false }},
		{label: "cost, raw count, bar 12 (the shipped rule)",
			apply: func(sc *SimConfig) {
				sc.WildcardTrigger, sc.WildcardTriggerFirstHalfOnly = true, true
				sc.WildcardReservation = 12
			}},
		{label: "single-week drift > 3.0 (horizon-5, NOT fixture-aware)",
			apply: func(sc *SimConfig) {
				sc.WildcardTrigger, sc.WildcardTriggerFirstHalfOnly = true, true
				sc.WildcardDriftBar = 3.0
			}},
	}
	for _, b := range wildcardLookaheadBars {
		bar := b
		arms = append(arms, policyVariant{
			label: fmt.Sprintf("lookahead value > %.0f (horizon-1, fixture-aware)", bar),
			apply: func(sc *SimConfig) {
				sc.WildcardTrigger, sc.WildcardTriggerFirstHalfOnly = true, true
				sc.WildcardLookaheadBar = bar
			},
		})
	}
	runPolicySweep(t, arms, starts)

	fmt.Printf("\nRead the SHAPE across the ladder, not any single arm: a plateau with\n")
	fmt.Printf("a cliff is what this project accepts as evidence for a knob, and one\n")
	fmt.Printf("arm clearing a threshold is not.\n")
}
