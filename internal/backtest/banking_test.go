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
	"context"
	"fmt"
	"path/filepath"
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
// The reader's half of that rule, including the correction to its own wording
// and the dose bar that decides when an arm is unmeasurable rather than null,
// is in stats/README.md under "The transfer-banking mediator". It is stated
// there rather than a fourth time here, because nothing checks prose for drift
// and this sentence already appears in three files.
//
// One function rather than a few lines at each construction site, on the rule
// this package is named for: two expressions of one quantity end with the
// measured one not being the one that runs. chipReadingsOf is the same shape for
// the same reason.
//
// It gates on DecisionWeeks rather than on a flag, so a SimResult assembled by
// hand reports the block as absent instead of reporting a season that decided
// nothing. ⚠️ That is the gate's justification and not a description of the
// variance decomposition, which builds its rows from a real Simulate result and
// simply never calls this — its blanks come from HasBanking defaulting false.
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

// TestTheSweepWritesTheBankingBlock closes the join nothing else covers.
//
// The liveness test above covers decide → Simulate → SimResult. The CSV tests
// cover a hand-built cellRow → file. **The one line that connects them —
// `row.BankingMediator, row.HasBanking = bankingOf(res)` in runPolicySweep — was
// covered by neither**, and a review proved it: deleting that line and running
// the whole package returned ok. Both columns would then be blank in every cell
// of every sweep for ever, and the sweep would still print and bank normally.
//
// That is the exact silent no-op this block exists to detect, arriving in the
// block's own wiring. runPolicySweep is reachable only under DIAG, so this
// asserts the join at the level a gate test can afford: a real SimResult put
// through bankingOf and the sink, which is what the sweep does with it.
func TestTheSweepWritesTheBankingBlock(t *testing.T) {
	cur, prior, base := chipSim(t)
	base.BankLookahead = true
	res, err := Simulate(cur, prior, base)
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "cells.csv")
	sink, err := openCellSink(path)
	if err != nil {
		t.Fatal(err)
	}
	row := cellRow{
		Sweep: sink.sweepLabel("T"), RunID: sink.run(), Variant: "bank lookahead",
		Season: "2025-26", PriorSeason: "2024-25", StartGW: 1, Weeks: len(res.Weeks),
	}.under(base.Oracles)
	row.BankingMediator, row.HasBanking = bankingOf(res)
	sink.cell(row)
	sink.close()

	_, rows := readCells(t, path)
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	r := rows[0]
	// Every column of the block populated from a real replayed season. A blank in
	// any of them is the join having gone silent.
	for _, col := range []string{
		"decision_weeks", "consulted_weeks", "weighed_weeks",
		"banked_weeks", "free_at_decision",
	} {
		if r[col] == "" {
			t.Errorf("%s is blank on a real banking arm — the sweep's cellRow is "+
				"not being filled from the SimResult, which is silent everywhere "+
				"else in this package", col)
		}
	}
	if got := atoiOrFail(t, r["decision_weeks"]); got != res.Banking.DecisionWeeks {
		t.Errorf("decision_weeks is %d and the season decided %d",
			got, res.Banking.DecisionWeeks)
	}
	if got := atoiOrFail(t, r["consulted_weeks"]); got != res.Banking.DecisionWeeks {
		t.Errorf("consulted_weeks is %d over %d decision weeks — a banking arm "+
			"consults the rule every week", got, res.Banking.DecisionWeeks)
	}
}

// TestTheBankingFunnelNests pins the invariant that makes a zero attributable.
//
// decision >= consulted >= weighed >= banked, on any row. Each step removes one
// explanation for a zero, and that reading only holds while each really is a
// subset of the one above it — a counter incremented in the wrong branch would
// break the nesting and quietly turn the block back into an ambiguous count.
func TestTheBankingFunnelNests(t *testing.T) {
	cur, prior, base := chipSim(t)
	// The third arm is one that actually banks. Without it the nesting is only
	// ever checked where the last step of the funnel is zero, which is the step
	// most likely to be incremented in the wrong branch.
	for _, c := range []struct {
		name string
		sc   SimConfig
	}{
		{"greedy", base},
		{"bank lookahead", func() SimConfig { s := base; s.BankLookahead = true; return s }()},
		{"bank lookahead, firing", bankingArm(base)},
	} {
		res, err := Simulate(cur, prior, c.sc)
		if err != nil {
			t.Fatal(err)
		}
		b := res.Banking
		if b.DecisionWeeks < b.ConsultedWeeks ||
			b.ConsultedWeeks < b.WeighedWeeks ||
			b.WeighedWeeks < b.BankedWeeks {
			t.Errorf("%s: funnel does not nest — decision %d, consulted %d, "+
				"weighed %d, banked %d", c.name, b.DecisionWeeks,
				b.ConsultedWeeks, b.WeighedWeeks, b.BankedWeeks)
		}
	}
}

// bankingArm is a SimConfig under which the banking rule actually fires.
//
// # Why one is needed, and why it is this one
//
// At shipped config the rule is consulted on all 37 decision weeks of a replayed
// season and banks **zero** times, so every test written against a shipped-config
// banking arm executes the guards, the comparison and the false branch — and
// never the banked branch, the early return, or the BankedWeeks increment. A
// review proved the cost: the accrual bug could be put straight back and all
// three of this file's guards still passed, because the loop one of them iterates
// has an unreachable condition.
//
// **MaxHits: 0 is the single lever changed**, and it is the mechanism rather than
// a fudge. `MoveLimit` is `free + hits`, so at the shipped one hit the now-arm
// already reaches 2 moves and the later arm 3 — the extra free transfer buys a
// capability only if the best package needs three moves, while the shorter
// horizon costs a flat fifth of the gain. With no hit allowance the now-arm is
// one move and the later arm two, so banking buys the paired
// downgrade-and-upgrade the rule exists to reach. Measured on 2025-26 from GW1:
// 5 banked weeks against 0 at the shipped setting.
func bankingArm(base SimConfig) SimConfig {
	sc := base
	sc.BankLookahead = true
	sc.MaxHits = 0
	return sc
}

// TestTheBankingRuleActuallyFires is the liveness guard for the banked branch.
//
// Everything else about banking can be green while the branch has never run
// once. This is the test that executes it: the early return, the BankedWeeks
// increment, and the accrual behaviour the test below pins.
//
// It asserts a floor of one rather than an exact count. The count is a fact about
// the football and would rot; that the branch is reachable at all is a fact about
// the code and is what has to hold.
func TestTheBankingRuleActuallyFires(t *testing.T) {
	cfg := loadConfig(t)
	ctx := context.Background()

	// ⚠️ SEVERAL CELLS, and that is the point of this version. The previous one
	// was pinned to a single season and start, and it failed the first time a
	// scoring change perturbed that squad — not because banking stopped working
	// but because that one cell banked exactly ONCE and stopped being a
	// discriminating case.
	//
	// Measured at the time: banking separated in 17 of 18 cells, and the only
	// one it did not separate in was the fixture this test was pinned to. A
	// liveness guard whose whole claim rests on one marginal cell is one squad
	// perturbation from failing, and re-pinning it to a luckier cell would only
	// relocate that fragility rather than remove it.
	cells := []struct {
		cur, prior string
		start      int
	}{
		{"2021-22", "2020-21", 1},
		{"2022-23", "2021-22", 1},
		{"2023-24", "2022-23", 1},
		{"2024-25", "2023-24", 6},
		{"2025-26", "2024-25", 1},
		{"2025-26", "2024-25", 6},
	}

	ran, fired, separated := 0, 0, 0
	for _, c := range cells {
		prior, err := Load(ctx, cfg.CacheDir, c.prior)
		if err != nil {
			continue
		}
		cur, err := Load(ctx, cfg.CacheDir, c.cur)
		if err != nil {
			continue
		}
		base := SimConfig{
			Weights: cfg.Weights, MinGain: cfg.Review.MinGainForTransfer,
			MinGainHit: cfg.Review.MinGainForHit, BankUpTo: sweepBankLimit,
			MaxHits: cfg.Review.MaxHitsPerWeek, Budget: 1000,
			FreeCost: cfg.Review.FreeTransferValue, StartGW: c.start, WeeklyXI: true,
		}
		on := bankingArm(base)
		off := on
		off.BankLookahead = false

		res, err := Simulate(cur, prior, on)
		if err != nil {
			t.Fatalf("%s@%d: %v", c.cur, c.start, err)
		}
		plain, err := Simulate(cur, prior, off)
		if err != nil {
			t.Fatalf("%s@%d control: %v", c.cur, c.start, err)
		}
		// The control must not consult the rule at all, in every cell. This is a
		// fact about the code rather than the football, so it holds everywhere
		// and is asserted everywhere.
		if plain.Banking.ConsultedWeeks != 0 {
			t.Errorf("%s@%d: the control arm consulted the rule %d times",
				c.cur, c.start, plain.Banking.ConsultedWeeks)
		}
		ran++
		if res.Banking.BankedWeeks > 0 {
			fired++
		}
		if res.Points != plain.Points {
			separated++
		}
		t.Logf("%s@%-2d banked=%2d  on=%d off=%d", c.cur, c.start,
			res.Banking.BankedWeeks, res.Points, plain.Points)
	}
	if ran == 0 {
		t.Skip("archive unreachable for every cell")
	}

	// Reachability: the banked branch executes. Every other guard in this file
	// passes on an arm that never runs it, which is how the accrual bug survived
	// a suite that claimed to pin it.
	if fired == 0 {
		t.Errorf("the banking rule never banked in any of %d cells", ran)
	}
	// Liveness: the firing reaches the SCORED path, not merely a counter. A
	// confinement check on a path that cannot carry the effect confirms nothing.
	//
	// ⚠️ A majority rather than all, because an individual cell separating is a
	// fact about the football and will rot. That the rule moves seasons at all
	// is a fact about the code and is what has to hold. When this was written the
	// true rate was 17 of 18.
	if separated*2 < ran {
		t.Errorf("banking changed the season in only %d of %d cells — the rule is "+
			"reaching its counter without reaching the decision", separated, ran)
	}
}

// TestABankedWeekAccruesExactlyOneTransfer pins the arithmetic that was wrong.
//
// The banked branch of `decide` used to increment `free` a second time, on top
// of the weekly accrual, so a banked week ended two transfers up where FPL grants
// one. Fixed on correctness: FPL grants one a gameweek and the code granted two.
//
// ⚠️ **It runs on `bankingArm`, and that is the whole point.** Written against
// shipped config this test was vacuous — the rule banks zero times there, so the
// loop below iterated a condition that could never be met, and a review restored
// the deleted lines and watched it pass. A guard whose subject never executes is
// not a guard.
//
// Asserted as an invariant over the replay rather than as a unit test on
// `decide`, which needs an engine, a wallet and a season. The allowance may never
// rise by more than one across a gameweek, whatever the policy did, because one
// per gameweek is the whole of FPL's rule.
func TestABankedWeekAccruesExactlyOneTransfer(t *testing.T) {
	cur, prior, base := chipSim(t)
	res, err := Simulate(cur, prior, bankingArm(base))
	if err != nil {
		t.Fatal(err)
	}
	if res.Banking.BankedWeeks == 0 {
		t.Fatal("this arm banked nothing, so the branch under test never ran and " +
			"the loop below proves nothing — see TestTheBankingRuleActuallyFires")
	}
	moves := map[int]int{}
	for _, mv := range res.Moves {
		moves[mv.GW]++
	}
	for i := 1; i < len(res.Weeks); i++ {
		prev, wk := res.Weeks[i-1], res.Weeks[i]
		// Week.Free is what survived each decision, so a week that spent nothing
		// can only have accrued. A rise of two is the double grant.
		if moves[wk.GW] == 0 && wk.Free-prev.Free > 1 {
			t.Fatalf("GW%d made no transfer and the allowance rose from %d to %d — "+
				"FPL grants one free transfer a gameweek, so banking must decline "+
				"to spend rather than grant a second", wk.GW, prev.Free, wk.Free)
		}
	}
}

func TestDiagBanking(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)

	pairs := sweepPairNames()

	freeHist := map[int]int{}
	movesHist := map[int]int{}
	weeks, totalMoves := 0, 0
	heldSum, decisions := 0, 0

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
		// The mediator's own figure, beside the histogram rather than instead of
		// it. They are different quantities and the histogram was labelled as this
		// one for as long as it existed — see Week.Free.
		heldSum += res.Banking.FreeHeld
		decisions += res.Banking.DecisionWeeks
		byGW := map[int]int{}
		for _, mv := range res.Moves {
			byGW[mv.GW]++
		}
		for _, n := range byGW {
			movesHist[n]++
			totalMoves += n
		}
	}

	// ⚠️ **This histogram is Week.Free, which is what SURVIVED each decision** —
	// not what the search had. It said "in hand when the decision was made" and
	// that was wrong: `decide` spends before Week.Free is written, so a week that
	// made two moves is counted at what was left. It also includes the opening
	// week, which makes no decision at all. The allowance the search actually ran
	// with is printed underneath, from the mediator.
	//
	// Both are kept because both are real. The residue is what a manager carries
	// into next week; the mediator's figure is what this week could spend. The
	// greedy reading below — that the policy spends every transfer as soon as
	// anything clears the gate — is about the residue, and is only about the
	// residue.
	fmt.Printf("\nFree transfers left AFTER each weekly decision (Week.Free),\n")
	fmt.Printf("%s from GW1, bank limit %d:\n\n", seasonsLabel(len(pairs)), sweepBankLimit)
	fmt.Printf("%-14s %8s %8s\n", "left after", "weeks", "share")
	for f := 0; f <= sweepBankLimit; f++ {
		if freeHist[f] == 0 {
			continue
		}
		fmt.Printf("%-14d %8d %7.0f%%\n", f, freeHist[f],
			100*float64(freeHist[f])/float64(weeks))
	}

	if decisions > 0 {
		fmt.Printf("\nAllowance the search actually ran with, mean over %d decision\n", decisions)
		fmt.Printf("weeks (BankingMediator.FreeAtDecision): %.3f\n", float64(heldSum)/float64(decisions))
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
