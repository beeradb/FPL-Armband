package backtest

// Does nudging the budget actually produce a different squad?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagBudgetJitter -v -timeout 1h
//
// Budget jitter is proposed as a third averaging axis, alongside seasons and
// entry gameweeks. The idea rests on the response surface being a step function:
// the optimiser returns a *discrete* squad, so a small change in the money
// available flips which player occupies a slot, and a different squad scores
// differently for the rest of the season. Averaging over nearby budgets should
// therefore average over independent draws from the *same* football, which is
// exactly what start points do on a different axis.
//
// # Why this has to be checked before the axis is built
//
// The whole value of an extra axis is extra *independent* samples. If a £0.1m
// nudge usually leaves the optimum unchanged, the extra cells are duplicates of
// the ones already there — and pooling duplicates does not add information, it
// shrinks the standard error while adding nothing, which is worse than not
// having the axis at all. It would manufacture confidence.
//
// So the first question is not "does jitter reduce the noise" but "how many
// *distinct* squads does it generate". That is cheap to answer and decides
// whether the expensive part is worth running.
//
// # £0.1m, not £0.5m
//
// FPL prices move in tenths. The £0.5m step in TestDiagSearchQuality was chosen
// for a pre-season landscape sweep and has no claim to being the granularity at
// which the answer changes.

import (
	"fmt"
	"os"
	"sort"
	"testing"

	"armband/internal/analysis"
)

func TestDiagBudgetJitter(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)

	pairs := sweepPairNames()
	// Tenths around the real allowance, which is what FPL's price grid uses.
	jitters := []int{-3, -2, -1, 0, 1, 2, 3}
	starts := []int{1, 11, 21}

	fmt.Printf("\nOpening squads at budgets around £100.0m, in tenths.\n")
	fmt.Printf("'distinct' counts how many different fifteens the seven budgets produce;\n")
	fmt.Printf("'changed' is how many players differ from the £100.0m squad at each step.\n\n")
	fmt.Printf("%-10s %6s %10s   %s\n", "season", "start", "distinct", "players changed vs £100.0m, by tenth")
	fmt.Printf("%-10s %6s %10s   %s\n", "", "", "of 7", "-3   -2   -1   +1   +2   +3")

	totalDistinct, cells := 0, 0
	for _, pair := range pairs {
		prior := loadSeason(t, cfg, pair[0])
		cur := loadSeason(t, cfg, pair[1])

		for _, start := range starts {
			squads := map[int][]int{}
			for _, j := range jitters {
				e, _ := EngineAt(cur, prior, start-1, SimConfig{Weights: cfg.Weights})
				e.Priors = newPriorIndex(prior)
				sq, err := e.Optimize(analysis.OptimizeRequest{
					Budget: 1000 + j, MinMinutes: 600, MinExpectedMinutes: 55,
					BenchWeight: analysis.DefaultBenchWeight,
				})
				if err != nil {
					continue
				}
				ids := make([]int, 0, 15)
				for _, p := range sq.Players {
					ids = append(ids, p.ID)
				}
				sort.Ints(ids)
				squads[j] = ids
			}
			base, ok := squads[0]
			if !ok {
				continue
			}
			seen := map[string]bool{}
			for _, ids := range squads {
				seen[fmt.Sprint(ids)] = true
			}
			var diffs []string
			for _, j := range jitters {
				if j == 0 {
					continue
				}
				diffs = append(diffs, fmt.Sprintf("%-4d", differing(base, squads[j])))
			}
			fmt.Printf("%-10s %6d %10d   %s\n", pair[1], start, len(seen), joinAll(diffs))
			totalDistinct += len(seen)
			cells++
		}
	}

	if cells > 0 {
		fmt.Printf("\nmean distinct squads per cell: %.2f of %d budgets tried\n",
			float64(totalDistinct)/float64(cells), len(jitters))
	}
	fmt.Printf("\nIf this is close to 1, the axis is dead: the budgets return the same\n")
	fmt.Printf("fifteen and pooling them would shrink the standard error without adding\n")
	fmt.Printf("any information. If it is close to %d, every nudge is a genuinely\n", len(jitters))
	fmt.Printf("different draw and the axis is worth its runtime.\n")
}

// differing counts how many players are in one squad and not the other.
func differing(a, b []int) int {
	if len(b) == 0 {
		return -1
	}
	in := map[int]bool{}
	for _, id := range a {
		in[id] = true
	}
	n := 0
	for _, id := range b {
		if !in[id] {
			n++
		}
	}
	return n
}

func joinAll(xs []string) string {
	out := ""
	for _, x := range xs {
		out += x + " "
	}
	return out
}
