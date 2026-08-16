package backtest

// Re-measures FreeTransferValue (shipped 2.0) on the current model and the
// shared paired-difference harness, whose size the printed header counts rather
// than this comment. Every earlier figure for this constant
// came from single-path GW1 replays, and two things have moved underneath it
// since: min_gain_for_free_transfer settled back to 0.4 (so it is no longer
// silently sharing the job with a second constant at a different value), and
// the doubles-counting and sales-at-selling-price fixes changed what the
// model sees. The work queue asks two questions: does the confidence-threshold story
// still hold, and is the charge still doing the job it was introduced for —
// stopping round-trip churn — or has a settled min_gain taken that over.
//
// HOLD is provably invariant to this constant (it gates a transfer decision
// nothing about HOLD depends on), so only POLICY is measured. Round-trips are
// counted directly rather than inferred from points, since a charge could
// move points without changing churn or vice versa.
//
//	DIAG=1 go test ./internal/backtest -run TestDiagFreeTransferValue -v -timeout 1h

import (
	"context"
	"fmt"
	"os"
	"testing"
)

// roundTrips counts players sold and later bought back within one replay.
func roundTrips(moves []Move) int {
	sold := map[int]bool{}
	trips := 0
	for _, m := range moves {
		if m.OutID != 0 {
			sold[m.OutID] = true
		}
		if m.InID != 0 && sold[m.InID] {
			trips++
			sold[m.InID] = false // count each round-trip once
		}
	}
	return trips
}

func TestDiagFreeTransferValue(t *testing.T) {
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
	shipped := cfg.Review.FreeTransferValue
	candidates := []float64{1.0, 1.5, shipped, 2.5, 3.0}

	type cell struct {
		policy map[float64]float64
		trips  map[float64]int
		moves  map[float64]int
	}
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
			c := cell{policy: map[float64]float64{}, trips: map[float64]int{}, moves: map[float64]int{}}
			for _, fc := range candidates {
				sc := SimConfig{
					Weights: cfg.Weights, MinGain: cfg.Review.MinGainForTransfer,
					MinGainHit: cfg.Review.MinGainForHit, BankUpTo: sweepBankLimit,
					MaxHits: cfg.Review.MaxHitsPerWeek, Budget: 1000,
					FreeCost: fc, StartGW: start, WeeklyXI: true,
				}
				res, err := Simulate(cur, prior, sc)
				if err != nil {
					t.Fatal(err)
				}
				c.policy[fc] = float64(res.Points) / weeks
				c.trips[fc] = roundTrips(res.Moves)
				c.moves[fc] = len(res.Moves)
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
	extract := func(fc float64) (mean float64, n int) {
		var sum float64
		for _, c := range cells {
			sum += c.policy[fc] - c.policy[shipped]
			n++
		}
		if n == 0 {
			return 0, 0
		}
		return sum / float64(n), n
	}

	fmt.Printf("\nFreeTransferValue re-measured, shipped %.1f, %s.\n",
		shipped, gridLabel(len(pairs), len(starts)))
	fmt.Printf("POLICY only — HOLD is provably invariant to this constant.\n\n")
	fmt.Printf("%-6s %9s %9s %7s %11s %7s %10s %8s\n",
		"value", "mean/gw", "naive SE", "t", "cluster SE", "t", "round-trips", "moves")
	for _, fc := range candidates {
		totalTrips, totalMoves := 0, 0
		for _, c := range cells {
			totalTrips += c.trips[fc]
			totalMoves += c.moves[fc]
		}
		if fc == shipped {
			fmt.Printf("%-6.1f %9s %10d %8d  (baseline)\n",
				fc, "-", totalTrips, totalMoves)
			continue
		}
		m, _ := extract(fc)
		fmt.Printf("%-6.1f %+9.4f %10d %8d\n", fc, m, totalTrips, totalMoves)
	}
}
