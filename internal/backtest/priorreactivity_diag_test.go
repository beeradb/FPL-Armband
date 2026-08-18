package backtest

// Prior reactivity under the exit levers: the 2x2 the user's hypothesis asks
// for.
//
//	DIAG=1 EXP=PRIORX FPL_SWEEP_SEASONS=extended FPL_SWEEP_STARTS=1,16 \
//	FPL_CELLS=/tmp/priorx.csv \
//	  scripts/replay -run TestDiagPriorReactivityUnderExitLevers -v -timeout 3h
//
// # The question
//
// The repair-cost series established that a held squad sits a flat 6-9 players
// below the model's fresh optimum, and the record's flat prior ships at k=8
// with the unresolved direction UPWARD — heavier prior, slower reaction to new
// data. Every earlier measurement of a heavier prior ran on a machine with
// nothing to hold a transfer FOR: no banking, no chips, no preparation. The
// user's reading, 2026-08-17: "We had nothing to bank for, so no reason to
// wait, and we didn't even have the concept of chips then." This arm re-judges
// prior reactivity under the chip-and-banking configuration.
//
// # The design — two factors, one cell grid, all contrasts pre-registered
//
// Factor A, prior reactivity: BlendRateK 8 (shipped) against 24 (the heaviest
// rung of the banked ladder). Factor B, exit levers: OFF (shipped) against ON,
// where ON is the OVERRIDE mode the user directed — a chip plan SET by the
// analysis layer, not chosen by an optimiser: `anchoredPlan` (the calendar
// anchor on the known doubles and blanks, full sight), `AnticipateChips` so
// the transfer decision knows the plan, and `BankLookahead` so banking can
// act. Four arms, one grid: extended six seasons x entry GW1/GW16 = 12 cells
// per arm, 48 cells, POLICY.
//
// Registered contrasts (the full 2x2 — three, not a search over seven):
//
//	A main      (k24 - k8), averaged over levers off and on. Predicted <= 0:
//	              every heavier rung read negative on POLICY on the old machine.
//	B main      (on - off), averaged over k=8 and k=24. Direction not
//	              pre-registered — the levers have never been scored together.
//	AxB         (k24-k8)|on  -  (k24-k8)|off. Predicted >= 0 from the user's
//	              mechanism: a slower reaction is worth relatively MORE once a
//	              held squad has an exit to unwind into and a bank to wait with.
//	              A reversal is information, not a failure.
//
// Multiplicity: Holm over the three contrasts. Each gets its own threshold
// from stats/variance_components.R on the banked cells.
//
// # The live-cell moderator, registered before the run
//
// Per the user's criterion: a cell can improve only if its window from entry
// contains at least one double or blank AND the plan's first chip does not
// land immediately after entry (entry+1), where it would replace a squad that
// was just drafted near-optimal and has nothing to buy. The split is computed
// from the fixture calendar and the plan — both upstream of any result — and
// BOTH halves are reported. Prediction: the A main effect and the AxB
// interaction concentrate in the live half; the inert half is the control the
// design needs. ⚠️ GW1 entries are the weakest case for "near-optimal at
// entry" — the opening fifteen is built on the season's weakest information —
// so the near-optimality criterion is entry-dependent, stated here rather than
// discovered later.
//
// # Liveness, stated so a null cannot hide a confinement
//
// Every levers-on cell must PLAY at least one chip for the ON corner to be a
// comparison rather than a confinement; the plan is printed per cell below and
// the chip columns in the banked CSV are the check. banked_weeks is printed
// from the same CSV in the writeup: banking is expected to stay inert at
// shipped MaxHits (the 2->3+ boundary does nothing) and an inert mediator is
// reported as the branch never executing, never as a null.

import (
	"fmt"
	"os"
	"testing"

	"armband/internal/analysis"
)

func TestDiagPriorReactivityUnderExitLevers(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)
	pairs := loadPairs(t, cfg)
	starts := sweepStarts()

	fmt.Printf("\n=== prior reactivity under the exit levers: a registered 2x2\n")
	fmt.Printf("%s\n", gridLabel(len(pairs), len(starts)))
	fmt.Printf("FPL_SWEEP_SEASONS=%s  FPL_SWEEP_STARTS=%s  entry points %v\n",
		gridEnv("FPL_SWEEP_SEASONS", "extended (the shipped default)"),
		gridEnv("FPL_SWEEP_STARTS", "1,6,11,16,21,26 (the shipped default)"), starts)
	fmt.Printf("Arms: k=8/24 x levers off/on. ON = anchoredPlan chips (override\n")
	fmt.Printf("mode, set not optimised) + AnticipateChips + BankLookahead.\n\n")

	// The live-cell split, printed before any cell runs: calendar and plan are
	// deterministic in (season, entry), upstream of every result.
	fmt.Printf("%-9s %-5s %-4s %-4s %-6s %-6s %-5s %s\n",
		"season", "start", "WC", "BB", "FH", "TC", "live", "reason")
	live := 0
	for _, p := range pairs {
		for _, start := range starts {
			plan := anchoredPlan(p.Cur, start)
			census := censusOf(p.Cur)
			doubles, blanks := 0, 0
			for _, w := range census {
				if w.gw < start || !w.played {
					continue
				}
				if w.doubling > 0 {
					doubles++
				}
				if w.blanking > 0 {
					blanks++
				}
			}
			first := firstChipWeek(plan)
			isLive := (doubles > 0 || blanks > 0) && (first == 0 || first >= start+2)
			reason := "no double or blank in window"
			if doubles > 0 || blanks > 0 {
				if first == start+1 {
					reason = "first chip immediate (entry+1)"
				} else {
					reason = fmt.Sprintf("%dd %db, first chip GW%d", doubles, blanks, first)
				}
			}
			if isLive {
				live++
			}
			fmt.Printf("%-9s GW%-4d %3d %3d %3d %3d  %-5v %s\n",
				p.Name, start, plan.Wildcard, plan.BenchBoost, plan.FreeHit,
				plan.TripleCaptain, isLive, reason)
		}
	}
	fmt.Printf("\n%d of %d cells live. Both halves are reported; the prediction is\n",
		live, len(pairs)*len(starts))
	fmt.Printf("that the k effect and the interaction concentrate in the live half.\n\n")

	leversOn := func(sc *SimConfig) {
		sc.ChipPlanner = anchoredPlan
		sc.AnticipateChips = true
		sc.BankLookahead = true
	}
	arms := []policyVariant{
		{label: "k8, levers off", apply: func(sc *SimConfig) {}},
		{label: "k24, levers off", apply: func(sc *SimConfig) {
			sc.Weights.BlendRateK = 24
		}},
		{label: "k8, levers on", apply: leversOn},
		{label: "k24, levers on", apply: func(sc *SimConfig) {
			leversOn(sc)
			sc.Weights.BlendRateK = 24
		}},
	}
	runPolicySweep(t, arms, starts)
}

// firstChipWeek is the earliest gameweek any chip in the plan fires, 0 for a
// chipless plan. The live-cell criterion's "not immediate" half reads it.
func firstChipWeek(plan analysis.ChipPlan) int {
	first := 0
	for _, gw := range []int{plan.Wildcard, plan.BenchBoost, plan.FreeHit, plan.TripleCaptain} {
		if gw > 0 && (first == 0 || gw < first) {
			first = gw
		}
	}
	return first
}
