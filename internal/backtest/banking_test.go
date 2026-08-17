package backtest

// Does the policy ever bank transfers, and can it reach a premium if it does?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagBanking -v -timeout 30m
//
// A premium upgrade usually needs more than one move: the money is locked in a
// player of a different position, so buying him means selling a forward *and*
// funding the difference from somewhere else. RankPairs can express that — one
// upgrade against several sales — but only if there are transfers in hand to
// spend, and the weekly decision is greedy: it spends a free transfer the moment
// any move clears the gate.
//
// So the question is not whether a three-way swap is *expressible*. It is
// whether the policy ever accumulates the transfers such a swap would need.

import (
	"fmt"
	"os"
	"testing"

	"armband/internal/analysis"
)

// bankingOf is the cells file's transfer-banking block, and the ONLY place a
// SimResult is turned into one.
//
// # The pre-registered liveness rule this exists to serve
//
// **Any banking arm whose banked_weeks is 0 everywhere is a comparison that
// never ran, and its deliverable is the mediator count, not a null.** The
// recorded verdict that the policy never banks a transfer was reached with
// nothing counting whether shouldBank ever fired, so it could not be told apart
// from an arm that was wired and unreachable — and this record's standing rule
// is that a byte-identical result is not a tie until its mediator has been
// checked.
//
// One function rather than a few lines at each construction site, on the rule
// this package is named for: two expressions of one quantity end with the
// measured one not being the one that runs. chipReadingsOf is the same shape for
// the same reason.
//
// It gates on DecisionWeeks rather than on a flag, so a SimResult assembled by
// hand — the variance decomposition builds cellRows from one — reports the block
// as absent instead of reporting a season that decided nothing.
func bankingOf(res *SimResult) (BankingMediator, bool) {
	if res == nil || res.Banking.DecisionWeeks <= 0 {
		return BankingMediator{}, false
	}
	return res.Banking, true
}

// TestTheBankingMediatorIsCountedOnEveryDecisionWeek is the liveness guard for
// the two banking columns, and it is deliberately not a guard on their values.
//
// # What it refuses
//
// The failure being designed against is silence: `decide` stops returning its
// weekBanking, or Simulate stops accumulating it, or a guard appears in front of
// shouldBank — and every one of those leaves a banking arm reporting zero banked
// weeks, which reads exactly like the recorded verdict it is supposed to be
// testing. Nothing errors and the number is plausible, which is the shape this
// package has shipped several times.
//
// So the assertion is **the rule was consulted on every decision week**, which is
// a property of the wiring rather than of the football. It fails on all three
// failures above and cannot rot with the data.
//
// # What it must NOT assert
//
// That banking ever fires. Whether it does is the finding the column exists to
// report, on a metric and a grid this test has neither — pinning it here would
// bake one season's answer into the gate, and a test pinned to what the data did
// rots within days.
func TestTheBankingMediatorIsCountedOnEveryDecisionWeek(t *testing.T) {
	cur, prior, base := chipSim(t)

	// Chips are off by default, so every gameweek after the first reaches
	// `decide` — TestChipsAreOffByDefault is what makes that safe to assume.
	greedy, err := Simulate(cur, prior, base)
	if err != nil {
		t.Fatal(err)
	}
	lookaheadCfg := base
	lookaheadCfg.BankLookahead = true
	lookahead, err := Simulate(cur, prior, lookaheadCfg)
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		name string
		res  *SimResult
		on   bool
	}{{"greedy", greedy, false}, {"bank lookahead", lookahead, true}} {
		b := c.res.Banking
		if want := len(c.res.Weeks) - 1; b.DecisionWeeks != want {
			t.Errorf("%s: %d decision weeks over %d played, want %d — every week "+
				"but the first reaches decide when no chip is planned",
				c.name, b.DecisionWeeks, len(c.res.Weeks), want)
		}
		// The arm's own switch, read back off what ran. Zero consulted weeks in a
		// banking arm is the exact silence this test exists for.
		switch {
		case c.on && b.ConsultedWeeks != b.DecisionWeeks:
			t.Errorf("%s: shouldBank was asked in %d of %d decision weeks — a "+
				"banking arm that never reaches the rule reports the same clean "+
				"null as one where the rule never fires",
				c.name, b.ConsultedWeeks, b.DecisionWeeks)
		case !c.on && b.ConsultedWeeks != 0:
			t.Errorf("%s: shouldBank is only reachable behind BankLookahead, and "+
				"this arm reports %d consulted weeks", c.name, b.ConsultedWeeks)
		}
		if b.BankedWeeks > b.ConsultedWeeks {
			t.Errorf("%s: %d banked weeks out of %d consulted",
				c.name, b.BankedWeeks, b.ConsultedWeeks)
		}
		// The allowance column, on the two bounds that hold whatever the season
		// did: the accrual runs before the search, so every decision week holds at
		// least one free transfer, and nothing may exceed the bank ceiling.
		if b.FreeHeld < b.DecisionWeeks {
			t.Errorf("%s: %d free transfers summed over %d decision weeks — the "+
				"weekly accrual runs before the search, so each is at least 1",
				c.name, b.FreeHeld, b.DecisionWeeks)
		}
		if m := b.MeanFreeAtDecision(); m > float64(base.BankUpTo) {
			t.Errorf("%s: mean allowance %v exceeds the bank ceiling of %d",
				c.name, m, base.BankUpTo)
		}
		// And the cells file's block is derived from it rather than recomputed.
		if got, ok := bankingOf(c.res); !ok || got != b {
			t.Errorf("%s: bankingOf returned (%+v, %v), want (%+v, true)",
				c.name, got, ok, b)
		}
	}
}

// TestAChippedWeekIsNotADecisionWeek pins free_at_decision's denominator.
//
// A wildcard replaces the whole squad and a free hit lends one for a week, and
// neither runs the transfer decision at all — so counting those weeks would
// divide the summed allowance by more weeks than were ever decided, and would
// understate it by however many chips the arm played. The banking column would
// then move with an arm's chip plan for a reason that has nothing to do with
// banking.
func TestAChippedWeekIsNotADecisionWeek(t *testing.T) {
	cur, prior, base := chipSim(t)

	plain, err := Simulate(cur, prior, base)
	if err != nil {
		t.Fatal(err)
	}
	// One chip of each kind, both in the first set. chipSim replays 2025-26,
	// where FPL resets the chips at GW19, so both must sit before it.
	chipped := base
	chipped.Chips = analysis.ChipPlan{Wildcard: 8, FreeHit: 12}
	got, err := Simulate(cur, prior, chipped)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Weeks) != len(plain.Weeks) {
		t.Fatalf("the chipped arm played %d weeks and the plain one %d; this test "+
			"compares decision counts and needs the same season",
			len(got.Weeks), len(plain.Weeks))
	}
	if want := plain.Banking.DecisionWeeks - 2; got.Banking.DecisionWeeks != want {
		t.Errorf("%d decision weeks with a wildcard and a free hit played, want "+
			"%d — a chipped week makes no transfer decision and must not enter "+
			"free_at_decision's denominator", got.Banking.DecisionWeeks, want)
	}
}

func TestDiagBanking(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)

	pairs := sweepPairNames()

	freeHist := map[int]int{}
	movesHist := map[int]int{}
	weeks, totalMoves := 0, 0

	for _, pair := range pairs {
		prior := loadSeason(t, cfg, pair[0])
		cur := loadSeason(t, cfg, pair[1])
		sc := sweepConfig(cfg, 1, true)
		res, err := Simulate(cur, prior, sc)
		if err != nil {
			t.Fatal(err)
		}
		for _, w := range res.Weeks {
			freeHist[w.Free]++
			weeks++
		}
		byGW := map[int]int{}
		for _, mv := range res.Moves {
			byGW[mv.GW]++
		}
		for _, n := range byGW {
			movesHist[n]++
			totalMoves += n
		}
	}

	fmt.Printf("\nFree transfers in hand when each weekly decision was made,\n")
	fmt.Printf("%s from GW1, bank limit %d:\n\n", seasonsLabel(len(pairs)), sweepBankLimit)
	fmt.Printf("%-14s %8s %8s\n", "in hand", "weeks", "share")
	for f := 0; f <= sweepBankLimit; f++ {
		if freeHist[f] == 0 {
			continue
		}
		fmt.Printf("%-14d %8d %7.0f%%\n", f, freeHist[f],
			100*float64(freeHist[f])/float64(weeks))
	}

	fmt.Printf("\nMoves made in one gameweek, when any were made:\n\n")
	fmt.Printf("%-14s %8s %8s\n", "moves", "weeks", "share")
	var multi int
	for n := 1; n <= 6; n++ {
		if movesHist[n] == 0 {
			continue
		}
		var tot int
		for k := 1; k <= 6; k++ {
			tot += movesHist[k]
		}
		if n >= 3 {
			multi += movesHist[n]
		}
		fmt.Printf("%-14d %8d %7.0f%%\n", n, movesHist[n],
			100*float64(movesHist[n])/float64(tot))
	}
	fmt.Printf("\nweeks with three or more moves — the shape a premium upgrade needs: %d\n", multi)
	fmt.Printf("\nIf the bank is almost always 1, the policy is spending every transfer as\n")
	fmt.Printf("soon as any move clears the gate, and a three-way swap can never be\n")
	fmt.Printf("assembled however well RankPairs could express one.\n")
}
