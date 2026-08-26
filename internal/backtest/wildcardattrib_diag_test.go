package backtest

// THE FIRST-HALF WILDCARD, WITH EVERY DIFFERENCE ATTRIBUTABLE
//
//	DIAG=1 FPL_CELLS=<path> go test ./internal/backtest \
//	    -run TestDiagWildcardAttribution -v -timeout 120m
//
// # Why this replaces two earlier sweeps rather than adding to them
//
// The wildcard rule was measured twice on 2026-08-26 and the two runs landed at
// DIFFERENT COMMITS — `3f25e9bc` and `1ec5f567` — with `changesInXI`, the
// per-set firing fix and `xiDriftSeries` in between. So the interesting
// comparison across them, drift-triggered against cost-triggered, carries three
// code changes as well as the rule, and `sweep_inference.R`'s own code-state
// guard refuses to difference the two files. Correctly: the same shipped arm
// reads `hits +0.58` in one and `-0.03` in the other, and nothing can say why.
//
// **This runs every arm in one process at one commit.** The comparisons are then
// arithmetic rather than argument.
//
// # What each arm isolates
//
//	control                 no trigger at all — every arm is read against this
//	cost, RAW, bar 12       the SHIPPED rule, exactly as it ships
//	cost, XI-only, bar 12   ...minus the raw count. THE INPUT, alone.
//	cost, XI-only, bar 10   ...minus the bar. THE BAR, alone, at the best rung
//	drift > 3.0             the RULE, alone: reads the gap, not the repair price
//	drift > 5.0             ...at the bar that fires rarely and reads sharply
//
// Each neighbouring pair differs in one thing. That is the property two earlier
// sweeps lacked and the reason their most interesting number could not be used.
//
// # ⚠️ What it still cannot do
//
// **Resolve.** Every arm measured so far sits far inside a threshold of 26-31 a
// season-path, and nothing here changes the instrument's power. Read the SIGNS
// and the MECHANISM columns — hits taken, weeks fired — not significance.
//
// **See the fixture run.** Every drift arm reads `xiDriftOf`, which prices on the
// shipped horizon where `FixtureLoadInScore` is false, so it cannot tell a double
// from a blank. `xiDriftSeries` fixes that and is **not wired to a trigger yet**;
// a fixture-aware arm is the next one to add, not one of these.
//
// **Price the value of waiting.** Injuries revealed, rotations observed. Nothing
// models it and the replay is as blind to it as the rules are.

import (
	"fmt"
	"os"
	"testing"
)

func TestDiagWildcardAttribution(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	starts := sweepStarts()

	fmt.Printf("\n=== the first-half wildcard, every difference attributable.\n")
	fmt.Printf("All arms at ONE commit, confined to GW1-%d, each read against a\n", ChipResetGW-1)
	fmt.Printf("control that plays no trigger. Neighbouring arms differ in one\n")
	fmt.Printf("thing: the count, then the bar, then the rule. Metric: POLICY.\n")

	arms := []policyVariant{
		{label: "no wildcard trigger (control)",
			apply: func(sc *SimConfig) { sc.WildcardTrigger = false }},
		{label: "cost, RAW count, bar 12 — SHIPPED",
			apply: func(sc *SimConfig) {
				sc.WildcardTrigger, sc.WildcardTriggerFirstHalfOnly = true, true
				sc.WildcardReservation = 12
			}},
		{label: "cost, XI-only count, bar 12 — the INPUT alone",
			apply: func(sc *SimConfig) {
				sc.WildcardTrigger, sc.WildcardTriggerFirstHalfOnly = true, true
				sc.WildcardReservation, sc.RepairCountsXIOnly = 12, true
			}},
		{label: "cost, XI-only count, bar 10 — the BAR alone",
			apply: func(sc *SimConfig) {
				sc.WildcardTrigger, sc.WildcardTriggerFirstHalfOnly = true, true
				sc.WildcardReservation, sc.RepairCountsXIOnly = 10, true
			}},
		{label: "drift > 3.0 pts/gw — the RULE alone",
			apply: func(sc *SimConfig) {
				sc.WildcardTrigger, sc.WildcardTriggerFirstHalfOnly = true, true
				sc.WildcardDriftBar = 3.0
			}},
		{label: "drift > 5.0 pts/gw — the rule, fired rarely",
			apply: func(sc *SimConfig) {
				sc.WildcardTrigger, sc.WildcardTriggerFirstHalfOnly = true, true
				sc.WildcardDriftBar = 5.0
			}},
	}
	runPolicySweep(t, arms, starts)

	fmt.Printf("\n⚠️ Read hits and wc_trig_gw beside every arm. The claim worth\n")
	fmt.Printf("checking is MECHANICAL: a wildcard repairs the squad for free, so a\n")
	fmt.Printf("rule that fires it and leaves the policy taking MORE hits has fired\n")
	fmt.Printf("on a squad that did not need it. That reading rests on one run and\n")
	fmt.Printf("is what this sweep exists to confirm or kill.\n")
	fmt.Printf("⚠️ Read `--scale=per_path`. A bar that never fires is inert, not\n")
	fmt.Printf("neutral, and reads identically to the control.\n")
}
