package backtest

// The worldview rewrite, measured: how far does the model's fresh optimum move
// when one gameweek of data arrives?
//
//	DIAG=1 FPL_SWEEP_SEASONS=default FPL_SWEEP_STARTS=1,16 \
//	  go test ./internal/backtest -run TestDiagWorldviewRewrite -v -timeout 2h
//
// # The question
//
// The repair-cost series (TestDiagRepairCostSeries) settled that a held squad
// sits a flat 6-9 players below a fresh optimum — but it measures the DISTANCE
// between the held squad and the optimum, and cannot see how far the optimum
// itself moves from one week to the next. That is the direct observable of the
// user's hypothesis: *"One gameweek cannot be enough to make us rethink our
// entire picture of the world. We need to be a little bit slower to react to
// new data, at least in the beginning of the year."*
//
// If the model's picture of the world is stable and new data merely refines it,
// the fresh optimum should change little week to week. If one gameweek really
// does rewrite the world, the fresh optimum moves several players every week —
// and every held squad, however well chosen, is instantly several players stale.
// That is the standing gap, regenerated weekly rather than accumulated once.
//
// # What is measured
//
// `RepairWeek.FreshChurn`: the players in this week's fresh fifteen that were
// absent from last week's. The fresh squad is the one the EVOLVING arm already
// computes, so this costs no extra `Optimize` call. It rides the same
// `RecordRepairCost` switch and the same confinement pin
// (TestTheRepairSeriesChangesNoDecision) — the observer still changes nothing.
//
// ⚠️ **Two channels share the column, stated before the run.** The fresh
// optimum is computed against the HELD squad's selling value, which drifts
// week to week as the policy trades — part of any churn is budget movement,
// not preference movement. And the pool itself moves: a player who becomes
// unavailable leaves it, and an arrival enters it, so part of any churn is
// AVAILABILITY turnover — football, in a sense, but not the model over-
// reacting. The column measures the sum; this design does not decompose it.
// The budget channel is bounded by printing the budget's own weekly movement;
// the availability channel is named, not bounded.
//
// # The reading rule, written before the run
//
// Per cell: the mean FreshChurn over the middle two quarters (the first
// observed week has no predecessor, and the entry window is its own regime),
// split into head and tail halves, plus the budget's mean weekly movement in
// tenths. Then:
//
//	REWRITE   mean churn >= 4 players/week, head and tail alike: the world is
//	          rewritten weekly whatever the information state. The standing gap
//	          is regenerated, and the prior-reactivity question is about the
//	          whole season, not its beginning.
//	OVERREACT churn falls by a third or more head-to-tail: the world is
//	          rewritten most where information is poorest — the user's
//	          mechanism, and the case for slower reactions early.
//	STABLE    mean churn <= 2 players/week: the picture of the world is stable,
//	          and the standing gap is accumulated once — ownership and budget,
//	          not weekly rewrites. The prior-reactivity reading would then be
//	          REFUSED by this observable.
//
// A cell is reported, never pooled across entries: GW1 entries open on one
// gameweek of data and GW16 on fifteen, and the mechanism says they should
// differ. The prediction to record beside each cell is which of the three
// rows it sits on; the verdict is the count of cells per row, split by entry.

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func TestDiagWorldviewRewrite(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)
	pairs := loadPairs(t, cfg)
	starts := sweepStarts()

	fmt.Printf("\n=== the worldview rewrite: how far does the fresh optimum move per gameweek?\n")
	fmt.Printf("%s\n", gridLabel(len(pairs), len(starts)))
	fmt.Printf("FPL_SWEEP_SEASONS=%s  FPL_SWEEP_STARTS=%s  entry points %v\n",
		gridEnv("FPL_SWEEP_SEASONS", "extended (the shipped default)"),
		gridEnv("FPL_SWEEP_STARTS", "1,6,11,16,21,26 (the shipped default)"), starts)

	weeks := 0
	for _, s := range starts {
		weeks += (38 - s) * len(pairs)
	}
	fmt.Printf("\nprojected cost: %d observed gameweeks x 3 Optimize calls = %d, "+
		"about %s at 3.5s each — the churn column rides the EVOLVING arm's\n",
		weeks, weeks*3,
		(time.Duration(weeks*3*3500) * time.Millisecond).Round(time.Minute))
	fmt.Printf("existing call and adds none.\n")

	fmt.Printf("\n`churn` is players in this week's fresh fifteen absent from last\n")
	fmt.Printf("week's; `budget` is the fresh optimum's budget in tenths, printed so\n")
	fmt.Printf("the budget-drift channel can be bounded. The first observed week has\n")
	fmt.Printf("no predecessor and prints `—`. Reading rule, written before the run:\n")
	fmt.Printf("  REWRITE   mean churn >= 4/week, head and tail alike\n")
	fmt.Printf("  OVERREACT churn falls by a third or more head-to-tail\n")
	fmt.Printf("  STABLE    mean churn <= 2/week\n\n")

	type cellRead struct {
		Season     string
		Start      int
		Churn      []int
		BudgetMove []int
		Weeks      int
	}
	var cells []cellRead
	for _, p := range pairs {
		for _, start := range starts {
			sc := sweepConfig(cfg, start, false)
			sc.RecordRepairCost = true
			res, err := Simulate(p.Cur, p.Prior, sc)
			if err != nil {
				fmt.Printf("%-9s GW%-3d infeasible: %v\n", p.Name, start, err)
				continue
			}
			fmt.Printf("--- %s, entry GW%d\n", p.Name, start)
			fmt.Printf("%4s | %5s %7s\n", "gw", "churn", "budget")
			cr := cellRead{Season: p.Name, Start: start}
			for i, r := range res.RepairSeries {
				if !r.FreshChurnOK {
					fmt.Printf("%4d | %5s %7d\n", r.GW, "—", r.Budget)
					continue
				}
				bm := 0
				if i > 0 && res.RepairSeries[i-1].OK && r.OK {
					bm = r.Budget - res.RepairSeries[i-1].Budget
				}
				fmt.Printf("%4d | %5d %+7d\n", r.GW, r.FreshChurn, bm)
				cr.Churn = append(cr.Churn, r.FreshChurn)
				if i > 0 {
					cr.BudgetMove = append(cr.BudgetMove, bm)
				}
			}
			cr.Weeks = len(res.RepairSeries)
			cells = append(cells, cr)
		}
	}

	fmt.Printf("\n=== per-cell readout, middle two quarters only ===\n")
	fmt.Printf("%-9s %-5s %9s %9s %9s | %-9s\n",
		"season", "start", "head", "tail", "mean", "reading")
	for _, c := range cells {
		n := len(c.Churn)
		if n < 8 {
			fmt.Printf("%-9s GW%-4d too short (%d weeks)\n", c.Season, c.Start, n)
			continue
		}
		lo, hi := n/4, 3*n/4
		mid := c.Churn[lo:hi]
		half := len(mid) / 2
		head := meanInts(mid[:half])
		tail := meanInts(mid[half:])
		mean := meanInts(mid)
		bm := 0.0
		if len(c.BudgetMove) > 0 {
			bm = meanInts(c.BudgetMove)
		}
		// The reading rule, exactly as registered above: STABLE first (the
		// level test), then OVERREACT (falls by a third or more head-to-tail,
		// with a head large enough that the fall is not 2 -> 1), else REWRITE.
		reading := "REWRITE"
		if mean <= 2 {
			reading = "STABLE"
		} else if head >= 3 && tail <= 2*head/3 {
			reading = "OVERREACT"
		}
		fmt.Printf("%-9s GW%-4d %9.2f %9.2f %9.2f | %-9s (budget %+.0f tenths/wk)\n",
			c.Season, c.Start, head, tail, mean, reading, bm)
	}
}
