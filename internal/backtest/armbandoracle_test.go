package backtest

// What is a perfect armband worth?
//
//	DIAG=1 EXP=ORACLEARMBAND FPL_CELLS=/tmp/oraclearmband/cells.csv \
//	    go test ./internal/backtest -run '^TestDiagArmbandOracle$' -count=1 -v -timeout 3h
//	Rscript stats/sweep_inference.R /tmp/oraclearmband/cells.csv
//
// # Why this one, second
//
// The variance decomposition puts the armband at **+4.779 points a gameweek**
// statically — ahead of re-picking the eleven weekly (+2.771) and ahead of
// autosubs (+2.360), and second only to transfers. It is also the one decision
// this project has argued about most and measured least: three captaincy *rules*
// were compared against each other, and none of them against the ceiling.
//
// So this bounds better **judgement given the same data**. The eleven is still the
// model's own, picked on the model's own scores; only the armband is hindsight.
//
// # It bounds captain and vice jointly, and that must be said rather than found
//
// FPL passes the armband to the vice-captain whenever the captain records no
// minutes, and the replay's own diagnostic puts the model's captain blanking in
// 9.6% of weeks. An oracle captain necessarily played — he is chosen from the
// players who did — so under this oracle the fallback never fires and the ~16
// points a season the vice rule is separately worth are inside this figure. Read
// it as a bound on "who wears the armband", not on the captain alone.
//
// # The invariance is four columns and it is free
//
// `decide` never reads the captain, so **transfer count and hits must be
// byte-identical**. Those are integers counted without noise, which makes them a
// far sharper check than any points column: one changed transfer is visible where
// a tenth of a point a gameweek is not. The no-captain rung doubles nobody and so
// cannot see an armband, and the fixed-captain rung is deliberately left
// un-oracled — its definition is "the day-one pick", and a hindsight day-one pick
// would be neither pinned nor an instrument.
//
// # The mediator
//
// How many weeks the oracle captained somebody other than the model's choice,
// counted in the replay rather than inferred from a points difference. A points
// difference of zero has two explanations; a change count of zero has one, and it
// is "the oracle is wired and inert".

import (
	"fmt"
	"os"
	"sort"
	"testing"
)

func TestDiagArmbandOracle(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	starts := sweepStarts()

	fmt.Printf("\n=== the armband oracle, full grid.\n")
	fmt.Printf("Baseline is the shipped model. Positive means hindsight gains, so the\n")
	fmt.Printf("mean is an upper bound on better captaincy judgement — captain AND\n")
	fmt.Printf("vice jointly, because an oracle captain always played and the\n")
	fmt.Printf("vice-captain fallback therefore never fires.\n")
	fmt.Printf("HOLD is the primary metric: the armband is a scoring decision, not a\n")
	fmt.Printf("transfer one, and moves and hits are pinned as invariances precisely\n")
	fmt.Printf("because decide() never reads the captain.\n")

	var rows []armbandMediator
	collect := func(pair seasonPair, start int, res *SimResult) {
		if res.Armband == nil {
			t.Errorf("%s@%d ran under %s and reported no armband mediator — the "+
				"axis is stamped and inert", pair.Name, start,
				(Oracles{Decision: AxisArmband}).Stamp())
			return
		}
		rows = append(rows, armbandMediator{
			season: pair.Name, start: start,
			weeks: res.Armband.Weeks, changed: res.Armband.Changed,
		})
	}

	oracle := oracleVariant(Oracles{Decision: AxisArmband}, "perfect armband", nil)
	oracle.observe = collect
	runPolicySweep(t, []policyVariant{
		{label: "real (ships)", apply: func(sc *SimConfig) {}},
		oracle,
	}, starts)

	reportArmbandMediator(t, rows)
}

// armbandMediator is one cell's count of how often hindsight disagreed.
type armbandMediator struct {
	season         string
	start          int
	weeks, changed int
}

func reportArmbandMediator(t *testing.T, rows []armbandMediator) {
	t.Helper()
	if len(rows) == 0 {
		t.Fatal("the oracled arm observed no cells, so nothing above was measured")
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].start != rows[j].start {
			return rows[i].start < rows[j].start
		}
		return rows[i].season < rows[j].season
	})
	fmt.Printf("\nMEDIATOR — weeks the hindsight armband differed from the model's.\n")
	fmt.Printf("%-9s %6s %7s %8s %8s\n", "season", "start", "weeks", "changed", "share")
	var weeks, changed int
	for _, r := range rows {
		fmt.Printf("%-9s %6d %7d %8d %7.0f%%\n", r.season, r.start, r.weeks, r.changed,
			100*float64(r.changed)/float64(max(r.weeks, 1)))
		weeks += r.weeks
		changed += r.changed
	}
	fmt.Printf("%-9s %6s %7d %8d %7.0f%%\n", "all", "", weeks, changed,
		100*float64(changed)/float64(max(weeks, 1)))
	fmt.Printf("\nA mediator that does not move means the wiring is broken, not that\n")
	fmt.Printf("the effect is zero. Everything above is a hindsight upper bound.\n")
	if changed == 0 {
		t.Error("the oracle never captained anyone the model would not have, across " +
			"the whole grid — an oracle that changes nothing measures nothing and " +
			"reports it as a clean null")
	}
}
