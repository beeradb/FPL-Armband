package backtest

// How big is a difference that means nothing?
//
// Every "inside the noise" verdict in AGENTS.md is calibrated against a jitter
// floor of about 150 points over four seasons, and that number was inherited
// from an era whose totals were roughly 10% lower. It has never been measured at
// the current scoring, and a single-variant baseline run cannot measure it: noise
// here is *sensitivity*, not randomness, so it only becomes visible when
// something is perturbed and the paired differences are collected.
//
//	DIAG=1 go test ./internal/backtest -run TestDiagNoiseFloor -v -timeout 3h
//
// The design is a set of arms that should all be near-null in truth. The
// optimiser returns a *discrete* squad, so a hair's-breadth score change flips
// one slot, that squad plays on for every remaining week, and the transfer path
// compounds it. A 1-2% nudge to a scoring constant is far too small to be a real
// model change and far too large to be invisible, which is exactly the property
// that makes it a ruler.
//
// The first arm is the important one and is not a nudge at all. "identical
// (control)" applies precisely what the baseline applies, so its paired
// difference must be exactly 0.000 with a standard error of exactly zero. If it
// is not, the replay is nondeterministic — map iteration order, concurrency,
// something reachable from Simulate — and every paired result in AGENTS.md is
// measuring that as well as whatever it meant to measure. That would be a far
// larger finding than the floor, so it is checked rather than assumed.
//
// Read the SE column, not the mean. The mean of a near-null arm is one draw from
// the noise; the standard error is the width of the thing being measured.
//
// ⚠️ The detection threshold is `t_crit(df) x SE x 38`, NOT `2 x SE x 38`, which
// this comment said until 2026-08-13. The critical value is 2.571 at the CR2 df
// of 5, so the doubling shortcut understates by 29% — take the figure from
// `stats/variance_components.R`, which prints both the p = 0.05 effect and the
// 80%-power MDE per arm, and take the df from the comparison rather than
// assuming 5: it is resolved per contrast and is often lower.

import (
	"testing"
)

func TestDiagNoiseFloor(t *testing.T) {
	requireDiag(t)

	// The shipped conventions, matching TestDiagBaseline exactly so the two runs
	// are comparable. Every arm below starts from this.
	base := func(sc *SimConfig) { sc.WeeklyXI = true }

	v := []policyVariant{
		{label: "shipped (baseline)", apply: base},
		// The control. Must difference to exactly zero.
		{label: "identical (control)", apply: base},
		// Near-nulls. MinutesWeight is the exponent AGENTS.md already documents
		// as moving four-season points by 67 on a 2% nudge, so it is the arm
		// with a prior attached; the other two check that the floor is a
		// property of the harness rather than of one constant.
		{label: "minutes_weight +2%", apply: func(sc *SimConfig) {
			base(sc)
			sc.Weights.MinutesWeight = 1.275
		}},
		{label: "minutes_weight -2%", apply: func(sc *SimConfig) {
			base(sc)
			sc.Weights.MinutesWeight = 1.225
		}},
		{label: "fixture_weight +1.5%", apply: func(sc *SimConfig) {
			base(sc)
			sc.Weights.FixtureWeight = 0.66
		}},
		{label: "bonus_weight +0.7%", apply: func(sc *SimConfig) {
			base(sc)
			sc.Weights.BonusWeight = 1.51
		}},
	}

	runPolicySweep(t, v, sweepStarts())
}
