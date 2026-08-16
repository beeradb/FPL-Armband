package backtest

// Is a bench boost worth more when a wildcard has prepared for it?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagChipSequence -v -timeout 2h
//
// TestDiagChipWeekOracle measures the two scoring chips on the squad the policy
// happened to hold, and that squad has a worthless bench by construction:
// XIValue credits bench players at zero, so the fifteen converges on eleven
// good players and four who cannot cover. Bench boost pays all fifteen, so that
// measurement is a floor.
//
// The play the chip is actually used in is a sequence — wildcard into a squad
// with a real bench, boost it, then transfer the surplus back out. This
// measures the first two steps against doing neither and against boosting
// alone, which is the comparison that says whether preparation is what the
// chip is worth.
//
// # On the timing
//
// The boost week is taken from the chip-week oracle, so it is hindsight
// — but it is the *same* hindsight in every arm, and the question here is not
// when to play the chip but whether preparing for it pays.
//
// ⚠️ That holding the week fixed is load-bearing, so what licenses it has to be
// stated correctly, and an earlier version of this note made two claims in one
// sentence — "measured at 8.3 points a season, below this harness's detection
// threshold" — both of which are retracted. Timing reads **+8.3 pooled** and
// **13.3 at GW1**, pooled over cells whose windows differ, so it mixes estimands
// and the column is the reading rather than the pooled figure. And it has **no
// detection threshold of its own**: the "42-59" this record once gave it was
// borrowed from another comparison and is withdrawn, so there is nothing to be
// "below" and the claim to be below one had nothing behind it.
//
// So the licence is NOT that timing is indistinguishable from zero — a null is a
// tie and never a refutation. It is a design argument, and it needs no size at
// all: letting the week vary between arms would add a timing channel this 2x2
// cannot separate from the preparation channel it is asking about, whatever
// timing turns out to be worth.

import (
	"context"
	"fmt"
	"os"
	"testing"

	"armband/internal/analysis"
)

func TestDiagChipSequence(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)
	ctx := context.Background()

	// Season, prior, and the bench-boost oracle week from the chip-week oracle,
	// as it read at one entry point before the oracle moved onto the shared grid.
	cases := []struct {
		prior, cur string
		boostGW    int
	}{
		{"2021-22", "2022-23", 27},
		{"2022-23", "2023-24", 35},
		{"2023-24", "2024-25", 36},
		{"2024-25", "2025-26", 8},
	}

	fmt.Printf("\nBench boost with and without a wildcard preparing for it.\n")
	fmt.Printf("Boost week is the chip-week oracle, same in every arm.\n")
	fmt.Printf("Wildcard goes the week before, since FPL allows one chip a week.\n\n")
	fmt.Printf("%-9s %5s %8s %8s %8s %10s %10s\n",
		"season", "gw", "none", "boost", "wc only", "wc+boost", "wc(plain)")

	var sumBoost, sumSeq, sumWC, sumPlain int
	for _, c := range cases {
		prior, err := Load(ctx, cfg.CacheDir, c.prior)
		if err != nil {
			t.Fatal(err)
		}
		cur, err := Load(ctx, cfg.CacheDir, c.cur)
		if err != nil {
			t.Fatal(err)
		}
		base := SimConfig{
			Weights: cfg.Weights, MinGain: cfg.Review.MinGainForTransfer,
			MinGainHit: cfg.Review.MinGainForHit, BankUpTo: sweepBankLimit,
			MaxHits: cfg.Review.MaxHitsPerWeek, Budget: 1000,
			FreeCost: cfg.Review.FreeTransferValue, StartGW: 1, WeeklyXI: true,
		}

		run := func(p analysis.ChipPlan) int {
			sc := base
			sc.Chips = p
			r, err := Simulate(cur, prior, sc)
			if err != nil {
				t.Fatal(err)
			}
			return r.Points
		}

		none := run(analysis.ChipPlan{})
		boost := run(analysis.ChipPlan{BenchBoost: c.boostGW})
		// The wildcard on its own, in the same week it would prepare from. This
		// separates "the wildcard costs something here" from "building for the
		// boost costs something", which the first version of this test could not
		// tell apart.
		wcOnly := run(analysis.ChipPlan{Wildcard: c.boostGW - 1})
		// FPL allows one chip per gameweek, so the wildcard goes the week
		// *before* the boost — the sequence as it is actually played.
		seq := run(analysis.ChipPlan{Wildcard: c.boostGW - 1, BenchBoost: c.boostGW})
		// The same sequence with the rebuild NOT told the boost is coming, so
		// it builds an ordinary squad. If this beats the arm above, optimising
		// the fifteen for one chip week is what costs, not the wildcard.
		wildcardBuildsForBoost = false
		plain := run(analysis.ChipPlan{Wildcard: c.boostGW - 1, BenchBoost: c.boostGW})
		wildcardBuildsForBoost = true

		sumBoost += boost - none
		sumSeq += seq - boost
		sumWC += wcOnly - none
		sumPlain += plain - boost
		fmt.Printf("%-9s %5d %8d %8d %8d %10d %10d\n",
			c.cur, c.boostGW, none, boost, wcOnly, seq, plain)
	}

	n := float64(len(cases))
	fmt.Printf("\nbench boost alone, against no chips:          %+.1f\n", float64(sumBoost)/n)
	fmt.Printf("wildcard alone, against no chips:            %+.1f\n", float64(sumWC)/n)
	fmt.Printf("wildcard+boost, against boost alone:         %+.1f\n", float64(sumSeq)/n)
	fmt.Printf("  same, rebuild not told the boost is coming: %+.1f\n", float64(sumPlain)/n)
	fmt.Printf("\nRead the middle two together. If the wildcard alone is already negative the\n")
	fmt.Printf("cost is the rebuild, not the preparation — this policy re-optimises every\n")
	fmt.Printf("week, so it has no accumulated mistakes for a wildcard to undo, which is\n")
	fmt.Printf("most of what the chip is for a human manager.\n")
}
