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
// # ⚠️ It replays every season under TODAY'S chip rules, on purpose
//
// `ChipSets: 2` gives all six seasons two sets, which five of them did not have.
// The record's standing rule says not to, and the user overrode it 2026-08-25:
// *"We are just applying current chip rules to old data, but the underlying
// points are the same."* That is right — scores, minutes and fixtures are
// untouched and only the ALLOWANCE is counterfactual, which is the question a
// product advising under current rules actually has.
//
// ⚠️ **The POWER half of that warning survives and is carried here.** Across the
// six archived first halves there are 15 doubling club-gameweeks out of 189, and
// 11 of the 15 are one COVID-rescheduled 2020-21 round. **A first-half chip arm
// is collinear with "a chip on a plain week" in five seasons of six**, so the
// projected first set buys observations that can distinguish almost nothing.
// Expect variance, not signal, from that half — and never read a null there as a
// fact about chips.
//
// # ⚠️ The first-half wildcard is placed by OFFSET, not by squad quality
//
// The rule is *"you mostly just wildcard if your team is bad"*, which is a
// condition on the squad and not a position on the calendar. Measuring it needs a
// squad-quality reading DURING the run; `xiDriftOf` is that reading and it now
// exists, but the planner seam resolves a whole season up front and cannot see a
// live squad. **So the first set here is a placeholder and this arm does not test
// the reactive rule.** A null in the first half says nothing about it.

import (
	"fmt"
	"os"
	"testing"

	"armband/internal/analysis"
)

// twoRegimeSchedule is the user's rule expressed across BOTH chip sets: a
// first-set wildcard early, and the other three bundled on the second-half
// calendar with the triple captain placed last.
func twoRegimeSchedule(lag int) func(cur *Season, start int) analysis.ChipSchedule {
	return func(cur *Season, start int) analysis.ChipSchedule {
		var sch analysis.ChipSchedule
		// First set: the wildcard only. Placed by offset because the rule it
		// stands in for is a condition on the squad, not the calendar — see the
		// file header. It must land before the reset or it is not a first-set
		// chip at all.
		if wc := start + 4; wc < ChipResetGW {
			sch.First.Wildcard = wc
		}
		from := ChipResetGW - 1
		if start > from {
			from = start
		}
		sch.Second = bundleFrom(cur, from, lag)
		return sch
	}
}

// secondHalfBundle is the same bundle without the first-set wildcard, so the two
// can be told apart: it isolates "the bundle is worth placing on the calendar"
// from "and an early wildcard is worth having as well".
func secondHalfBundle(lag int) func(cur *Season, start int) analysis.ChipPlan {
	return func(cur *Season, start int) analysis.ChipPlan {
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
	fmt.Printf("⚠️ TWO chip sets in every season, including five that had one:\n")
	fmt.Printf("today's rules on old football, deliberately. Metric: POLICY.\n")

	// ⚠️ Every arm declares ChipSets: 2 in its LABEL as well as its config,
	// because that field is not fingerprinted and a sidecar cannot tell these
	// cells from ones measured under the rules the seasons actually had.
	arms := []policyVariant{
		{label: "four chips, fixed offsets (control, 2 sets)",
			apply: func(sc *SimConfig) { sc.ChipSets = 2; sc.ChipPlanner = fourChipControl }},
		{label: "second-half bundle, 4gw sight (2 sets)",
			apply: func(sc *SimConfig) { sc.ChipSets = 2; sc.ChipPlanner = secondHalfBundle(4) }},
		{label: "two-regime: early wildcard + second-half bundle (2 sets)",
			apply: func(sc *SimConfig) { sc.ChipSets = 2; sc.ChipScheduleP = twoRegimeSchedule(4) }},
	}
	runPolicySweep(t, arms, starts)

	fmt.Printf("\n⚠️ Read against TestDiagAnchoredChips, which is a DIFFERENT arm:\n")
	fmt.Printf("three chips not four, spread over all 38 gameweeks, and a triple\n")
	fmt.Printf("captain that goes unplaced in 16 of 36 cells. These cells are not\n")
	fmt.Printf("comparable with those and must not be differenced against them.\n")
}
