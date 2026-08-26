package backtest

// Does triggering the wildcard on XI DRIFT beat triggering it on repair cost?
//
//	DIAG=1 FPL_CELLS=<path> go test ./internal/backtest \
//	    -run TestDiagWildcardDriftTrigger -v -timeout 90m
//
// # The two rules, and why they are not two units for one rule
//
// The shipped trigger fires when repairing the squad by transfers would cost
// more in hits than a decayed option bar: `4 x max(0, changes - free)` against
// `analysis.ChipBarAt`. **`changes` is a plain count over all fifteen** —
// `changesBetween` — so a £4.0m bench swap scores exactly like losing a captain.
//
// The user's objection is the whole reason `xidrift.go` exists: *"when measuring
// drift we should only do it in the starting 11 … weighted by xpoints, not number
// of changes. Switching a benched player is basically never worth a transfer."*
//
// Measured on the opening squads, the two readings correlate **0.676** — so the
// ranking they induce differs materially, and eleven cells reading "7 changes"
// span 0.79 to 4.46 points of actual XI cost.
//
// # ⚠️ What this comparison canNOT settle, stated before the arms
//
// **The drift arm loses the option-value decay**, because `ChipBarAt` prices a
// one-off hit cost and drift is a per-gameweek rate — see `WildcardDriftBar`. So
// a difference between the arms is a difference between TWO RULES, one of which
// also stopped waiting for a better week. **It is not a clean read on the
// measure.** Isolating that needs a decayed bar fitted to drift, which does not
// exist and which nothing here provides.
//
// ⚠️ **The bars are not comparable across arms and must not be read as one
// ladder.** The cost arm's reservation is in hit points; the drift bars below are
// in expected points per gameweek on the eleven. They are swept because that is
// how this project locates a knob, not because 2.0 here means what 2.0 means
// there.

import (
	"fmt"
	"os"
	"testing"
)

// wildcardDriftBars brackets the measured drift distribution rather than
// guessing: TestDiagXIDrift reads a mean of 3.98 points on the XI five gameweeks
// after entry, with cells from 0.35 to 11.46. A bar above the top of that range
// never fires and a bar below the bottom fires in week one, so the ladder spans
// the middle where the rule can actually discriminate.
var wildcardDriftBars = []float64{1.5, 3.0, 5.0, 8.0}

func TestDiagWildcardDriftTrigger(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	starts := sweepStarts()

	fmt.Printf("\n=== wildcard trigger: XI DRIFT against REPAIR COST. Metric: POLICY.\n")
	fmt.Printf("Control plays no wildcard trigger at all, so each arm is read\n")
	fmt.Printf("against not having the rule rather than against the other rule.\n")
	fmt.Printf("⚠️ The drift arms carry NO option-value decay — ChipBarAt prices a\n")
	fmt.Printf("one-off hit cost and drift is a per-gameweek rate. A difference is\n")
	fmt.Printf("between two RULES, not between two readings of one rule.\n")

	arms := []policyVariant{
		{label: "no wildcard trigger (control)",
			apply: func(sc *SimConfig) { sc.WildcardTrigger = false }},
		{label: "trigger on repair cost (shipped rule)",
			apply: func(sc *SimConfig) { sc.WildcardTrigger = true }},
	}
	for _, b := range wildcardDriftBars {
		bar := b
		arms = append(arms, policyVariant{
			label: fmt.Sprintf("trigger on XI drift > %.1f pts/gw", bar),
			apply: func(sc *SimConfig) {
				sc.WildcardTrigger = true
				sc.WildcardDriftBar = bar
			},
		})
	}
	runPolicySweep(t, arms, starts)

	fmt.Printf("\nRead the SHAPE across the drift ladder, not any single arm: a\n")
	fmt.Printf("plateau with a cliff is what this project accepts as evidence for a\n")
	fmt.Printf("knob, and a single arm clearing a threshold is not.\n")
	fmt.Printf("⚠️ Read with `--scale=per_path`: a chip is an event count.\n")
	fmt.Printf("⚠️ The wc_trig_* columns say how often each rule FIRED. An arm that\n")
	fmt.Printf("never fired is inert, not neutral, and reads identically to the\n")
	fmt.Printf("control — check the fire count before reading a null.\n")
}
