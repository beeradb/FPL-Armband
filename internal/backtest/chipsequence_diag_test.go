package backtest

import (
	"fmt"
	"os"
	"testing"

	"armband/internal/analysis"
)

// Does the wildcard-into-boost sequence pay, on the paired grid?
//
//	DIAG=1 EXP=CHIPSEQ FPL_CELLS=/tmp/chipseq/cells.csv \
//	    go test ./internal/backtest -run '^TestDiagChipSequencePaired$' -count=1 -v -timeout 6h
//
// # Why this exists when TestDiagChipSequence already does
//
// That test measures the same sequence and measures it the noisiest way this
// record has: season *totals* down a single path, from one entry gameweek, on
// three or four seasons. This file's own standing rule is to judge a sweep on
// paired differences rather than on totals, and the record's is that a totals-era
// figure is the least trustworthy kind it carries. So the play the chip actually
// lives in — wildcard, then boost the week after — has never been measured on the
// 36-cell paired grid.
//
// `TestDiagChipPreparation` did not measure it either, and deliberately: no arm
// there plays a wildcard at all. That was right for *that* question, because the
// anchored post-mortem found pinning the wildcard put the boost immediately after
// the rebuild in 30 of 30 cells for one arm against 3-5 for the others, which
// installs the effect being tested into the baseline. But that finding closes
// *pinning against arms whose boost weeks differ*. Here every arm shares one
// placement and one wildcard week, so the wildcard is genuinely common-mode and
// cancels from the paired difference — which is the condition
// AGENTS.md's rule actually states: holding a confounder constant is safe when it
// is constant *with respect to the thing being varied*.
//
// # The arms are a 2x2, and that is the repair
//
// The first version of this ran three arms across two sweeps and read the
// interaction between them by eye. That is not a paired contrast: the two sweeps
// place different chips — one plays a wildcard and one does not — so a difference
// between them conflates "preparation on top of preparation" with "preparation in
// a season containing a rebuild". The four combinations in one sweep give a
// cell-paired interaction with a standard error instead.
//
// Every arm plays the same chips in the same weeks: the anchored placement plus a
// wildcard the gameweek before the bench boost. What varies is which preparation
// channel is on.
//
//	                    | rebuild blind | rebuild builds for the boost
//	transfers blind     | neither       | rebuild only
//	transfers prepare   | transfers only| both
//
// **The interaction is the estimand**: (both − transfers only) − (rebuild only −
// neither), per cell. It is what "the chips are played in unison" means as a
// measurable quantity — whether the two channels add, or whether one makes the
// other redundant.
//
// # Pre-registered
//
//   - **The interaction is predicted NEGATIVE on the chip week.** A wildcard one
//     week earlier has already bought the bench, so ordinary transfers have little
//     left to add and can only cost XI quality elsewhere. A positive interaction
//     would be surprising and worth distrusting.
//   - The rebuild channel should be **large and positive on the boost week**: the
//     opening-squad path buys 59% more bench quality for £2.5m when told the chip
//     is coming, and here it has a whole wildcard to spend.
//   - **The primary is the chip week's own gain**, now a per-cell column
//     (`bench_boost_pts`), paired. The season POLICY line is **declared
//     expected-unresolved in advance**. ⚠️ The reason first given here — "the
//     interaction costs roughly twice the main effect's SE" — is an
//     independent-samples rule and is **wrong on a paired grid**: measured on this
//     sweep's own cells the interaction is *cheaper* than the noisier factor's
//     main effect, 0.216 against 0.599 season-clustered. A difference of
//     differences within one cell cancels the path divergence a single difference
//     carries. Quote the threshold from this comparison's own cells, never a
//     global median.
//   - **The denominator is the cells that place the chip**, not the grid. A cell
//     with no bench boost is one where the intervention could not run.
//   - **HOLD must be byte-identical in all four arms.** ⚠️ An earlier version of
//     this comment predicted the opposite — that a wildcard rebuilds the fifteen
//     so the held arm must diverge. That was wrong: HOLD never transfers and never
//     plays a chip, so no chip setting can reach it. It is a confinement check
//     here exactly as it is in TestDiagChipPreparation.
//   - **Report naive, CR2-on-season and CR2-on-start together**, and prefer the
//     start/fixed reading where the season variance component is indistinguishable
//     from zero — a clustered SE can collapse because the axis is null rather than
//     because the seasons agree, which this record has already retracted once.
func TestDiagChipSequencePaired(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	starts := sweepStarts()

	fmt.Printf("\n=== does building the wildcard squad FOR the bench boost pay?\n")
	fmt.Printf("Every arm plays the same chips in the same weeks: anchored placement\n")
	fmt.Printf("plus a WILDCARD the week before the boost — the sequence the chip is\n")
	fmt.Printf("actually played in, and the one TestDiagChipPreparation had to leave\n")
	fmt.Printf("out to keep its own baseline clean.\n")
	fmt.Printf("The wildcard is common-mode here, so its own value cancels; what\n")
	fmt.Printf("varies is whether the rebuild knows the chip is coming.\n")
	fmt.Printf("**HOLD MUST NOT MOVE.** It never transfers and never plays a chip,\n")
	fmt.Printf("so no chip setting can reach it — a HOLD MOVED flag here is a real\n")
	fmt.Printf("confinement failure, not an expected consequence of the wildcard.\n")
	fmt.Printf("**Read the boost-week table under the grid first.**\n\n")

	// Print the plans once, so the arms are auditable rather than asserted.
	cfg := loadConfig(t)
	placed, skipped := 0, 0
	for _, pair := range loadPairs(t, cfg) {
		for _, start := range starts {
			p := sequencePlan(pair.Cur, start)
			if p.Wildcard > 0 {
				placed++
			} else {
				skipped++
			}
			fmt.Printf("  %-9s @%-3d WC%2d BB%2d FH%2d TC%2d\n",
				pair.Name, start, p.Wildcard, p.BenchBoost, p.FreeHit, p.TripleCaptain)
		}
	}
	fmt.Printf("\nwildcard placed in %d cells, absent in %d (no boost, or the week\n",
		placed, skipped)
	fmt.Printf("before it already carries another chip). A cell with no wildcard is\n")
	fmt.Printf("identical in every arm and contributes a zero to each difference.\n")

	type weekGain struct{ gain, cells int }
	seen := make([]weekGain, 4)
	watch := func(i int) func(seasonPair, int, *SimResult) {
		return func(_ seasonPair, _ int, res *SimResult) {
			for _, w := range res.Weeks {
				if w.BenchBoost {
					seen[i].gain += w.BenchBoostGain
					seen[i].cells++
				}
			}
		}
	}

	// The 2x2. Every arm plays the same chips in the same weeks; the wildcard is
	// common-mode. What varies is which of the two preparation channels is on.
	arms := []policyVariant{
		{
			label: "neither",
			apply: func(sc *SimConfig) {
				sc.ChipPlanner = sequencePlan
				sc.WildcardIgnoresBoost = true
			},
			observe: watch(0),
		},
		{
			label: "rebuild only",
			apply: func(sc *SimConfig) {
				sc.ChipPlanner = sequencePlan
			},
			observe: watch(1),
		},
		{
			label: "transfers only",
			apply: func(sc *SimConfig) {
				sc.ChipPlanner = sequencePlan
				sc.WildcardIgnoresBoost = true
				sc.PrepareBenchBoost = true
			},
			observe: watch(2),
		},
		{
			label: "both",
			apply: func(sc *SimConfig) {
				sc.ChipPlanner = sequencePlan
				sc.PrepareBenchBoost = true
			},
			observe: watch(3),
		},
	}

	runPolicySweep(t, arms, starts)

	fmt.Printf("\n--- what the bench boost returned in the week it was played ---\n")
	fmt.Printf("%-32s %16s\n", "arm", "bench boost")
	for i, a := range arms {
		if seen[i].cells == 0 {
			fmt.Printf("%-32s %16s\n", a.label, "-")
			continue
		}
		fmt.Printf("%-32s %10.1f (%d)\n", a.label,
			float64(seen[i].gain)/float64(seen[i].cells), seen[i].cells)
	}
	fmt.Printf("\nThe INTERACTION is the point: (both - transfers only) minus\n")
	fmt.Printf("(rebuild only - neither). If the two channels are redundant it is\n")
	fmt.Printf("negative — the rebuild has already bought the bench, so ordinary\n")
	fmt.Printf("transfers have nothing left to add and only cost XI quality.\n")
	fmt.Printf("Compute it from the per-cell columns in FPL_CELLS, not from these\n")
	fmt.Printf("means: bench_boost_pts is now a per-cell column, so the interaction\n")
	fmt.Printf("is a paired contrast with a standard error rather than four\n")
	fmt.Printf("arm-level averages differenced by eye.\n")
}

// sequencePlan is the anchored placement plus a wildcard the week before the
// bench boost.
//
// It delegates to `anchoredPlan` rather than restating the placement rule: a
// second copy of "which week carries the biggest double" is the drift this
// package keeps paying for, and the two chip diagnostics have to place the chips
// identically or their results cannot be read against each other.
//
// The wildcard is dropped rather than moved when the week before the boost is
// unavailable — it is before the entry gameweek, or another chip already holds
// it, which FPL forbids. Moving it elsewhere would make the wildcard's *week* a
// variable, and a cell where it cannot be placed is simply a cell where all four
// arms are identical.
func sequencePlan(cur *Season, start int) analysis.ChipPlan {
	p := anchoredPlan(cur, start)
	if p.BenchBoost == 0 {
		return p
	}
	wc := p.BenchBoost - 1
	if wc <= start || wc == p.FreeHit || wc == p.TripleCaptain {
		return p
	}
	p.Wildcard = wc
	return p
}
