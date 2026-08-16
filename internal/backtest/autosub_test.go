package backtest

// Measures the formation-legality fix to the replay's autosubs (see
// legalAutosubs in replay.go) on both metrics, at the standard grid — whatever
// sweepPairNames and sweepStarts return, which the printed header counts rather
// than this comment.
//
// Two distinct errors are corrected at once and they push opposite ways:
//
//   - A blanking *keeper* was replaced by the highest-scoring bench
//     outfielder, because bestXIWith orders the reserve keeper last. FPL
//     never makes that swap — the eleven holds exactly one keeper. This is
//     mostly a mis-assignment rather than an over-credit: some bench player
//     was going to be credited either way, so the net effect is the
//     difference between the outfielder's return and the reserve keeper's.
//   - A blanking *defender* in a three-at-the-back side was replaced even
//     when every bench outfielder would have taken the eleven to two
//     defenders, where FPL makes no substitution at all. That one is a pure
//     over-credit and does not wash out.
//
// So the expected sign is negative — the corrected replay should score
// slightly *lower*, because it stops paying for substitutions that never
// happened. A negative result here is the fix working, not the fix failing.
//
//	DIAG=1 go test ./internal/backtest -run TestDiagLegalAutosubs -v -timeout 1h

import (
	"context"
	"fmt"
	"os"
	"testing"
)

func TestDiagLegalAutosubs(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)
	ctx := context.Background()

	// The shared grid, not a pasted copy. A diagnostic measuring a different
	// season population from the sweeps it is quoted beside is a silent
	// failure, and FPL_SWEEP_SEASONS/FPL_SWEEP_STARTS cannot reach a literal.
	pairs := sweepPairNames()
	starts := sweepStarts()

	type cell struct{ holdOn, holdOff, policyOn, policyOff float64 }
	cells := map[string]cell{}

	for _, pair := range pairs {
		prior, err := Load(ctx, cfg.CacheDir, pair[0])
		if err != nil {
			t.Fatal(err)
		}
		cur, err := Load(ctx, cfg.CacheDir, pair[1])
		if err != nil {
			t.Fatal(err)
		}
		for _, start := range starts {
			weeks := float64(38 - start + 1)
			if weeks <= 0 {
				continue
			}
			sc := SimConfig{
				Weights: cfg.Weights, MinGain: cfg.Review.MinGainForTransfer,
				MinGainHit: cfg.Review.MinGainForHit, BankUpTo: sweepBankLimit,
				MaxHits: cfg.Review.MaxHitsPerWeek, Budget: 1000,
				FreeCost: cfg.Review.FreeTransferValue, StartGW: start, WeeklyXI: true,
			}

			legalAutosubs = true
			resOn, err := Simulate(cur, prior, sc)
			if err != nil {
				t.Fatal(err)
			}
			holdOn := float64(Hold(cur, prior, sc, resOn.OpeningSquad)) / weeks
			policyOn := float64(resOn.Points) / weeks

			legalAutosubs = false
			resOff, err := Simulate(cur, prior, sc)
			if err != nil {
				t.Fatal(err)
			}
			holdOff := float64(Hold(cur, prior, sc, resOff.OpeningSquad)) / weeks
			policyOff := float64(resOff.Points) / weeks
			legalAutosubs = true

			cells[fmt.Sprintf("%s@%d", pair[1], start)] = cell{
				holdOn: holdOn, holdOff: holdOff, policyOn: policyOn, policyOff: policyOff,
			}
		}
	}

	// Mean paired difference and the cell count. NO standard error, no t, no
	// verdict word — those come from stats/sweep_inference.R and nowhere else.
	//
	// This closure used to compute a naive SE and an average-the-seasons
	// clustered SE inline. That is the estimator this package retired for having
	// no small-sample correction and no principled degrees of freedom, and a
	// second copy of it is the DefaultBenchWeight-versus-Weights.BenchWeight bug
	// class: two implementations of one quantity, with the measured one not
	// necessarily the one that runs. Set FPL_CELLS and let R do the arithmetic.
	extract := func(get func(cell) (on, off float64)) (mean float64, n int) {
		var sum float64
		for _, c := range cells {
			on, off := get(c)
			sum += on - off
			n++
		}
		if n == 0 {
			return 0, 0
		}
		return sum / float64(n), n
	}

	report := func(label string, get func(cell) (on, off float64)) {
		m, n := extract(get)
		fmt.Printf("%-10s %+9.4f  (n=%d, ~%.0f/season)\n", label, m, n, m*38)
	}

	fmt.Printf("\nLegal autosubs (corrected) vs the old unchecked substitution, %s.\n",
		gridLabel(len(pairs), len(starts)))
	fmt.Printf("Points per gameweek played. A negative figure is the fix removing\n")
	fmt.Printf("substitutions FPL would never have made.\n\n")
	fmt.Printf("%-10s %9s\n", "metric", "mean/gw")
	fmt.Printf("Standard errors, t and any verdict come from stats/sweep_inference.R\n")
	fmt.Printf("on the cells FPL_CELLS emits — not from here.\n\n")
	report("HOLD", func(c cell) (float64, float64) { return c.holdOn, c.holdOff })
	report("POLICY", func(c cell) (float64, float64) { return c.policyOn, c.policyOff })
}
