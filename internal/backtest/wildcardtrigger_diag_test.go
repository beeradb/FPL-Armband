package backtest

// The wildcard state trigger's falsifier, and it is the whole deliverable of that
// lever.
//
//	DIAG=1 go test ./internal/backtest -run TestDiagWildcardTrigger -v -timeout 2h
//
// # Why this is a decision count and not a points figure
//
// **The replay cannot value a wildcard.** It replaces all fifteen, so the
// within-season spread swamps the chip, and this record already says a wildcard
// replay must not be read as a valuation. So the question a wildcard arm can
// answer is: *does the rule fire, and when.*
//
// # The falsifier
//
// "Do not build a state trigger for the wildcard" is a closed line, and its stated
// reason is that the tested trigger — the literal reading of *"cannot fix it with
// free transfers"* — **measures transfer scarcity rather than squad quality, so it
// fires at GW2 when the model has least data**. A repair cost priced in POINTS is
// a magnitude rather than a scarcity proxy: it is `4 x max(0, changes - free)`, so
// a squad the free transfers can reach costs nothing to repair however many moves
// it takes.
//
// That was an argument. **It is refuted. The trigger still fires immediately.**
//
// Run 2026-08-17 on four seasons at entry GW1 and GW16, reservations 0/4/8/12/20.
// At the shipped reservation of 12 it fires in the cell's SECOND week in **8 of 8
// cells** — GW2 from a GW1 entry, GW17 from a GW16 entry — and **weighed exactly
// one week in 8 of 8**, because it fires the first time it is asked. Only the
// reservation of 20 moves anything, and only on the GW16 cells, by one to three
// weeks.
//
// The repair costs are **20/36/24/32 points at a GW1 entry** and **12/12/12/4 at a
// GW16 entry**. ⚠️ **Those are POINTS, not player counts.** The cost is
// `4 x max(0, changes - free)` and `free` is 2 at the firing week — the weekly
// accrual runs before the search — so the implied number of players the model would
// replace is `cost/4 + 2`: **7, 11, 8 and 10** at GW1 and **5, 5, 5 and 3** at GW16.
// An earlier draft quoted "five to nine players", which was the HIT count read as a
// player count, and quoted the GW1 column as if it were general when the mid-season
// column is roughly half the size. Both corrected in review.
//
// **The closure stands. The mechanism is a HYPOTHESIS and this diagnostic cannot
// establish it.**
//
// The repair cost IS a magnitude rather than a move count — that part of the
// original argument was right, and it is not what fails. What fails is that the
// magnitude is large from the first week it is measured, and there are two readings
// of why:
//
//   - **Churn.** The model has just re-scored everyone with one more round of data,
//     so the optimum has moved a long way from the fifteen bought last week. This is
//     a RATE, and it predicts the cost falling as the season settles.
//   - **A standing gap.** Any held fifteen sits some distance from a fresh
//     unconstrained argmax over the whole pool, because the held squad is a
//     constrained solution to last week's problem and the argmax is not. This is a
//     LEVEL, non-zero at every cutoff, and it predicts the cost staying put.
//
// ⚠️ **This diagnostic cannot separate them, by construction**: the rule fires on
// first consultation and then stops, so the cost is never observed as a series on a
// fixed squad. The only place it is seen twice is the bar-20 arm, and there it goes
// **12 → 16 over four weeks and 12 → 24 over two** — flat-to-rising, which is the
// standing-gap signature rather than the churn one. And the GW16 cells fire at GW17
// with fifteen gameweeks of data behind them, so information poverty cannot be the
// operative mechanism for half the grid.
//
// The reading that survives both is the weaker and sufficient one: **the quantity
// is dominated by held-versus-fresh distance rather than by squad quality**, which
// is what the recorded closure says a wildcard trigger must not measure.
//
// ⚠️ **Do not "fix" this by raising the reservation.** The 20-point column shows
// what that buys: the rule still fires within four weeks, because the cost it is
// measuring does not fall. What would have to change is the QUANTITY — a repair
// cost measured against a squad the model still endorses, rather than against a
// fresh argmax over the whole pool. Note that this is the standing-gap reading
// written as a prescription, which is why the two must agree.
//
// The lever ships off and stays off. This diagnostic remains so the next session
// re-runs it rather than re-deriving the argument.
//
// ⚠️ **A reservation of 0 is a positive control, not a setting.** It fires on the
// first positive repair cost, which is early by construction — so an early week in
// that column says the rule is wired, and says nothing about the shipped bar.
//
// ⚠️ **The repair cost costs a full `Optimize` every eligible week**, which is the
// expensive call in this package. That is affordable only because the lever ships
// off.

import (
	"fmt"
	"os"
	"testing"
)

func TestDiagWildcardTrigger(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)
	pairs := loadPairs(t, cfg)
	starts := sweepStarts()

	fmt.Printf("\n=== the wildcard state trigger: does it fire, and when\n")
	// The grid, stamped rather than described in prose. The recorded result is a
	// FOUR-season, two-entry-point figure and needs
	// `FPL_SWEEP_SEASONS=default FPL_SWEEP_STARTS=1,16` — at the shipped default
	// `sweepPairNames` returns SIX seasons including 2019-20, whose `POLICY` path
	// is not a sample of the same process (FPL granted unlimited free transfers
	// before the GW30+ deadline). A reader running this bare would get a different
	// grid under the same heading, which is how "8 of 8" stops meaning anything.
	fmt.Printf("%s\n", gridLabel(len(pairs), len(starts)))
	for _, p := range pairs {
		if !TransferPathComparable(p.Name) {
			fmt.Printf("⚠️  %s is in this grid and its transfer path is NOT "+
				"comparable — read its row as a decision count only.\n", p.Name)
		}
	}
	fmt.Printf("Repair cost is 4 x max(0, changes - free), where `changes` is how many\n")
	fmt.Printf("of the fifteen the model's own optimum would replace. It is compared\n")
	fmt.Printf("against a reservation that decays to exactly 0 in the last week the\n")
	fmt.Printf("chip could be played, so late in the window any repair cost fires.\n")
	fmt.Printf("\n⚠️  The FALSIFIER: the recorded closure of this line rests on the\n")
	fmt.Printf("tested trigger firing at GW2, when the model has least data. If the\n")
	fmt.Printf("repair-cost trigger fires at GW2 too, it has the same defect and the\n")
	fmt.Printf("closure stands. Read the `bar 0` column as a positive control only.\n")
	fmt.Printf("\n⚠️  No points figure is printed and none should be derived: the replay\n")
	fmt.Printf("cannot value a wildcard.\n\n")

	bars := []float64{0, 4, 8, 12, 20}
	fmt.Printf("%-9s %-6s", "season", "entry")
	for _, b := range bars {
		fmt.Printf(" | bar %-4.0f gw/cost/weighed", b)
	}
	fmt.Printf("\n")

	early := 0
	total := 0
	for _, p := range pairs {
		for _, start := range starts {
			sc := sweepConfig(cfg, start, false)
			fmt.Printf("%-9s GW%-4d", p.Name, start)
			for _, b := range bars {
				arm := sc
				arm.WildcardTrigger = true
				arm.WildcardReservation = b
				res, err := Simulate(p.Cur, p.Prior, arm)
				if err != nil {
					fmt.Printf(" | %-24s", "infeasible")
					continue
				}
				m := res.Wildcard
				fmt.Printf(" | %2d %8.1f %10d", m.FiredGW, m.FiredValue, m.WeighedWeeks)
				// The falsifier's own counter, at the shipped bar alone. A
				// reservation of 0 is a control and is deliberately excluded:
				// counting it would report the control's earliness as the rule's.
				if b == 12 {
					total++
					if m.FiredGW > 0 && m.FiredGW <= start+1 {
						early++
					}
				}
			}
			fmt.Printf("\n")
		}
	}
	fmt.Printf("\nat the shipped reservation of 12: fired in the cell's SECOND week in\n")
	fmt.Printf("%d of %d cells, on %s.\n", early, total, gridLabel(len(pairs), len(starts)))
	fmt.Printf("That is the recorded defect's signature — a rule that\n")
	fmt.Printf("fires immediately is reading transfer scarcity, not squad quality.\n")
	fmt.Printf("A high count here does NOT reopen the line; it confirms the closure.\n")
}
