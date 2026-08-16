package backtest

import (
	"fmt"
	"os"
	"sort"
	"testing"
)

// What could truncating the transfer horizon at a wildcard possibly save?
//
//	DIAG=1 go test ./internal/backtest -run '^TestDiagPreWildcardReturn$' -v -timeout 2h
//
// # A bound, not a sweep, and that is the point
//
// A transfer made three gameweeks before a planned wildcard earns for three
// gameweeks, not five: `playWildcard` replaces all fifteen. The gate scores and
// charges it over `DecisionHorizon` regardless. `effectiveHorizon` already
// truncates the same quantity at GW38 for exactly this reason and does not
// truncate at the wildcard — one quantity, two terminating events, one
// implemented.
//
// The obvious response is a two-arm sweep. This runs first because it is one arm
// and it can *close* the line: truncation can only ever refuse moves, so the sum
// of what the refusable moves actually lost is an upper bound on what perfect
// truncation could save. If that bound is small, no sweep is worth running, and
// this record's own rule is to make the comparison sharper rather than run more
// cells.
//
// # What it measures
//
// One arm, the shipped policy, with a wildcard planned the gameweek before the
// anchored bench boost — `sequencePlan`, the same placement the 2x2 uses, so the
// two are readable against each other. For every transfer made inside the
// wildcard's shadow (the decision weeks whose horizon overshoots it), it scores
// the two players over **the gameweeks the move actually earned in** — from the
// week it was made to the week before the rebuild — and subtracts what it paid.
//
//   - **`refusable`** is the sum over moves that came out negative. Truncation
//     can only refuse moves, so this is the ceiling on what it could recover.
//   - **`total`** is the sum over all of them. It is what truncation would destroy
//     if it refused the good ones too, which a horizon rule cannot help doing —
//     it is a blunt instrument that cannot see which is which in advance.
//
// The gap between them is the whole size of the question. A rule that recovered
// every negative and lost every positive would net `total`; one that recovered
// everything and lost nothing would net `-refusable`. Nothing real is outside
// that interval.
//
// # Pre-registered
//
//   - **If `refusable` is under about 15 points a season the line closes on a
//     bound**, and the sweep is not run. That figure is the record's own
//     comparable: perfect price timing, the other "act at the right moment"
//     capability, is +15 a season at t 0.95 and is recorded as a bound rather
//     than a measurement.
//   - `total` is expected to be **positive** — these are moves the shipped policy
//     chose and its transfers are worth points on average — which is precisely why
//     a horizon truncation is not obviously an improvement.
//   - This is an **event count, not a rate.** Report per season-path. Dividing a
//     handful of decisions by gameweeks played is the inflation this record has
//     already paid for once.
//   - It is a bound on the *points* channel only. `playWildcard` rebuilds from the
//     squad's **selling value**, so money accreted by a pre-wildcard move survives
//     the rebuild — and `budgetWeight` ships at zero, so the gate prices that at
//     nothing and this diagnostic cannot see it either. Both sides of the eventual
//     comparison share the blindness, so it does not bias a sweep; it does mean
//     "provably correct" overstates the truncation.
func TestDiagPreWildcardReturn(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)
	starts := sweepStarts()

	fmt.Printf("\n=== what could truncating the horizon at a wildcard save?\n")
	fmt.Printf("A bound, from one arm. Truncation can only REFUSE moves, so the\n")
	fmt.Printf("negative half of what pre-wildcard transfers actually returned is\n")
	fmt.Printf("the ceiling on what it could recover, and the total is what it\n")
	fmt.Printf("would destroy if it refused the good ones too.\n")
	fmt.Printf("**Read per season-path.** These are events, not a rate.\n\n")

	fmt.Printf("%-9s %5s %6s %7s %9s %9s %9s\n",
		"season", "start", "wc", "moves", "total", "refusable", "per move")

	var totAll, refAll float64
	var movesAll, cells, goneAll int
	bySeason := map[string]float64{}
	cellsBySeason := map[string]int{}

	for _, pair := range loadPairs(t, cfg) {
		for _, start := range starts {
			sc := sweepConfig(cfg, start, false)
			sc.ChipPlanner = sequencePlan
			res, err := Simulate(pair.Cur, pair.Prior, sc)
			if err != nil {
				t.Fatalf("%s@%d: %v", pair.Name, start, err)
			}
			wc := sc.ChipPlanner(pair.Cur, start).Wildcard
			if wc == 0 {
				continue // no wildcard: nothing casts a shadow
			}
			cells++

			var total, refusable float64
			n, gone := 0, 0
			for _, mv := range res.Moves {
				// Inside the shadow: made before the rebuild, and on a decision
				// whose horizon overshoots it.
				if mv.GW >= wc {
					continue
				}
				h := effectiveHorizon(sc.decisionHorizon(), mv.GW)
				if float64(mv.GW)+h <= float64(wc) {
					continue // the horizon already ended before the wildcard
				}
				// Scored over the weeks it actually earned in, and charged only
				// what refusing it would actually give back.
				//
				// ⚠️ **A free transfer is charged nothing here, and the first
				// version of this charged `FreeCost`.** That constant is a
				// *confidence threshold*, not points surrendered — refusing the
				// move recovers none of it, the manager simply keeps a transfer
				// whose shadow price this record measures at approximately zero.
				// Counting it inflated `refusable`, which is defined as what a
				// rule could *recover*, by 2 points per negative move. A hit is
				// different: those four points are real and are handed back.
				weeks := wc - mv.GW
				got := float64(pointsOver(pair.Cur, mv.InID, mv.GW, weeks) -
					pointsOver(pair.Cur, mv.OutID, mv.GW, weeks))
				paid := 0.0
				if mv.Hit {
					paid = 4
				}
				net := got - paid
				// The known bias, counted rather than caveated. When the player
				// sold records no minutes at all, `pointsOver` scores him zero
				// and this overstates the move — an autosub would have covered
				// him for free. The package measures that at 19% of transfers
				// and -2.223 pts/gw; `Judge` carries `OutPlayed` for it and this
				// post-mortem does not, so the count is the exposure.
				if minutesOver(pair.Cur, mv.OutID, mv.GW, weeks) == 0 {
					gone++
				}
				total += net
				if net < 0 {
					refusable += net
				}
				n++
			}
			totAll += total
			refAll += refusable
			movesAll += n
			goneAll += gone
			bySeason[pair.Name] += total
			// Per season, over the cells that placed a wildcard rather than over
			// the whole start grid. Dividing by `len(starts)` diluted a season
			// whose wildcard could not be placed in every cell — the same
			// "pooling cells where the intervention could not run" error this
			// package keeps correcting.
			cellsBySeason[pair.Name]++

			per := 0.0
			if n > 0 {
				per = total / float64(n)
			}
			fmt.Printf("%-9s %5d %6d %7d %9.1f %9.1f %9.2f\n",
				pair.Name, start, wc, n, total, refusable, per)
		}
	}

	if cells == 0 {
		t.Fatal("no cell placed a wildcard — the bound has no population")
	}
	fmt.Printf("\n--- the bound, per season-path ---\n")
	fmt.Printf("cells with a wildcard:        %d\n", cells)
	fmt.Printf("moves inside the shadow:      %d  (%.1f per cell)\n",
		movesAll, float64(movesAll)/float64(cells))
	fmt.Printf("  of which the seller went on to record NO minutes: %d (%.0f%%)\n",
		goneAll, 100*float64(goneAll)/float64(movesAll))
	fmt.Printf("  those overstate the move — an autosub would have covered him — so\n")
	fmt.Printf("  the refusable ceiling below is biased SMALL by that share.\n")
	fmt.Printf("what they actually returned:  %+.1f per season-path\n", totAll/float64(cells))
	fmt.Printf("REFUSABLE (the upper bound):  %+.1f per season-path\n", refAll/float64(cells))
	fmt.Printf("\nA perfect truncation recovers at most the refusable figure, and a\n")
	fmt.Printf("blunt one — which a horizon rule is — gives back some of the total.\n")
	fmt.Printf("Nothing real lies outside that interval.\n")

	fmt.Printf("\nby season (total per cell):\n")
	names := make([]string, 0, len(bySeason))
	for s := range bySeason {
		names = append(names, s)
	}
	sort.Strings(names)
	for _, s := range names {
		fmt.Printf("  %-9s %+8.1f  (%d cells)\n",
			s, bySeason[s]/float64(cellsBySeason[s]), cellsBySeason[s])
	}
}
