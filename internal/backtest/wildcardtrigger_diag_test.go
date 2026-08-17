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
// Run 2026-08-17 on four seasons at entry GW1 and GW16, reservations 0/4/8/12/20:
// at the shipped reservation of 12 it fires in the cell's SECOND week in **8 of 8
// cells** — GW2 from a GW1 entry, GW17 from a GW16 entry — with repair costs of
// 20/36/24/32 points at GW1 and 12/12/12/4 at GW16. It weighed exactly one week in
// seven of the eight, because it fires the first time it is asked. Only the
// reservation of 20 moves anything, and only on the GW16 cells, by one to three
// weeks.
//
// **So the closure stands, and the reason transports after all.** The repair cost
// IS a magnitude rather than a move count — that part of the argument was right —
// but the magnitude it measures is **model churn, not squad decay**. One gameweek
// after the opening fifteen is bought at the model's own optimum, that optimum has
// moved by five to nine players, because the engine has re-scored everyone with one
// more round of data. So the cost is large in exactly the week the record warns
// about, for exactly the reason it warns about: the model has least information
// then, and the trigger reads the resulting instability as a broken squad.
//
// ⚠️ **Do not "fix" this by raising the reservation.** The 20-point column shows
// what that buys: the rule still fires within four weeks, because the cost it is
// measuring does not fall. What would have to change is the QUANTITY — a repair
// cost measured against a squad the model still endorses, rather than against a
// fresh argmax over the whole pool. That is a different lever and it is unbuilt.
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

	fmt.Printf("\n=== the wildcard state trigger: does it fire, and when\n")
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
		for _, start := range sweepStarts() {
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
	fmt.Printf("%d of %d cells. That is the recorded defect's signature — a rule that\n", early, total)
	fmt.Printf("fires immediately is reading transfer scarcity, not squad quality.\n")
	fmt.Printf("A high count here does NOT reopen the line; it confirms the closure.\n")
}
