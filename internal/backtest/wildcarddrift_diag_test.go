package backtest

// WHEN DO YOU PLAY A WILDCARD THAT HAS NO DOUBLE TO ANCHOR TO?
//
// That is the whole question here, and the first-half window is only where it
// happens to be true. A wildcard aimed at a double gameweek is a CALENDAR
// decision and is measured elsewhere — anchor it on the biggest double within
// sight. A wildcard with no such target is a decision about the SQUAD: play it
// when the squad is bad enough. This asks what "bad enough" means, and whether
// the measure the shipped rule uses can express it.
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
// # ⚠️ ONE variable between the arms, and an earlier version had two
//
// Both arms go through `analysis.ChipBarAt`, so both keep the option-value decay
// and differ only in **what they read**: repair cost against XI drift.
//
// A first version bypassed the bar for the drift arm, reasoning that it prices a
// one-off hit cost while drift is a per-gameweek rate. That confused a UNIT with
// a SHAPE — `ChipReservationAt` is `base * Factor` with a dimensionless factor,
// so a base in drift units decays into drift units. The confound was real while
// it lasted: the drift arm would have differed in the reading AND in whether it
// waited, and any difference between the arms would have been unattributable.
// **Recorded because the mistake is cheap to repeat and was written up as an
// unavoidable limitation before it was checked.**
//
// ⚠️ **The bars are still not comparable across arms**, and sharing the curve does
// not make them so: the cost arm's base is in hit points and the drift bars are
// in expected points per gameweek on the eleven. The DECAY is shared; the SCALE
// is not. They are swept because that is how this project locates a knob, not
// because 3.0 here means what 3.0 means there.

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"armband/internal/config"
)

// wildcardDriftBars brackets the measured drift distribution rather than
// guessing: TestDiagXIDrift reads a mean of 3.98 points on the XI five gameweeks
// after entry, with cells from 0.35 to 11.46. A bar above the top of that range
// never fires and a bar below the bottom fires in week one, so the ladder spans
// the middle where the rule can actually discriminate.
// ⚠️ Bracketing the FIRST-HALF drift distribution, which is tighter than the
// whole-season one: a squad five gameweeks past entry has not had long to drift.
// A bar above the top never fires and one below the bottom fires in week one.
var wildcardDriftBars = []float64{1.0, 2.0, 3.0, 5.0}

func TestDiagWildcardDriftTrigger(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	starts := sweepStarts()

	weeks, seasons := firstHalfDoublingWeeks(t, loadConfig(t))
	fmt.Printf("\n=== FIRST-HALF wildcard trigger: XI DRIFT against REPAIR COST.\n")
	fmt.Printf("Confined to GW1-%d. Second-half timing is a CALENDAR question —\n", ChipResetGW-1)
	fmt.Printf("anchor on the doubles — and is measured elsewhere; the first half\n")
	fmt.Printf("has %d doubling gameweeks in %d seasons, so the rule there is a\n", len(weeks), seasons)
	fmt.Printf("condition on the SQUAD. Metric: POLICY.\n")
	fmt.Printf("  they are: %s\n", strings.Join(weeks, ", "))
	fmt.Printf("Control plays no wildcard trigger at all, so each arm is read\n")
	fmt.Printf("against not having the rule rather than against the other rule.\n")
	fmt.Printf("Both arms keep the option-value decay — same curve, different base\n")
	fmt.Printf("and different reading — so the arms differ in ONE variable.\n")
	fmt.Printf("⚠️ The BASES are still not comparable: hit points against points per\n")
	fmt.Printf("gameweek on the eleven. The decay is shared; the scale is not.\n")

	arms := []policyVariant{
		{label: "no wildcard trigger (control)",
			apply: func(sc *SimConfig) { sc.WildcardTrigger = false }},
		{label: "first-half trigger on repair cost (shipped rule)",
			apply: func(sc *SimConfig) {
				sc.WildcardTrigger = true
				sc.WildcardTriggerFirstHalfOnly = true
			}},
	}
	for _, b := range wildcardDriftBars {
		bar := b
		arms = append(arms, policyVariant{
			label: fmt.Sprintf("first-half trigger on XI drift > %.1f pts/gw", bar),
			apply: func(sc *SimConfig) {
				sc.WildcardTrigger = true
				sc.WildcardTriggerFirstHalfOnly = true
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

// firstHalfDoublingWeeks lists every (season, gameweek) before ChipResetGW where
// at least one club plays twice, over the PLAYED seasons of the grid in front of
// you — the second of each pair, since a prior is never replayed.
//
// It is derived rather than written down because the sentence it feeds is the
// whole justification for treating the first half as a squad condition rather
// than a calendar one, and FPL_SWEEP_SEASONS moves the grid underneath it.
//
// ⚠️ **It replaces a hard-coded "two", which was wrong.** On the extended grid
// there are three — 2020-21 GW19 (11 clubs), 2022-23 GW19 (2) and 2023-24 GW7
// (2) — and the label claimed two until TestPrintedGridLabelsAreDerived refused
// the literal and the count was checked. The argument survives either number;
// the point is that a count nobody could check had been printed as a premise.
func firstHalfDoublingWeeks(t *testing.T, cfg config.Config) ([]string, int) {
	t.Helper()
	var out []string
	seen := map[string]bool{}
	for _, p := range sweepPairNames() {
		if seen[p[1]] {
			continue
		}
		seen[p[1]] = true
		s := loadSeason(t, cfg, p[1])
		played, count, teams := teamGameweeks(s.Fixtures)
		for gw := 1; gw < ChipResetGW; gw++ {
			if !played[gw] {
				continue
			}
			var n int
			for team := range teams {
				if count[gw][team] >= 2 {
					n++
				}
			}
			if n > 0 {
				out = append(out, fmt.Sprintf("%s GW%d (%d clubs)", p[1], gw, n))
			}
		}
	}
	return out, len(seen)
}
