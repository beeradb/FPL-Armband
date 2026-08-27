package backtest

import (
	"fmt"
	"testing"
)

// WHAT DOES THE LOOKAHEAD READING ACTUALLY READ?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagWildcardLookaheadValue -v -count=1 -timeout 60m
//
// This exists for one reason: **to bracket `WildcardLookaheadBar` from measured
// numbers instead of by analogy with the two bars beside it.** Those bars are in
// different units — a one-off hit price around 12, and a per-gameweek rate around
// 1 to 5 — and this reading is a TOTAL over a run of gameweeks, so it is roughly
// the lookahead times the drift arm's number plus a hit price. Guessing a ladder
// from the neighbours would put every rung either above the top of the
// distribution, where nothing fires, or below the bottom, where it fires in week
// one. Both read as "the rule does nothing" in a sweep and neither is a result.
//
// It also prints how often the reading DISAGREES with the two shipped ones about
// which weeks are bad, because if it ranked weeks identically to the single-week
// drift there would be nothing to test.
//
// ⚠️ **It prints `PeakAt` too, and the expected answer is that it is always 0.**
// That is not a bug in this diagnostic. `wildcardValueOverNext`'s value is
// non-increasing in k by construction, so the peak is always now — see
// TestWildcardValueOverNextPricesTheLookahead, which pins it. The column is here
// so that a future change making waiting expressible is visible immediately
// rather than being assumed.
func TestDiagWildcardLookaheadValue(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	starts := sweepStarts()

	fmt.Printf("\n=== what the LOOKAHEAD wildcard reading reads, against the two shipped ones\n")
	fmt.Printf("Held squad built at entry, then read at entry+5 — the same drifted\n")
	fmt.Printf("squad TestDiagXIDrift measures, so the three readings describe one\n")
	fmt.Printf("situation and can be compared rung for rung.\n")
	fmt.Printf("⚠️ value is a TOTAL over the lookahead; drift is PER GAMEWEEK; cost is\n")
	fmt.Printf("a ONE-OFF hit price. They are three quantities, not three units.\n\n")
	fmt.Printf("%-9s %5s  %8s %9s %8s %7s %7s\n",
		"season", "entry", "value", "drift/gw", "cost", "peakAt", "weeks")

	var values, drifts, costs []float64
	var peaked int
	for _, pr := range loadPairsOrSkip(t, cfg) {
		for _, start := range starts {
			sc := SimConfig{Weights: cfg.Weights, StartGW: start, BankUpTo: 5}
			e, _ := EngineAt(pr.Cur, pr.Prior, start-1, sc)
			held, ok := repairSquad(e, nil, 1000, 0, sc)
			if !ok {
				continue
			}
			at := start + 4
			later, _ := EngineAt(pr.Cur, pr.Prior, at, sc)
			w := newWallet(1000)
			cost, drift, fresh, changes, ok := repairCostAndDrift(later, pr.Cur, w, held, at+1, 1, 0, sc)
			if !ok {
				continue
			}
			// The horizon-1 engine the live rule builds, for the same reason it
			// builds one: fixture load is only in the score at horizon 1.
			b, fx := PointInTimeWith(pr.Cur, pr.Prior, at, sc.Oracles)
			we := oneWeekEngine(b, fx, sc.Weights, later.Priors, later.Recent, later.TeamForm)
			series := xiDriftSeries(we, held, fresh, at+1, sc.wildcardLookahead())
			wv := wildcardValueOverNext(series, changes, 1, sc.BankUpTo)
			if wv.PeakAt != 0 {
				peaked++
			}
			fmt.Printf("%-9s %5d  %8.2f %9.2f %8.1f %7d %7d\n",
				pr.Name, start, wv.Now, drift, cost, wv.PeakAt, len(series))
			values, drifts, costs = append(values, wv.Now), append(drifts, drift), append(costs, cost)
		}
	}
	if len(values) == 0 {
		t.Skip("no cells")
	}

	fmt.Printf("\ncells %d | mean value %.2f | mean drift/gw %.2f | mean cost %.1f\n",
		len(values), meanOf(values), meanOf(drifts), meanOf(costs))
	lo, med, p90, hi := quantiles(values)
	fmt.Printf("value distribution: min %.2f  median %.2f  p90 %.2f  max %.2f\n", lo, med, p90, hi)
	fmt.Printf("\n**THIS is the range a bar ladder must span.** A bar above the max never\n")
	fmt.Printf("fires; one below the min fires in week one. Both read as an inert rule.\n")
	fmt.Printf("\ncorrelation with the single-week drift: %.3f\n", corrOf(values, drifts))
	fmt.Printf("correlation with the shipped hit cost:  %.3f\n", corrOf(values, costs))
	fmt.Printf("⚠️ A correlation near 1 with either would mean this reading ranks weeks\n")
	fmt.Printf("the same way and there is nothing new to test, whatever its units.\n")
	fmt.Printf("\nPeakAt was non-zero in %d of %d cells.\n", peaked, len(values))
	fmt.Printf("⚠️ Zero is the EXPECTED answer and is not evidence the rule works —\n")
	fmt.Printf("the value is non-increasing in k by construction, so a peak-gated rule\n")
	fmt.Printf("would be inert. A non-zero count here means a negative drift week, which\n")
	fmt.Printf("is the held eleven out-scoring the rebuilt one and is noise.\n")
}
