package backtest

// Does the bench-slot derivation's "every outfield blank is coverable"
// assumption actually bind on the elevens this optimiser builds?
//
// benchslots.go prices the three outfield bench slots as P(at least one, two,
// three outfield starters blank), treating any bench outfielder as able to
// cover any blanking starter. FPL does not work that way — a substitution only
// happens if the resulting eleven is legal — which is the same assumption the
// *scoring* path made until legalAutosubs fixed it, and that was worth -14
// points a season.
//
// Before porting the correction into the derivation, measure whether it can
// matter. An eleven constrains substitution when it sits at a positional
// minimum: three defenders (a blanking defender then needs a bench defender),
// one forward, or two midfielders. A 4-4-2 or 4-3-3 constrains nothing.
//
//	DIAG=1 go test ./internal/backtest -run TestDiagBenchSlotLegality -v -timeout 1h

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"armband/internal/analysis"
)

func TestDiagBenchSlotLegality(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	ctx := context.Background()

	// The shared grid, not a pasted copy — see TestTheGridIsDeclaredOnce.
	pairs := sweepPairNames()
	starts := sweepStarts()

	formations := map[string]int{}
	constrained := map[string]int{} // which minimum is hit
	total, anyConstraint := 0, 0
	// Of the constrained elevens, does the bench actually carry cover for the
	// constrained position? That is the case where the derivation is wrong:
	// it credits a slot that could never be used.
	uncoverable := 0

	for _, pair := range pairs {
		prior, err := Load(ctx, cfg.CacheDir, pair[0])
		if err != nil {
			t.Fatal(err)
		}
		cur, err := Load(ctx, cfg.CacheDir, pair[1])
		if err != nil {
			t.Fatal(err)
		}
		idx := newPriorIndex(prior)
		for _, start := range starts {
			e, _ := EngineAt(cur, prior, start-1, SimConfig{Weights: cfg.Weights})
			e.Priors = idx
			sq, err := e.Optimize(analysis.OptimizeRequest{
				MinMinutes: 600, MinExpectedMinutes: 55,
				BenchWeight: analysis.DefaultBenchWeight,
			})
			if err != nil {
				continue
			}
			total++
			counts := map[string]int{}
			for _, p := range sq.StartingXI {
				counts[p.Position]++
			}
			formations[sq.Formation]++

			benchByPos := map[string]int{}
			for _, p := range sq.Bench {
				benchByPos[p.Position]++
			}

			hit := false
			for _, c := range []struct {
				pos string
				min int
			}{{"DEF", 3}, {"MID", 2}, {"FWD", 1}} {
				if counts[c.pos] != c.min {
					continue
				}
				hit = true
				constrained[c.pos]++
				// A blanking player in this position can only be replaced by a
				// bench player of the same position.
				if benchByPos[c.pos] == 0 {
					uncoverable++
				}
			}
			if hit {
				anyConstraint++
			}
		}
	}

	// `total` counts the squads the optimiser actually returned, and the grid
	// label counts the cells it was asked for. They are different numbers on
	// purpose — a cell whose Optimize errors is skipped above — so both are
	// printed and neither is written as a literal that could contradict the
	// other.
	fmt.Printf("\n=== Opening elevens, %d squads from %s ===\n\n", total, gridLabel(len(pairs), len(starts)))
	var forms []string
	for f := range formations {
		forms = append(forms, f)
	}
	sort.Strings(forms)
	for _, f := range forms {
		fmt.Printf("  %-8s %3d  (%.0f%%)\n", f, formations[f], 100*float64(formations[f])/float64(total))
	}

	fmt.Printf("\nElevens sitting at a positional minimum, so that a blank there can only\n")
	fmt.Printf("be covered by a bench player of the same position:\n\n")
	fmt.Printf("  at the DEF minimum (3): %d\n", constrained["DEF"])
	fmt.Printf("  at the MID minimum (2): %d\n", constrained["MID"])
	fmt.Printf("  at the FWD minimum (1): %d\n", constrained["FWD"])
	fmt.Printf("  any constraint at all:  %d of %d (%.0f%%)\n",
		anyConstraint, total, 100*float64(anyConstraint)/float64(total))
	fmt.Printf("\n  constrained positions with NO bench cover of that position: %d\n", uncoverable)
	fmt.Printf("  (this is the case the derivation prices and FPL would never pay)\n")
}
