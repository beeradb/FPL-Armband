package analysis

import (
	"fmt"
	"math/rand"
	"testing"
)

// A synthetic pool the size of a real filtered candidate pool, built
// deterministically so a benchmark measures the optimiser and not the weather.
//
// Deliberately full of ties: prices land on a £0.1m grid and scores on a coarse
// grid too, because tie density is what makes the stable-sort order load-bearing
// (see TestStableTieOrderFollowsInputOrder). A pool of distinct floats would
// benchmark the easy case and hide the trap.
func benchPool(n int) []PlayerMetrics {
	rng := rand.New(rand.NewSource(20260810))
	teams := make([]string, 20)
	for i := range teams {
		teams[i] = fmt.Sprintf("T%02d", i)
	}
	quota := []string{"GKP", "DEF", "DEF", "DEF", "DEF", "DEF", "MID", "MID",
		"MID", "MID", "MID", "FWD", "FWD"}

	out := make([]PlayerMetrics, 0, n)
	for i := 0; i < n; i++ {
		pos := quota[i%len(quota)]
		// Price on the real £0.1m grid, 3.9 to 15.0.
		price := 39 + rng.Intn(112)
		// Score on a 0.05 grid, so ties are common.
		score := float64(rng.Intn(160)) * 0.05
		out = append(out, PlayerMetrics{
			ID:              i + 1,
			Name:            fmt.Sprintf("p%04d", i+1),
			Team:            teams[rng.Intn(len(teams))],
			Position:        pos,
			Price:           float64(price) / 10,
			Score:           score,
			Status:          "available",
			ExpectedMinutes: float64(rng.Intn(91)),
			StartShare:      float64(rng.Intn(101)) / 100,
			Minutes:         rng.Intn(3000),
		})
		out[len(out)-1].ValueScore = out[len(out)-1].Score / out[len(out)-1].Price
	}
	return out
}

// benchSquad takes a legal fifteen out of a pool: 2/5/5/3 with at most three
// per club, cheap enough to leave the search somewhere to go.
func benchSquad(pool []PlayerMetrics) []PlayerMetrics {
	need := map[string]int{"GKP": 2, "DEF": 5, "MID": 5, "FWD": 3}
	clubs := map[string]int{}
	var out []PlayerMetrics
	for _, p := range pool {
		if need[p.Position] == 0 || clubs[p.Team] >= MaxPerClub {
			continue
		}
		need[p.Position]--
		clubs[p.Team]++
		out = append(out, p)
		if len(out) == SquadSize {
			break
		}
	}
	return out
}

func BenchmarkBestXIWith(b *testing.B) {
	squad := benchSquad(benchPool(600))
	if len(squad) != SquadSize {
		b.Fatalf("fixture squad has %d players", len(squad))
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		xi, bench, form := bestXIWith(squad, nil)
		if len(xi) != 11 || len(bench) != 4 || form == "" {
			b.Fatal("degenerate result")
		}
	}
}

// BenchmarkObjectiveWith is the cold entry point, which allocates a scratch per
// call. It is what an occasional caller outside the search pays.
func BenchmarkObjectiveWith(b *testing.B) {
	squad := benchSquad(benchPool(600))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if objectiveWith(squad, DefaultBenchWeight, nil, false) == 0 {
			b.Fatal("zero objective")
		}
	}
}

// BenchmarkObjectiveScratch is what the local search actually pays: the same
// evaluation with the scratch reused, which is the whole point of it existing.
// The gap between this and BenchmarkObjectiveWith is the cost of the scratch.
func BenchmarkObjectiveScratch(b *testing.B) {
	squad := benchSquad(benchPool(600))
	sc := &xiScratch{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if sc.objective(squad, DefaultBenchWeight, nil, false) == 0 {
			b.Fatal("zero objective")
		}
	}
}

// BenchmarkPolish is the honest proxy for Optimize: the profile puts the time in
// the local search, and polish is that search. Optimize itself cannot be
// benchmarked without the live API, since AllMetrics runs the whole scoring
// pipeline over a bootstrap.
func BenchmarkPolish(b *testing.B) {
	for _, n := range []int{200, 600} {
		b.Run(fmt.Sprintf("pool%d", n), func(b *testing.B) {
			pool := benchPool(n)
			squad := benchSquad(pool)
			spend := 0
			for _, p := range squad {
				spend += priceUnits(p)
			}
			budget := spend + 60
			e := &Engine{}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				got, score, _ := e.polish(squad, pool, budget, DefaultBenchWeight,
					false, map[int]bool{}, nil, true, changeBudget{})
				if len(got) != SquadSize || score == 0 {
					b.Fatal("degenerate result")
				}
			}
		})
	}
}
