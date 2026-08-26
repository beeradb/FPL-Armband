package backtest

import (
	"fmt"
	"os"
	"testing"
)

// Does building the squad toward a chip pay, once the objective can say so?
//
//	DIAG=1 EXP=CHIPPREP FPL_CELLS=/tmp/chipprep/cells.csv \
//	    go test ./internal/backtest -run '^TestDiagChipPreparation$' -count=1 -v -timeout 4h
//
// # What this measures that TestDiagAnchoredChips could not
//
// That sweep varies *placement* — which week each chip is played in — and found
// about +10 a season for anchoring on the calendar, flat across two, four, six and
// full gameweeks of sight. The other half of how the chips are actually played is
// preparation: holding fifteen playable footballers into the boost week instead of
// eleven and four bodies, and owning the right premium in the tripled week.
//
// **It was blocked by expression, not by knowledge**, which is why it is worth
// running. The pool of candidate doublers is knowable weeks ahead from the public
// fixture list, so a policy could act on it — but `XIValue` took a squad and
// nothing else, so a bench worth boosting was priced at zero however much the
// model knew, and the triple captain had no expression anywhere. `SuggestBenchWeight`
// computed the right number on the opening-squad path and `SimConfig.anticipate`
// threw it away, because the transfer path had nowhere to put it.
//
// That also explains a null already on the record: the coherent `AnticipateChips`
// arm measured +2.5 a season and its own commit says it "cannot carry the bench
// boost". It tested the two levers that were wired and was silent on the two that
// were not, so flat is what it had to return. It is not evidence against
// preparation.
//
// # The design
//
// Every arm plays the same chips in the same weeks — `anchoredPlan`, which places
// the boost on the biggest remaining double and plays no wildcard — so the
// difference is attributable to preparation rather than to placement. That is the
// matched-arm design `TestDiagAnchoredChips` already established, reused rather
// than restated: a second copy of the placement rule here is the drift this
// package keeps paying for.
//
// **No arm plays a wildcard**, for the reason recorded there. Pinning the wildcard
// to a common week put the boost immediately after the rebuild in 30 of 30 cells
// for one arm and 3-5 of 30 for the others, which is a bigger confound than the one
// it removed — and it is a *worse* confound here than there, because a
// wildcard-rebuilt squad is the one case where the bench is already prepared, so
// the arm being tested would have had its own effect installed in the baseline.
//
// # Read the boost week before the season
//
// The season columns are the question, and they are the noisy reading: this record
// puts a real transfer-policy effect at 39 points a season against a POLICY median
// threshold near 70. The chip week's own realised gain is the sharp one — it is
// the mechanism, it is measured on the week the preparation was for, and it is
// printed below the grid. A preparation arm that does not move the chip week has
// not worked at all, whatever the season total says; one that moves the chip week
// and not the season has worked and been paid for elsewhere, which is the fourth
// arrival of "a better predictor can make a worse policy" and the outcome this
// record would bet on.
//
// # Pre-registered
//
//   - Direction, bench: the chip week's own gain goes **up**. This is close to
//     mechanical — the credit buys bench quality and the chip pays for bench
//     quality — so a null here means the wiring is inert, not that preparation
//     fails. It is the liveness check.
//   - Direction, captain: the tripled week's gain goes **up**, by less, because the
//     policy already buys the best players it can afford and the armband is
//     already worth double every week.
//   - Season, both: **unresolved is the expected reading.** Transfers are scarce
//     and the record's standing finding is that transfer volume barely moves a
//     season.
//   - HOLD must be **byte-identical in every arm**. The credit lives on
//     `analysis.SquadState`, which only the transfer searches read, so the opening
//     fifteen cannot see it. HOLD MOVED in this grid means the credit has leaked
//     into squad construction and nothing else in the table is readable.
//
// # What it is not
//
// The placement rule shared by every arm is full sight, which this record calls a
// target rather than a policy. Preparation measured against the best available
// placement is therefore an **upper bound on preparation under a realistic
// placement** — the boost sits on the biggest double, which is the most a prepared
// bench can be worth. It is not hindsight *about the arm*: both sides get the same
// placement, and the credit reads only `cfg.Chips`, which the planner already
// resolved.
//
// `AnticipateChips` is off in every arm, so this is one lever rather than two. The
// combination is a further arm and deliberately not this one.
func TestDiagChipPreparation(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	starts := sweepStarts()

	fmt.Printf("\n=== does building the squad TOWARD a chip pay?\n")
	fmt.Printf("Every arm plays the SAME chips in the SAME weeks (anchored placement,\n")
	fmt.Printf("no wildcard). What varies is whether the weekly transfer decision\n")
	fmt.Printf("knows the chip is coming and can value a squad accordingly.\n")
	fmt.Printf("The bench boost and the triple captain are SEPARATE arms: one buys at\n")
	fmt.Printf("the cheap end of the squad and the other at the expensive end, so a\n")
	fmt.Printf("combined arm would measure their sum and neither.\n")
	fmt.Printf("**HOLD must not move.** The credit is on the transfer path only.\n")
	fmt.Printf("**Read the chip-week table under the grid first** — it is the mechanism,\n")
	fmt.Printf("and the season columns are this harness's noisiest reading.\n\n")

	type chipWeek struct {
		boostGain, boostCells int
		tcGain, tcCells       int
	}
	seen := make([]chipWeek, 4)

	// One observer per arm, writing into that arm's slot. What the chip actually
	// returned in the week it was played, from the squad the policy actually held.
	watch := func(i int) func(seasonPair, int, *SimResult) {
		return func(_ seasonPair, _ int, res *SimResult) {
			for _, w := range res.Weeks {
				if w.BenchBoost {
					seen[i].boostGain += w.BenchBoostGain
					seen[i].boostCells++
				}
				if w.TripleCaptain {
					seen[i].tcGain += w.TripleCaptainGain
					seen[i].tcCells++
				}
			}
		}
	}

	arms := []policyVariant{
		{
			label:   "no preparation",
			apply:   func(sc *SimConfig) { sc.ChipPlanner = anchoredPlan },
			observe: watch(0),
		},
		{
			label: "build toward the bench boost",
			apply: func(sc *SimConfig) {
				sc.ChipPlanner = anchoredPlan
				sc.PrepareBenchBoost = true
			},
			observe: watch(1),
		},
		{
			label: "build toward the triple captain",
			apply: func(sc *SimConfig) {
				sc.ChipPlanner = anchoredPlan
				sc.PrepareTripleCaptain = true
			},
			observe: watch(2),
		},
		{
			label: "build toward both",
			apply: func(sc *SimConfig) {
				sc.ChipPlanner = anchoredPlan
				sc.PrepareBenchBoost = true
				sc.PrepareTripleCaptain = true
			},
			observe: watch(3),
		},
	}

	runPolicySweep(t, arms, starts)

	fmt.Printf("\n--- what the chip returned in the week it was played ---\n")
	fmt.Printf("Per cell that played it, so the arms are comparable even where a chip\n")
	fmt.Printf("is out of reach from a late entry. This is the mechanism check: an arm\n")
	fmt.Printf("that does not move its own chip's week has not worked.\n\n")
	fmt.Printf("%-32s %14s %14s\n", "arm", "bench boost", "triple captain")
	for i, a := range arms {
		mean := func(total, cells int) string {
			if cells == 0 {
				return "     -"
			}
			return fmt.Sprintf("%6.1f (%d)", float64(total)/float64(cells), cells)
		}
		fmt.Printf("%-32s %14s %14s\n", a.label,
			mean(seen[i].boostGain, seen[i].boostCells),
			mean(seen[i].tcGain, seen[i].tcCells))
	}
	fmt.Printf("\nThe bench-boost arm must move the bench-boost column and leave the\n")
	fmt.Printf("triple-captain one alone, and vice versa. A preparation arm that moves\n")
	fmt.Printf("the OTHER chip's column is reaching an axis it declared it cannot reach.\n")
}
