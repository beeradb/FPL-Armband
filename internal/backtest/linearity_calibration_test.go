package backtest

// Do a player's returns accrue linearly in the minutes he plays?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagMinutesLinearity -v
//
// This is the assumption underneath MinutesWeight and its per-position scale.
// An exponent above 1 says a part-time player is worth *less* than his minutes
// share of his own per-90 output; an exponent of 1 says exactly proportional.
// Nobody has checked which is true, or whether the answer differs by position —
// and the per-position scale only makes sense if it does.
//
// # Within-player, or it measures quality instead
//
// Across players the answer is obvious and useless: players who play less are
// worse, so their per-90 output is lower and any exponent above 1 looks
// justified. The model applies the exponent to a player's *own* rate, so the
// question is what happens to that rate when he personally plays less. Every
// figure here therefore compares a player's partial starts against his own full
// matches in the same season.
//
// The residual confound is worth stating: a player substituted at 60 is
// sometimes having a bad game, and one substituted at 80 is often having a good
// one in a settled match. That biases partial starts downward, which is the
// same direction as the effect being looked for — so a null here is strong and
// a small negative is weak.

import (
	"context"
	"fmt"
	"math"
	"testing"

	"armband/internal/analysis"
)

func TestDiagMinutesLinearity(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	ctx := context.Background()

	names := map[int]string{1: "GKP", 2: "DEF", 3: "MID", 4: "FWD"}
	type acc struct {
		n            float64
		ratio        float64
		ratios       []float64
		fullPer90    float64
		partialPer90 float64
		partialMins  float64
	}
	by := map[int]*acc{1: {}, 2: {}, 3: {}, 4: {}}

	// Named so the header below counts this list rather than restating it.
	seasons := []string{"2022-23", "2023-24", "2024-25", "2025-26"}

	for _, sn := range seasons {
		s, err := Load(ctx, cfg.CacheDir, sn)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range s.Players {
			a := by[p.Type]
			if a == nil {
				continue
			}
			var fullPts, fullMins, fullN float64
			var partPts, partMins, partN float64
			for _, g := range p.GWs {
				if g.Starts == 0 || g.Minutes == 0 {
					continue // only starts: a cameo is a different question
				}
				if g.Minutes >= 90 {
					fullPts += float64(g.Points)
					fullMins += float64(g.Minutes)
					fullN++
					continue
				}
				if g.Minutes < 45 {
					continue // an early substitution is usually an injury
				}
				partPts += float64(g.Points)
				partMins += float64(g.Minutes)
				partN++
			}
			// Enough of both to be his own control.
			if fullN < 5 || partN < 5 || fullMins == 0 || partMins == 0 {
				continue
			}
			full90 := fullPts / (fullMins / 90)
			part90 := partPts / (partMins / 90)
			if full90 <= 0 {
				continue
			}
			a.n++
			a.ratio += part90 / full90
			a.ratios = append(a.ratios, part90/full90)
			a.fullPer90 += full90
			a.partialPer90 += part90
			a.partialMins += partMins / partN
		}
	}

	fmt.Printf("\nPer-90 output in partial starts against the same player's full\n")
	fmt.Printf("matches, same season. %s, 5+ of each.\n\n", seasonsLabel(len(seasons)))
	fmt.Printf("%-6s %7s %12s %12s %10s %12s %9s\n",
		"pos", "n", "full pts/90", "part pts/90", "ratio", "mean part", "implied")
	for _, pos := range []int{1, 2, 3, 4} {
		a := by[pos]
		if a.n == 0 {
			continue
		}
		r := a.ratio / a.n
		mins := a.partialMins / a.n
		// An exponent b satisfies (m/90)^(b-1) = ratio, since the model already
		// prices the m/90 of minutes and the exponent is the excess.
		implied := 1.0
		if share := mins / 90; share > 0 && share < 1 && r > 0 {
			implied = 1 + math.Log(r)/math.Log(share)
		}
		fmt.Printf("%-6s %7.0f %12.3f %12.3f %10.3f %12.1f %9.2f\n",
			names[pos], a.n, a.fullPer90/a.n, a.partialPer90/a.n, r, mins, implied)
	}

	fmt.Printf("\nratio is what a partial start returns per 90 against the player's own\n")
	fmt.Printf("full matches. Below 1 means playing less costs him rate as well as\n")
	fmt.Printf("minutes, which is what an exponent above 1 prices. 'implied' is the\n")
	fmt.Printf("exponent that reproduces the ratio at the observed partial length.\n")
	// Read from the engine rather than re-derived here: an inline copy of the
	// formula printed "midfielders effectively 1.0000" while the model used a
	// different number, which defeats the comparison this line exists for.
	fmt.Printf("Shipped: %.2f global, midfielders effectively %.4f.\n",
		cfg.Weights.MinutesWeight, analysis.MinutesExponentForPosition(cfg.Weights, "MID"))

	for _, pos := range []int{1, 2, 3, 4} {
		a := by[pos]
		if len(a.ratios) < 30 {
			continue
		}
		m, s := a.ratio/a.n, sd(a.ratios)
		se := s / math.Sqrt(a.n)
		fmt.Printf("  %-4s ratio %.3f ± %.3f  [%.3f, %.3f]\n",
			names[pos], m, 1.96*se, m-1.96*se, m+1.96*se)
	}
}
