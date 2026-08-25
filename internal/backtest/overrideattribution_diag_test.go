package backtest

// Attributing the override-mode effect: what the FIELDING and the PLAN each buy.
//
//	DIAG=1 EXP=OVRX FPL_SWEEP_SEASONS=extended FPL_SWEEP_STARTS=1,16 \
//	FPL_CELLS=/tmp/ovrx.csv \
//	  scripts/replay -run TestDiagOverrideModeAttribution -v -timeout 3h
//
// # The question
//
// The override configuration resolved at +73.0 a season, and the decomposition
// put the chip WEEK payouts at only ~16 of it. The other ~30 is some mixture of
// the free hit, the transfers made anticipating the plan, and `WeeklyXI`
// fielding the imminent eleven every week. Nobody knows which buys what — and
// the answer changes what to build next. If the fielding alone is most of it,
// the interesting knob is neither the prior nor the chips' placement.
//
// # The design — a 2x2 on the resolution's own grid, all contrasts registered
//
// Factor A, the PLAN: off (shipped) against on, where on is the same override
// mode the resolution used — `anchoredPlan` SET by the analysis layer rather
// than chosen by an optimiser, plus `AnticipateChips` so the transfer decision
// knows the plan, plus `BankLookahead` so banking can act on it.
//
// Factor B, the FIELDING: `WeeklyXI` off (shipped) against on.
//
// Four arms, one grid: extended six seasons x entry GW1/GW16 = 12 cells per
// arm, 48 cells, POLICY. The shipped corner (plan off, fielding off) is the
// baseline, and the both-on corner reproduces the +73 configuration.
//
// Registered contrasts — three, fixed before the first cell:
//
//	FIELDING    (fielding on, plan off) - shipped. Direction NOT pre-registered.
//	              WeeklyXI has never been scored alone on this grid. The note
//	              that raised this asks precisely whether it carries the effect.
//	PLAN        (plan on, fielding off) - shipped. Predicted >= 0: the chip
//	              weeks pay something even misfielded, and the decomposition
//	              already banks ~16 a season of week payouts.
//	AxB         (both - plan-only) - (fielding-only - shipped). Predicted >= 0,
//	              and this is the mechanism claim: a bench boost on a week whose
//	              eleven was chosen for a DIFFERENT gameweek is worth less, so
//	              the plan should pay more once the weeks are fielded properly.
//	              A reversal is information, not a failure.
//
// Multiplicity: Holm over the three. Each contrast takes its own threshold from
// stats/variance_components.R on the banked cells; a contrast that does not
// clear its own t_crit is not a result whatever the p says.
//
// # Registered limitations, so the verdicts cannot over-read
//
//   - **The free hit is NOT separated.** It rides inside the plan factor,
//     because it is part of `anchoredPlan`. Splitting it needs a third factor
//     (plan with/without the free hit) and is worth running only if PLAN
//     resolves AND the banked payout columns say the BB/TC weeks do not account
//     for it. Registered here so the follow-up is not mistaken for this arm.
//   - **`anchoredPlan` is full sight.** It knows every double and blank from
//     entry, so PLAN and AxB are UPPER BOUNDS on what a real manager's plan
//     buys; imperfect sight costs some of it.
//   - **AnticipateChips and BankLookahead move WITH the plan.** The PLAN factor
//     is therefore "the plan plus the machine that acts on it", not the chip
//     schedule alone. BankLookahead is recorded inert at shipped config, which
//     narrows but does not eliminate this.
//   - **This does not decompose the +73.** The resolution ran a different
//     configuration on a different question; these four arms share its grid but
//     the contrasts are between THESE arms. No figure here may be subtracted
//     from 73.0.
//
// # The live-cell moderator, registered before the run
//
// Same criterion as the prior-reactivity 2x2, and for the same reason: a cell
// can only show a plan effect if its window contains something for a plan to
// act on. A cell is live when its window from entry holds at least one double
// or blank AND the plan's first chip does not land at entry+1, where it would
// replace a squad just drafted near-optimal. Both halves are reported; the
// prediction is that PLAN and AxB concentrate in the live half while FIELDING
// does not — fielding the imminent eleven pays every week, not only chip weeks.
//
// Moderator floor: if the live count within one entry-point half falls below 4
// of 6 seasons, that half is reported as "insufficient data", not as a finding.

import (
	"fmt"
	"os"
	"testing"
)

func TestDiagOverrideModeAttribution(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)
	pairs := loadPairs(t, cfg)
	starts := sweepStarts()

	fmt.Printf("\n=== override-mode attribution: fielding vs plan, a registered 2x2\n")
	fmt.Printf("%s\n", gridLabel(len(pairs), len(starts)))
	fmt.Printf("FPL_SWEEP_SEASONS=%s  FPL_SWEEP_STARTS=%s  entry points %v\n",
		gridEnv("FPL_SWEEP_SEASONS", "extended (the shipped default)"),
		gridEnv("FPL_SWEEP_STARTS", "1,6,11,16,21,26 (the shipped default)"), starts)
	fmt.Printf("Arms: plan off/on x fielding off/on. PLAN ON = anchoredPlan\n")
	fmt.Printf("(override mode, set not optimised) + AnticipateChips + BankLookahead.\n")
	fmt.Printf("FIELDING ON = WeeklyXI. Baseline is the shipped corner, both off.\n\n")

	// The live split, printed before any cell runs: calendar and plan are
	// deterministic in (season, entry) and upstream of every result.
	fmt.Printf("%-9s %-5s %-4s %-4s %-6s %-6s %-5s %s\n",
		"season", "start", "WC", "BB", "FH", "TC", "live", "reason")
	live := 0
	liveByStart := map[int]int{}
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
				liveByStart[start]++
			}
			fmt.Printf("%-9s GW%-4d %3d %3d %3d %3d  %-5v %s\n",
				p.Name, start, plan.Wildcard, plan.BenchBoost, plan.FreeHit,
				plan.TripleCaptain, isLive, reason)
		}
	}
	fmt.Printf("\n%d of %d cells live. Registered prediction: PLAN and AxB\n",
		live, len(pairs)*len(starts))
	fmt.Printf("concentrate in the live half; FIELDING does not, because fielding\n")
	fmt.Printf("the imminent eleven pays in ordinary weeks too.\n")
	for _, start := range starts {
		n := liveByStart[start]
		verdict := fmt.Sprintf("%d of %d live", n, len(pairs))
		if n < 4 {
			verdict += " — INSUFFICIENT DATA for this half (floor is 4)"
		}
		fmt.Printf("  entry GW%-3d %s\n", start, verdict)
	}
	fmt.Println()

	// planOn is the override mode exactly as the resolution ran it, minus the
	// fielding — which is factor B and must not be smuggled in here.
	planOn := func(sc *SimConfig) {
		sc.ChipPlanner = anchoredPlan
		sc.AnticipateChips = true
		sc.BankLookahead = true
	}
	fieldingOn := func(sc *SimConfig) {
		sc.WeeklyXI = true
	}
	arms := []policyVariant{
		{label: "shipped (plan off, fielding off)", apply: func(sc *SimConfig) {}},
		{label: "fielding only", apply: fieldingOn},
		{label: "plan only", apply: planOn},
		{label: "plan + fielding", apply: func(sc *SimConfig) {
			planOn(sc)
			fieldingOn(sc)
		}},
	}
	runPolicySweep(t, arms, starts)
}
