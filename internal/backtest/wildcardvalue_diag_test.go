package backtest

// What is a wildcard actually worth: playing one against playing none.
//
//	DIAG=1 FPL_CELLS=<path> go test ./internal/backtest -run TestDiagWildcardValue -v -timeout 45m
//
// # The question, and why it is not the one already measured
//
// `TestDiagAnchoredChipDecomposition`'s wildcard block compares a wildcard at a
// fixed offset against a wildcard on a double: both arms play the chip, so its
// VALUE cancels and only the TIMING is measured (+6.4 a season-path against a
// threshold of 27.0, unresolved, and resolving in ZERO of six leave-one-out
// subsets). ⚠️ That was published as "+15.4 a season, failing five of six" —
// a per_gw reading of an event count, taken on a dirty tree; both figures are
// withdrawn. Read the banked cells at --scale=per_path. Nothing here has ever measured
// what the chip itself buys.
//
// # ⚠️ The record says this cannot be measured, and that claim is being tested
// # rather than assumed
//
// The standing position is that the replay cannot value a wildcard because "this
// policy has nothing for a wildcard to undo" — the weekly search never takes a
// position it could not unwind, so the chip rebuilds toward the objective the
// policy was already following and pays a week of transfers for it.
//
// That reasoning has a premise the repair-cost series later measured and
// contradicted: even the policy's own evolving fifteen sits a floor of 5.83 to
// 9.29 players away from a fresh unconstrained optimum, in 6 of 8 cells, and the
// gap is FLAT rather than decaying — a standing gap, not accumulated neglect. If
// that gap is real then there IS something to repair, and what a wildcard buys is
// closing it in one week instead of over the five-to-nine weeks of single
// transfers the policy would otherwise need, hit costs included.
//
// So this runs the comparison the older claim says is pointless. A null
// reproduces that claim on the current harness; anything else revises it.
//
// # Design, set by the owner
//
//   - **Against not playing one at all**, rather than against a different week.
//   - **On the EVOLVING squad** — the policy's own maintained fifteen — and
//     deliberately not the frozen opening fifteen. A frozen squad measures what a
//     wildcard is worth to an inattentive manager, and this project does not get
//     to claim credit for repairing neglect its own policy would not have caused.
//     The evolving arm is therefore a FLOOR on a real manager's gain.
//   - **Attributed from the wildcard until the NEXT wildcard, or season end.** A
//     second wildcard rebuilds the squad again and washes out the first one's
//     effect, so a chip's value belongs to the span it actually governs. Here
//     exactly one wildcard is played, so there is no next one and the span is
//     wildcard-to-season-end.
//   - **Which the whole-season difference gives exactly, with no windowing.**
//     Both arms run the identical policy from the identical entry, so every
//     gameweek before the chip is byte-identical between them and the full-season
//     difference contains nothing but the post-wildcard effect. ⚠️ That identity
//     is what licenses the un-windowed read, and it holds only while ONE wildcard
//     is played: a two-wildcard arm would need each chip attributed to its own
//     span, and the season total would answer neither.
//
// # Placement
//
// A fixed offset, not the anchored rule. Timing is a separate question with its
// own (unresolved) answer, and mixing the two would leave a positive result
// unattributable between "the chip is worth something" and "this week was a good
// week". The offset is `wildcardControlOffset`, the same one the decomposition's
// control arm used.
//
// ⚠️ One arm plays a wildcard and the other plays no chips at all, so unlike
// every other comparison in this area the two arms do NOT hold the chip set
// equal — that is the point here, and it is why `matchedChips` and its relatives
// are deliberately not used.

import (
	"fmt"
	"os"
	"testing"

	"armband/internal/analysis"
)

func TestDiagWildcardValue(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	starts := sweepStarts()

	fmt.Printf("\n=== what is a wildcard worth? Playing one against playing none.\n")
	fmt.Printf("Both arms run the same policy on the same evolving squad from the\n")
	fmt.Printf("same entry, so every week before the chip is identical and the\n")
	fmt.Printf("season difference is the wildcard's own effect. Metric: POLICY.\n")
	fmt.Printf("Wildcard at entry+%d. No other chip is played in either arm.\n",
		wildcardControlOffset)

	none := func(cur *Season, start int) analysis.ChipPlan {
		return analysis.ChipPlan{}
	}

	arms := []policyVariant{
		{label: "no wildcard (control)",
			apply: func(sc *SimConfig) { sc.ChipPlanner = none }},
		{label: "wildcard at a fixed offset",
			apply: func(sc *SimConfig) { sc.ChipPlanner = wildcardControl }},
	}
	runPolicySweep(t, arms, starts)

	fmt.Printf("\nA null here reproduces the standing claim that this policy has\n")
	fmt.Printf("nothing for a wildcard to undo. A positive result revises it, and\n")
	fmt.Printf("is a FLOOR: the evolving squad is better maintained than a real\n")
	fmt.Printf("manager's, so it understates what the chip buys in practice.\n")
}
