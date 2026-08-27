package backtest

// Measures whether LeagueShrinkK's out-of-sample MAE optimum (K=2-4, beating
// the shared BlendRateK=8 in THREE of four seasons — ⚠️ corrected 2026-08-15
// from "every one of four", which the recorded MAE ranges refute: K=8 wins
// 2022-23 outright. See AGENTS.md) also survives the argmax test, before it
// ships.
//
// This is exactly the shape of test this project has been burned by before:
// "recency on rates" predicted better out of sample and lost points at every
// setting, because a transfer policy is an argmax living in the tail of the
// estimate distribution, and the zero-prior population this constant governs
// is precisely the new-signing valuations that argmax has burned before (the
// GW2 +8.50/gw transfer into a player with 90 minutes of Premier League
// football that shrinkToLeague exists to prevent). A better predictor is not
// automatically a better policy, so this checks rather than assumes.
//
//	DIAG=1 go test ./internal/backtest -run TestDiagLeagueShrinkK -v -timeout 1h

import (
	"context"
	"fmt"
	"testing"
)

func TestDiagLeagueShrinkK(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	ctx := context.Background()

	// The shared grid, not a pasted copy. A diagnostic measuring a different
	// season population from the sweeps it is quoted beside is a silent
	// failure, and FPL_SWEEP_SEASONS/FPL_SWEEP_STARTS cannot reach a literal.
	pairs := sweepPairNames()
	starts := sweepStarts()
	candidates := []float64{2, 4, 8} // 8 is shipped/baseline

	type cell struct{ hold, policy map[float64]float64 }
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
			c := cell{hold: map[float64]float64{}, policy: map[float64]float64{}}
			for _, k := range candidates {
				sc := SimConfig{
					Weights: cfg.Weights, MinGain: cfg.Review.MinGainForTransfer,
					MinGainHit: cfg.Review.MinGainForHit, BankUpTo: sweepBankLimit,
					MaxHits: cfg.Review.MaxHitsPerWeek, Budget: 1000,
					FreeCost: cfg.Review.FreeTransferValue, StartGW: start, WeeklyXI: true,
				}
				sc.Weights.LeagueShrinkK = k
				res, err := Simulate(cur, prior, sc)
				if err != nil {
					t.Fatal(err)
				}
				c.hold[k] = float64(Hold(cur, prior, sc, res.OpeningSquad)) / weeks
				c.policy[k] = float64(res.Points) / weeks
			}
			cells[fmt.Sprintf("%s@%d", pair[1], start)] = c
		}
	}

	// Mean paired difference and the cell count. NO standard error, no t, no
	// verdict word — those come from stats/sweep_inference.R and nowhere else.
	//
	// This closure used to compute a naive SE and an average-the-seasons
	// clustered SE inline. That is the estimator this package retired for having
	// no small-sample correction and no principled degrees of freedom, and a
	// second copy of it is the two-implementations-of-one-quantity bug class.
	// Set FPL_CELLS and let R do the arithmetic.
	extract := func(get func(cell) map[float64]float64, k float64) (mean float64, n int) {
		var sum float64
		for _, c := range cells {
			m := get(c)
			sum += m[k] - m[8] // 8 is the shipped LeagueShrinkK
			n++
		}
		if n == 0 {
			return 0, 0
		}
		return sum / float64(n), n
	}

	report := func(label string, get func(cell) map[float64]float64, k float64) {
		m, n := extract(get, k)
		fmt.Printf("%-14s %+9.4f  (n=%d, ~%.0f/season)\n", label, m, n, m*38)
	}

	fmt.Printf("\nLeagueShrinkK vs shipped K=8, %s.\n", gridLabel(len(pairs), len(starts)))
	fmt.Printf("Points per gameweek played.\n\n")
	fmt.Printf("%-14s %9s %9s %7s %11s %7s\n", "metric", "mean/gw", "naive SE", "t", "cluster SE", "t")
	for _, k := range candidates {
		if k == 8 {
			continue
		}
		report(fmt.Sprintf("HOLD K=%.0f", k), func(c cell) map[float64]float64 { return c.hold }, k)
		report(fmt.Sprintf("POLICY K=%.0f", k), func(c cell) map[float64]float64 { return c.policy }, k)
	}
}
