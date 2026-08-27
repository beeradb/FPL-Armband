package backtest

// What is a perfect transfer gate worth, over the moves the model already
// proposes?
//
//	DIAG=1 EXP=ORACLEGATE FPL_CELLS=/tmp/oraclegate/cells.csv \
//	    go test ./internal/backtest -run '^TestDiagTransferGateOracle$' -count=1 -v -timeout 3h
//	Rscript stats/sweep_inference.R /tmp/oraclegate/cells.csv
//
// # The decision this settles
//
// The record puts the whole transfer gate inside a ~300-point noise band, and
// every constant in it — `min_gain_for_free_transfer`, `free_transfer_value`,
// `min_gain_for_hit`, the decision horizon — has been swept, argued over,
// shipped, retracted and re-swept without resolving. This bounds all of them at
// once, because no gate constant can be worth more than a gate that is right
// every time.
//
// **So the number below is decision-theoretic rather than descriptive.** If a
// perfect gate over the model's own proposals comes in below the transfer
// metric's detection threshold, then no constant in that family can ever be
// resolved on this harness and the tuning programme for it closes — which is
// worth far more than another sweep.
//
// # It separates the gate from the search, which nothing else here does
//
// The model proposes; the oracle only answers yes or no. A package the search
// never offers is invisible to this, so it bounds *acceptance* and not *reach*.
// That is the right split: the standing finding is that the unified search makes
// the same number of transfers as the bespoke one and gets less for them, so the
// problem is valuation rather than reach — and this measures how much of the
// valuation problem lives in the gate specifically.
//
// # The invariance is all three held rungs
//
// HOLD buys the opening fifteen and never transfers, so a gate cannot reach it.
// If any held rung moves, the oracle is changing squad selection and its figure
// bounds something other than the gate.
//
// # The mediator is the transfer count
//
// A gate that accepted and refused exactly what the shipped one does would report
// a clean null indistinguishable from "the gate is already perfect". Counting
// moves separates those two.

import (
	"fmt"
	"sort"
	"testing"
)

// gateCell is one cell's transfer count under one arm.
type gateCell struct {
	season      string
	start       int
	moves, hits int
	weeks       int
}

func TestDiagTransferGateOracle(t *testing.T) {
	requireDiag(t)
	starts := sweepStarts()

	fmt.Printf("\n=== the transfer-gate oracle, full grid.\n")
	fmt.Printf("The model proposes; the oracle only says yes or no, judged on what\n")
	fmt.Printf("the players actually returned over the horizon the decision was\n")
	fmt.Printf("justified on. POLICY is the metric — this is a transfer constant —\n")
	fmt.Printf("and all three held rungs are pinned, because HOLD makes no transfers.\n")

	var base, oracled []gateCell
	collect := func(into *[]gateCell) func(seasonPair, int, *SimResult) {
		return func(pair seasonPair, start int, res *SimResult) {
			*into = append(*into, gateCell{
				season: pair.Name, start: start,
				moves: res.Transfers, hits: res.Hits, weeks: len(res.Weeks),
			})
		}
	}

	shipped := policyVariant{label: "real (ships)", apply: func(sc *SimConfig) {}}
	shipped.observe = collect(&base)
	oracle := oracleVariant(Oracles{Decision: AxisTransferGate}, "perfect acceptance", nil)
	oracle.observe = collect(&oracled)

	runPolicySweep(t, []policyVariant{shipped, oracle}, starts)

	reportGateMediator(t, base, oracled)
}

func reportGateMediator(t *testing.T, base, oracled []gateCell) {
	t.Helper()
	if len(base) == 0 || len(base) != len(oracled) {
		t.Fatalf("observed %d baseline cells and %d oracled ones", len(base), len(oracled))
	}
	key := func(c gateCell) string { return fmt.Sprintf("%s@%d", c.season, c.start) }
	sort.Slice(base, func(i, j int) bool { return key(base[i]) < key(base[j]) })
	sort.Slice(oracled, func(i, j int) bool { return key(oracled[i]) < key(oracled[j]) })

	fmt.Printf("\nMEDIATOR — transfers accepted, shipped gate against a perfect one.\n")
	fmt.Printf("%-9s %6s %8s %8s %8s %8s\n",
		"season", "start", "shipped", "oracle", "hits s", "hits o")
	var bm, om, bh, oh int
	for i := range base {
		fmt.Printf("%-9s %6d %8d %8d %8d %8d\n", base[i].season, base[i].start,
			base[i].moves, oracled[i].moves, base[i].hits, oracled[i].hits)
		bm += base[i].moves
		om += oracled[i].moves
		bh += base[i].hits
		oh += oracled[i].hits
	}
	fmt.Printf("%-9s %6s %8d %8d %8d %8d\n", "all", "", bm, om, bh, oh)
	fmt.Printf("\nA mediator that does not move means the wiring is broken, not that\n")
	fmt.Printf("the gate is already perfect. Everything is a hindsight upper bound:\n")
	fmt.Printf("nobody can know on Friday which transfers will pay by May.\n")
	if bm == om {
		t.Error("the perfect gate accepted exactly as many transfers as the shipped " +
			"one across the whole grid, which is not credible — the oracle is " +
			"wired and inert, and a sweep of it reports a clean null")
	}
}
