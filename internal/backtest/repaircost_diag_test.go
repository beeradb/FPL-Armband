package backtest

// The repair cost as a TIME SERIES, which is the one shape `TestDiagWildcardTrigger`
// could not produce.
//
//	DIAG=1 FPL_SWEEP_SEASONS=default FPL_SWEEP_STARTS=1,16 \
//	  go test ./internal/backtest -run TestDiagRepairCostSeries -v -timeout 2h
//
// # The question
//
// The wildcard trigger's diagnostic found the repair cost large from the first week
// it could be measured — 20/36/24/32 points at a GW1 entry and 12/12/12/4 at GW16,
// which at `free` = 2 is 7/11/8/10 and 5/5/5/3 of the fifteen. Two mechanisms fit
// and it could not separate them, because the rule fires on first consultation and
// then becomes ineligible, so the cost is never seen as a series on a fixed squad:
//
//   - **CHURN**, a rate. The model over-weights recent football, so one gameweek of
//     data moves its preferences a long way. Predicts the gap is largest when
//     information is poorest and SHRINKS as the season accumulates data.
//   - **A STANDING GAP**, a level. Any held fifteen differs from a fresh
//     unconstrained argmax over the whole pool at every cutoff, because the argmax
//     is constrained neither by what is already owned nor by what selling costs.
//     Predicts the gap is non-zero and roughly FLAT.
//
// # What this measures, and what makes it a discriminator
//
// Two series per cell, both over the whole replayed season, and ⚠️ **they answer
// different questions and must not be pooled**:
//
//	FROZEN    the opening fifteen, held all season and never sold — the squad the
//	          `HOLD` arm scores. The squad is constant while the football is not,
//	          so decay accumulates here: a RISING series is decay and a flat one is
//	          not.
//	EVOLVING  the fifteen the `POLICY` arm actually holds. The policy is repairing
//	          continuously, so a persistent non-zero LEVEL is the standing gap.
//
// The predictions, written before the run so the result can fail:
//
//   - churn predicts BOTH series decline as the season progresses;
//   - a standing gap predicts EVOLVING flat and non-zero while FROZEN rises;
//   - both is a legitimate answer — a declining component on a non-zero floor —
//     and is reported as a floor and a decay rather than resolved into one.
//
// # Nothing acts on it
//
// The cost is computed and recorded and never used to trigger, transfer or
// rebuild. `SimConfig.RecordRepairCost` writes to `SimResult.RepairSeries` and no
// branch reads it; `TestTheRepairSeriesChangesNoDecision` replays a cell with the
// observer on and off and requires every point, every transfer and every weekly
// fifteen to be identical. That is not tidiness — a repair cost that could act is
// the wildcard state trigger, which is a closed line.
//
// # What is priced, stated rather than assumed
//
// The budget the fresh optimum is given is `wallet.value`, the squad's SELLING
// value, so FPL's half-of-any-rise rule is already charged on every held player
// exactly as a wildcard would pay it. That friction grows all season on a squad
// that never sells, which makes it the obvious confound for a rising FROZEN series
// — so the frozen fifteen is priced a SECOND time at market value, and the gap
// between the two change counts is printed as its own column. ⚠️ **That gap BOUNDS
// the selling-rule confound; it is not a clean subtraction.** The two pricings
// also differ in BUDGET SIZE — the gross budget is larger by the accumulated tax —
// so the fresh optimum at the gross budget solves a larger knapsack and may
// UPGRADE away from the frozen squad, and a negative gap means exactly that. If
// the standing gap is partly a selling-cost artefact, that is a finding rather
// than a nuisance.
//
// ⚠️ **What is NOT priced, in either arm, is that the fresh argmax is
// unconstrained by ownership.** It re-buys a player it already holds at his market
// price while the budget only credited his sale value, so a held player who has
// risen is charged half his rise for being kept. That is a real bias and it runs
// toward a rising series in the frozen arm. It is what the market-value column
// bounds.
//
// ⚠️ **A third mechanism sits inside the same signature and neither arm separates
// it: injury and absence accumulation.** Over a season the opening fifteen gathers
// injuries, suspensions and form losses; a fresh argmax over the current pool
// avoids the absent players and so replaces more of the frozen squad as the weeks
// pass — a RISING frozen series with a flat evolving one, the standing-gap shape.
// The market-value column bounds the selling rule but not this. So the diagnostic
// discriminates CHURN from NON-CHURN, and the non-churn residual is one thing it
// cannot decompose further.
//
// # Constraints this diagnostic keeps
//
// Counts, costs and series shapes. **No points figure, no detection threshold, no
// p-value, no recommendation to change a constant.** This discriminates two
// mechanisms; it does not measure what either is worth. The replay cannot value a
// wildcard, and none of the recorded closure is reopened by anything printed here.
//
// ⚠️ **It is expensive.** `Optimize` is the expensive call in this package and this
// makes THREE per gameweek per cell — roughly 3.5 s each on the machine this was
// written on, so a GW1 cell is about six minutes. The projected count is printed
// before the first cell runs, because a sweep killed at its timeout leaves a
// partial table that reads like a complete one with fewer cells.

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestDiagRepairCostSeries(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	pairs := loadPairs(t, cfg)
	starts := sweepStarts()

	fmt.Printf("\n=== the repair cost as a series: churn (a rate) or a standing gap (a level)\n")
	// The grid, stamped rather than described, and the data state beside it. The
	// recorded wildcard-trigger figures this is read against are a FOUR-season,
	// two-entry-point result; at the shipped default `sweepPairNames` returns six
	// seasons including 2019-20, whose POLICY path is not a sample of the same
	// process, and a reader running this bare would get a different grid under the
	// same heading.
	fmt.Printf("%s\n", gridLabel(len(pairs), len(starts)))
	fmt.Printf("FPL_SWEEP_SEASONS=%s  FPL_SWEEP_STARTS=%s  entry points %v\n",
		gridEnv("FPL_SWEEP_SEASONS", "extended (the shipped default)"),
		gridEnv("FPL_SWEEP_STARTS", "1,6,11,16,21,26 (the shipped default)"), starts)
	for _, p := range pairs {
		if !TransferPathComparable(p.Name) {
			fmt.Printf("⚠️  %s is in this grid and its transfer path is NOT "+
				"comparable — read its EVOLVING row as a decision count only.\n", p.Name)
		}
	}

	weeks := 0
	for _, s := range starts {
		weeks += (38 - s) * len(pairs)
	}
	fmt.Printf("\nprojected cost: %d observed gameweeks x 3 Optimize calls = %d, "+
		"about %s at 3.5s each.\n", weeks, weeks*3,
		(time.Duration(weeks*3*3500) * time.Millisecond).Round(time.Minute))
	fmt.Printf("Optimize is the expensive call in this package; a run killed at its\n")
	fmt.Printf("timeout leaves a partial table that reads like a complete one.\n")

	fmt.Printf("\nEach row is one gameweek. `chg` is how many of the fifteen a fresh\n")
	fmt.Printf("unconstrained optimum would replace; `free` is the allowance that arm\n")
	fmt.Printf("held; `cost` is 4 x max(0, chg - free), the hits and only the hits.\n")
	fmt.Printf("FROZEN is the opening fifteen held all season (the HOLD squad);\n")
	fmt.Printf("EVOLVING is the fifteen the policy actually holds (the POLICY squad).\n")
	fmt.Printf("`mkt` re-reads the FROZEN squad at market value, so FPL's\n")
	fmt.Printf("half-of-any-rise selling rule is not charged: mkt - chg is the\n")
	fmt.Printf("friction channel, and `tax` is what that rule costs in tenths.\n")
	fmt.Printf("⚠️  The two series answer different questions and are never pooled.\n")
	fmt.Printf("⚠️  Compare the arms on `chg`, never on `cost`: an arm that never\n")
	fmt.Printf("transfers accrues its allowance to the bank limit while one that does\n")
	fmt.Printf("spends it, so the same distance is priced against a different `free`.\n")
	fmt.Printf("⚠️  `decay` is head minus tail, so a NEGATIVE decay is a series that\n")
	fmt.Printf("grows. Churn predicts it positive.\n")
	fmt.Printf("⚠️  The first observed week is the entry week's squad in both arms, so\n")
	fmt.Printf("the two series start equal by construction and only diverge as the\n")
	fmt.Printf("policy transfers. That is the invariance check, not a result.\n\n")

	var rows []seriesRow
	for _, p := range pairs {
		for _, start := range starts {
			// `WeeklyXI` false, the sweep default, and here it is provably
			// irrelevant rather than a defaulted knob: it reaches only `ve`, the
			// engine the fielded eleven is picked on, and the repair cost is read
			// off `pe`, the engine the DECISION runs on. This package records the
			// opposite case as a trap — an arm about fixture quantity that leaves
			// it false has switched off half its own mechanism — so the reason it
			// does not bite here is stated rather than assumed.
			sc := sweepConfig(cfg, start, false)
			sc.RecordRepairCost = true
			res, err := Simulate(p.Cur, p.Prior, sc)
			if err != nil {
				fmt.Printf("%-9s GW%-3d infeasible: %v\n", p.Name, start, err)
				continue
			}
			rows = append(rows, reportRepairSeries(p.Name, start, res))
		}
	}
	summariseRepairSeries(rows)
}

// gridEnv names an environment switch's value, or says it is unset — so the stamp
// records what the run actually used rather than what a reader assumes.
func gridEnv(key, unset string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return unset
}

// seriesRow is one cell's two series, reduced to the shape statistics the two
// mechanisms disagree about.
//
// Every field is a count or a difference of counts. There is no standard error
// here and there must not be: this separates two mechanisms, and a threshold
// would claim it measured what either is worth.
type seriesRow struct {
	Season string
	Start  int
	// Frozen and Evolving are the change counts, gameweek by gameweek.
	Frozen, Evolving []int
	// Gross is the frozen count re-read at market value.
	Gross []int
	// Tax is the selling rule's bite on the frozen squad, in tenths.
	Tax []int
}

// reportRepairSeries prints one cell's table and returns its two series.
func reportRepairSeries(season string, start int, res *SimResult) seriesRow {
	row := seriesRow{Season: season, Start: start}
	fmt.Printf("--- %s, entry GW%d (%d observed gameweeks)\n",
		season, start, len(res.RepairSeries))
	fmt.Printf("%4s | %5s %4s %6s | %5s %4s %6s | %4s %6s\n",
		"gw", "chg", "free", "cost", "chg", "free", "cost", "mkt", "tax")
	fmt.Printf("%4s | %-18s | %-18s | %-11s\n", "", "EVOLVING", "FROZEN", "FROZEN at mkt")
	for _, r := range res.RepairSeries {
		if !r.OK || !r.FrozenOK || !r.FrozenGrossOK {
			// No reading is not a repair cost of zero, and the two license
			// opposite conclusions. Printed and excluded from the series.
			fmt.Printf("%4d | %s\n", r.GW, "no reading (a rebuild failed)")
			continue
		}
		fmt.Printf("%4d | %5d %4d %6.0f | %5d %4d %6.0f | %4d %6d\n",
			r.GW, r.Changes, r.Free, r.Cost,
			r.FrozenChanges, r.FrozenFree, r.FrozenCost,
			r.FrozenGrossChanges, r.FrozenGrossBudget-r.FrozenBudget)
		row.Evolving = append(row.Evolving, r.Changes)
		row.Frozen = append(row.Frozen, r.FrozenChanges)
		row.Gross = append(row.Gross, r.FrozenGrossChanges)
		row.Tax = append(row.Tax, r.FrozenGrossBudget-r.FrozenBudget)
	}
	fmt.Printf("  EVOLVING %s\n", shapeOf(row.Evolving))
	fmt.Printf("  FROZEN   %s\n", shapeOf(row.Frozen))
	fmt.Printf("  FROZEN at market  %s\n", shapeOf(row.Gross))
	fmt.Printf("\n")
	return row
}

// shape is a series reduced to what the two mechanisms disagree about.
type shape struct {
	First, Last, Min, Max int
	// Head and Tail are the means of the first and last thirds. A first-versus-last
	// comparison on an integer series is one draw against one draw; thirds are the
	// cheapest thing that is not.
	Head, Tail float64
	// Slope is the ordinary least-squares gradient per gameweek. It is a
	// description of the series, not an estimate of anything: there is no standard
	// error attached and none should be derived.
	Slope float64
	// Monotone is +1 for non-decreasing, -1 for non-increasing, 0 for neither.
	Monotone int
}

// Floor is the level a decaying series would settle at, read as the mean of the
// last third — the quantity a standing gap predicts is non-zero.
func (s shape) Floor() float64 { return s.Tail }

// Decay is how much of the opening level is gone by the end: head minus tail. The
// quantity churn predicts is positive and a standing gap predicts is zero. Reported
// BESIDE the floor rather than instead of it, because both is a legitimate answer.
func (s shape) Decay() float64 { return s.Head - s.Tail }

func shapeOf(v []int) string {
	s := shapeStats(v)
	if len(v) == 0 {
		return "no readings"
	}
	dir := "neither"
	switch s.Monotone {
	case 1:
		dir = "non-decreasing"
	case -1:
		dir = "non-increasing"
	}
	return fmt.Sprintf("first %d last %d min %d max %d | head %.2f tail %.2f "+
		"floor(tail) %.2f decay(head-tail) %+.2f slope %+.4f/gw | %s",
		s.First, s.Last, s.Min, s.Max, s.Head, s.Tail, s.Floor(), s.Decay(),
		s.Slope, dir)
}

func shapeStats(v []int) shape {
	var s shape
	if len(v) == 0 {
		return s
	}
	s.First, s.Last = v[0], v[len(v)-1]
	s.Min, s.Max = v[0], v[0]
	up, down := true, true
	for i, x := range v {
		if x < s.Min {
			s.Min = x
		}
		if x > s.Max {
			s.Max = x
		}
		if i > 0 {
			if x < v[i-1] {
				up = false
			}
			if x > v[i-1] {
				down = false
			}
		}
	}
	switch {
	case up:
		s.Monotone = 1
	case down:
		s.Monotone = -1
	}
	// Thirds, with the middle left out so the two ends do not share observations.
	third := len(v) / 3
	if third < 1 {
		third = 1
	}
	s.Head = meanInts(v[:third])
	s.Tail = meanInts(v[len(v)-third:])
	s.Slope = olsSlope(v)
	return s
}

func meanInts(v []int) float64 {
	if len(v) == 0 {
		return 0
	}
	sum := 0
	for _, x := range v {
		sum += x
	}
	return float64(sum) / float64(len(v))
}

// olsSlope is the least-squares gradient of a series against its own index — a
// one-line description of which way it goes, with no inference attached.
func olsSlope(v []int) float64 {
	n := float64(len(v))
	if n < 2 {
		return 0
	}
	var sx, sy, sxy, sxx float64
	for i, y := range v {
		x := float64(i)
		sx += x
		sy += float64(y)
		sxy += x * float64(y)
		sxx += x * x
	}
	den := n*sxx - sx*sx
	if den == 0 {
		return 0
	}
	return (n*sxy - sx*sy) / den
}

// summariseRepairSeries prints the reading rule and then the two series' shape
// statistics side by side, per entry point.
//
// Split by entry point rather than pooled, because the entry point IS the
// information regime the two mechanisms disagree about: a GW1 cell opens with one
// gameweek of data and a GW16 cell with fifteen, so churn predicts a difference
// between the columns and a standing gap predicts none.
func summariseRepairSeries(rows []seriesRow) {
	fmt.Printf("=== reading rule, written before the run\n")
	fmt.Printf("CHURN      both series decline: decay > 0 and floor small, in both arms,\n")
	fmt.Printf("           and the GW1 column decays harder than the mid-season one.\n")
	fmt.Printf("STANDING   EVOLVING flat and non-zero (decay ~ 0, floor > 0) while\n")
	fmt.Printf("GAP        FROZEN rises (slope > 0).\n")
	fmt.Printf("BOTH       a positive decay sitting on a positive floor. Report the two\n")
	fmt.Printf("           separately rather than choosing.\n")
	fmt.Printf("           ⚠️ BOTH is the EXPECTED reading, not a third mechanism: any\n")
	fmt.Printf("           standing gap plus any drift composes into a floor plus a\n")
	fmt.Printf("           decay. The verdict is the two sizes, never the category.\n")
	fmt.Printf("FRICTION   if FROZEN at market has a materially smaller slope than\n")
	fmt.Printf("           FROZEN, the selling rule is implicated in the frozen rise.\n")
	fmt.Printf("           The gap is a bound, not a subtraction — see the header.\n\n")

	starts := map[int]bool{}
	for _, r := range rows {
		starts[r.Start] = true
	}
	var order []int
	for s := range starts {
		order = append(order, s)
	}
	sort.Ints(order)

	fmt.Printf("%-9s %-5s %-10s %8s %8s %8s %10s\n",
		"season", "entry", "series", "floor", "decay", "slope", "monotone")
	for _, start := range order {
		for _, r := range rows {
			if r.Start != start {
				continue
			}
			for _, s := range []struct {
				name string
				v    []int
			}{
				{"EVOLVING", r.Evolving}, {"FROZEN", r.Frozen}, {"FROZEN mkt", r.Gross},
			} {
				st := shapeStats(s.v)
				fmt.Printf("%-9s GW%-3d %-10s %8.2f %8.2f %8.4f %10d\n",
					r.Season, r.Start, s.name, st.Floor(), st.Decay(), st.Slope,
					st.Monotone)
			}
		}
	}

	fmt.Printf("\n=== per entry point, averaged over seasons\n")
	fmt.Printf("%-6s %-10s %8s %8s %8s %6s\n",
		"entry", "series", "floor", "decay", "slope", "cells")
	for _, start := range order {
		for _, s := range []struct {
			name string
			pick func(seriesRow) []int
		}{
			{"EVOLVING", func(r seriesRow) []int { return r.Evolving }},
			{"FROZEN", func(r seriesRow) []int { return r.Frozen }},
			{"FROZEN mkt", func(r seriesRow) []int { return r.Gross }},
		} {
			var floor, decay, slope float64
			n := 0
			for _, r := range rows {
				if r.Start != start || len(s.pick(r)) == 0 {
					continue
				}
				st := shapeStats(s.pick(r))
				floor += st.Floor()
				decay += st.Decay()
				slope += st.Slope
				n++
			}
			if n == 0 {
				continue
			}
			fmt.Printf("%-6d %-10s %8.2f %8.2f %8.4f %6d\n",
				start, s.name, floor/float64(n), decay/float64(n),
				slope/float64(n), n)
		}
	}
	fmt.Printf("\n⚠️  Counts and shapes only. No points figure, no detection threshold\n")
	fmt.Printf("and no p-value is printed, and none may be derived: this separates two\n")
	fmt.Printf("mechanisms and measures what neither is worth.\n")
}
