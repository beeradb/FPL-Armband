package backtest

import (
	"fmt"
	"os"
	"sort"
	"testing"

	"armband/internal/analysis"
)

// TestDiagOptimizerDeterminismPrevalence answers the question
// TestDiagOptimizerIsNotDeterministic raises and cannot settle: **how much of the
// record is affected.**
//
//	DIAG=1 go test ./internal/backtest -run TestDiagOptimizerDeterminismPrevalence -v -timeout 120m
//
// The defect is that Optimize returns different fifteens from byte-identical
// inputs on one engine. That contradicts AGENTS.md's standing claim that "the
// replay is deterministic: the same inputs always give the same output", and
// byte-identity is the strongest form of evidence this project uses — "HOLD is
// byte-identical across all six settings", "0.0 and 0.4 return byte-identical
// seasons", "not one bank in 24 season-paths". If a third of Optimize calls
// diverged, none of those could have been reported.
//
// So the number that decides how much has to be re-run is not the size of the
// spread but **the share of landscapes on which it appears at all**. That is a
// count over a census of the landscapes the record is measured on, so it needs no
// significance machinery.
//
// # The landscape grid
//
// One landscape is one (season, entry gameweek, budget). Seasons and entry
// gameweeks are the record's own axes, reproduced exactly. Budget is added as a
// third because AGENTS.md establishes it as the landscape *generator* — "every
// £0.5m reparameterises the whole knapsack", where sweeping a scoring weight
// returns byte-identical optima — so a grid without it would under-sample.
//
// Each landscape is built once and Optimize is called `runs` times **on that one
// engine**, which is what isolates the optimiser from the data and the engine
// build.
func TestDiagOptimizerDeterminismPrevalence(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)
	pairs := loadPairs(t, cfg)

	runs := 8
	budgets := []int{990, 1000, 1010}

	type outcome struct {
		season   string
		start    int
		budget   int
		distinct int
		lo, hi   float64
		majority int // how many of `runs` gave the most common answer
	}
	var results []outcome

	for _, pair := range pairs {
		for _, start := range sweepStarts() {
			sim := sweepConfig(cfg, start, false)
			boot, fx := PointInTime(pair.Cur, pair.Prior, start-1)
			e := analysis.NewEngineFull(boot, fx, sim.Weights,
				analysis.Congestion{}, analysis.RoleRisk{})
			e.Priors = newPriorIndexMulti([]*Season{pair.Prior}, sim.PriorHalfLife)
			e.Recent = newRecentIndexWith(pair.Cur, start-1,
				sim.minutesHalfLife(), sim.Weights.RateHalfLife)

			for _, budget := range budgets {
				seen := map[float64]int{}
				var lo, hi float64
				bad := false
				for i := 0; i < runs; i++ {
					sq, err := e.Optimize(analysis.OptimizeRequest{
						Budget: budget, MinMinutes: 600, MinExpectedMinutes: 55,
						BenchWeight: sim.openingBenchWeight(),
					})
					if err != nil {
						t.Logf("%s @GW%d £%.1fm: %v", pair.Name, start,
							float64(budget)/10, err)
						bad = true
						break
					}
					s := sq.XIScore
					if seen[s] == 0 {
						if len(seen) == 0 || s < lo {
							lo = s
						}
						if len(seen) == 0 || s > hi {
							hi = s
						}
					}
					seen[s]++
				}
				if bad {
					continue
				}
				maj := 0
				for _, n := range seen {
					if n > maj {
						maj = n
					}
				}
				results = append(results, outcome{pair.Name, start, budget,
					len(seen), lo, hi, maj})
			}
		}
	}

	unstable := 0
	var spreads []float64
	for _, r := range results {
		if r.distinct > 1 {
			unstable++
			spreads = append(spreads, r.hi-r.lo)
		}
	}

	t.Logf("landscapes: %d (%d seasons x %d entry gameweeks x %d budgets), "+
		"%d identical Optimize calls each on one engine",
		len(results), len(pairs), len(sweepStarts()), len(budgets), runs)
	t.Logf("UNSTABLE: %d of %d landscapes (%.1f%%)",
		unstable, len(results), 100*float64(unstable)/float64(len(results)))
	if len(spreads) > 0 {
		sort.Float64s(spreads)
		var sum float64
		for _, s := range spreads {
			sum += s
		}
		t.Logf("spread on the unstable ones, XIScore pts/gw: "+
			"min %.4f  median %.4f  mean %.4f  max %.4f  (x38 for a season: "+
			"median %.1f, max %.1f)",
			spreads[0], median(spreads), sum/float64(len(spreads)),
			spreads[len(spreads)-1],
			38*median(spreads), 38*spreads[len(spreads)-1])
	}

	// By entry gameweek and by season, because the record's cells are indexed on
	// exactly those and a defect concentrated in one regime affects a different
	// set of claims from one spread evenly.
	byStart := map[int][2]int{}
	bySeason := map[string][2]int{}
	for _, r := range results {
		a, b := byStart[r.start], bySeason[r.season]
		a[1]++
		b[1]++
		if r.distinct > 1 {
			a[0]++
			b[0]++
		}
		byStart[r.start], bySeason[r.season] = a, b
	}
	for _, s := range sweepStarts() {
		t.Logf("  entry GW%-3d %d/%d unstable", s, byStart[s][0], byStart[s][1])
	}
	for _, p := range pairs {
		t.Logf("  %s      %d/%d unstable", p.Name,
			bySeason[p.Name][0], bySeason[p.Name][1])
	}

	fmt.Fprintln(os.Stderr, "season,start_gw,budget_tenths,distinct,lo,hi,spread,majority,runs")
	for _, r := range results {
		fmt.Fprintf(os.Stderr, "%s,%d,%d,%d,%.6f,%.6f,%.6f,%d,%d\n",
			r.season, r.start, r.budget, r.distinct, r.lo, r.hi, r.hi-r.lo,
			r.majority, runs)
	}
}
