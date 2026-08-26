package backtest

// Is the chip bundle better placed the way managers actually place it?
//
//	DIAG=1 FPL_CELLS=<path> go test ./internal/backtest -run TestDiagTwoRegimeChips -v -timeout 45m
//
// # The rule, from the user's own practice
//
//	"Bundle the wildcard, free hit, and bench boost in the second half of the
//	year. Always. Triple captain is an afterthought. Best available gameweek
//	after those 3 are played.
//	First half is more open. There are no blanks and doubles so you mostly just
//	wildcard if your team is bad."
//
// # ⚠️ The premise is right, and the arm it corrects was diluting itself
//
// Counted from the fixtures the replay loads, across the six played seasons:
// **two double gameweeks fall in GW1-19 against forty in GW20-38**, and the two
// most recent seasons have none at all before GW20. `TestDiagAnchoredChips`
// calendar-anchors chips across all 38 gameweeks, so in the first half its
// anchored arm and its fixed-offset control are choosing between weeks that are
// indistinguishable on the signal the rule reads — diluting the contrast across
// roughly half the grid.
//
// # What differs from the anchored arm
//
//  1. **The wildcard is IN the bundle.** `TestDiagAnchoredChips` excludes it
//     entirely and it has never had a bundled figure.
//  2. **The bundle is confined to GW20+**, where the calendar features are.
//  3. **Triple captain is placed LAST**, on the best gameweek still free after
//     the other three have taken theirs — rather than being given the
//     *second*-best double, which is why it goes unplaced in 16 of 36 cells.
//
// # ⚠️ Two things this canNOT do, and neither is a detail
//
// **It cannot give a season two chip sets that season did not have.**
// `ChipSetsFor` returns 1 before 2025-26, so five of the six seasons hold a
// single set of four chips for the whole year. The record's own standing rule is
// *"do not project the two-set chip rule backwards to buy chip observations"*,
// and this obeys it: the bundle is one set, placed late.
//
// **It cannot express the reactive first half at all.** `analysis.ChipPlan` has
// one slot per chip, so a planner returning a `ChipPlan` cannot hold both a
// first-half wildcard and a second-half one — that needs `analysis.ChipSchedule`,
// which this seam does not carry. "Wildcard if your team is bad" also needs a
// squad-quality signal during the run, which a plan resolved up front cannot see.
// `xiDriftOf` is the signal and it now exists; the seam does not. **So this tests
// the second-half bundle and NOT the two-regime strategy**, and a null here says
// nothing about the first half.

import (
	"fmt"
	"os"
	"testing"

	"armband/internal/analysis"
)

// secondHalfBundle is the user's rule expressed as a plan.
func secondHalfBundle(lag int) func(cur *Season, start int) analysis.ChipPlan {
	return func(cur *Season, start int) analysis.ChipPlan {
		// Never before the reset week, and never before entry.
		from := ChipResetGW - 1
		if start > from {
			from = start
		}
		return bundleFrom(cur, from, lag)
	}
}

// bundleFrom places the three calendar chips on the best features after `after`,
// then the triple captain on the best gameweek still free.
//
// It reuses `sightedWeeks`'s own selection by calling it with a later start,
// rather than reimplementing the double/blank search — one implementation of
// "which week is biggest", not two.
func bundleFrom(cur *Season, after, lag int) analysis.ChipPlan {
	p := sightedWeeks(cur, after, lag)
	// sightedWeeks fills BenchBoost, FreeHit and TripleCaptain. The wildcard is
	// what it has no opinion about, so it takes the best double still free —
	// which is what a manager rebuilding into a double gameweek is doing.
	taken := map[int]bool{p.BenchBoost: true, p.FreeHit: true, p.TripleCaptain: true}
	// The wildcard takes the best double still free after those three.
	for gw := after + 1; gw <= 38; gw++ {
		if !taken[gw] && p.Wildcard == 0 && weekHasDouble(cur, gw) {
			p.Wildcard = gw
			taken[gw] = true
			break
		}
	}
	// ⚠️ Triple captain LAST and always placeable: the best remaining double if
	// one is free, otherwise the first free week. The anchored arm's rule leaves
	// it unplaced in 16 of 36 cells, which is a chip not played rather than a
	// chip timed badly.
	if p.TripleCaptain == 0 {
		for gw := after + 1; gw <= 38; gw++ {
			if !taken[gw] {
				p.TripleCaptain = gw
				taken[gw] = true
				break
			}
		}
	}
	return p
}

// weekHasDouble is whether any club plays twice in `gw`.
func weekHasDouble(cur *Season, gw int) bool {
	_, count, teams := teamGameweeks(cur.Fixtures)
	for team := range teams {
		if count[gw][team] >= 2 {
			return true
		}
	}
	return false
}

// fourChipControl is the same four chips at fixed offsets from entry — the
// arbitrary rule the bundle has to beat. It extends `controlWeeks` with a
// wildcard slot, because an arm that plays four chips must be compared against
// an arm that plays four.
func fourChipControl(cur *Season, start int) analysis.ChipPlan {
	p := controlWeeks(cur, start)
	taken := map[int]bool{p.BenchBoost: true, p.FreeHit: true, p.TripleCaptain: true}
	for _, off := range []int{9, 10, 11, 12, 13, 8, 7, 6, 5, 4, 3, 2, 1} {
		gw := start + off
		if gw <= 38 && gw > start && !taken[gw] {
			p.Wildcard = gw
			break
		}
	}
	return p
}

func TestDiagTwoRegimeChips(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	starts := sweepStarts()

	fmt.Printf("\n=== the second-half bundle: wildcard + free hit + bench boost on\n")
	fmt.Printf("the calendar after GW%d, triple captain placed last on what is\n", ChipResetGW-1)
	fmt.Printf("left. Control plays the same FOUR chips at fixed offsets.\n")
	fmt.Printf("⚠️ One chip set: five of six seasons had only one, and this does\n")
	fmt.Printf("not project the two-set rule backwards. Metric: POLICY.\n")

	arms := []policyVariant{
		{label: "four chips, fixed offsets (control)",
			apply: func(sc *SimConfig) { sc.ChipPlanner = fourChipControl }},
		{label: "second-half bundle (4gw sight)",
			apply: func(sc *SimConfig) { sc.ChipPlanner = secondHalfBundle(4) }},
		{label: "second-half bundle (full sight)",
			apply: func(sc *SimConfig) { sc.ChipPlanner = secondHalfBundle(fullSight) }},
	}
	runPolicySweep(t, arms, starts)

	fmt.Printf("\n⚠️ Read against TestDiagAnchoredChips, which is a DIFFERENT arm:\n")
	fmt.Printf("three chips not four, spread over all 38 gameweeks, and a triple\n")
	fmt.Printf("captain that goes unplaced in 16 of 36 cells. These cells are not\n")
	fmt.Printf("comparable with those and must not be differenced against them.\n")
}
