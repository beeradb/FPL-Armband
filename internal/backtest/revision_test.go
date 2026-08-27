package backtest

import (
	"context"
	"fmt"
	"testing"

	"armband/internal/analysis"
)

// Stage-one checkpoint for unifying the two searches.
//
// In-season transfers run through RankSwaps/RankPairs; squad construction runs
// through Optimize. They solve the same problem under different constraints,
// and keeping two implementations is how the same two bugs got made twice —
// which is why swaps.go exists as the single transfer implementation in the
// first place.
//
// OptimizeRequest.MaxChanges makes Optimize able to express the in-season
// problem: best squad within k changes of the one you own. Before anything is
// rewired, this measures whether it actually finds squads at least as good as
// the transfer search does, for the same k, on the same squads, judged by the
// metric the transfer search itself optimises.
//
//	DIAG=1 go test ./internal/backtest -run TestDiagBoundedRevision -v -timeout 1800s
//
// XIValue is the yardstick, and benchWeight is pinned to zero so Optimize is
// scored on exactly what RankPairs maximises. Comparing them under different
// objectives would measure the objective change, not the search.
func TestDiagBoundedRevision(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	ctx := context.Background()

	cur, err := Load(ctx, cfg.CacheDir, "2024-25")
	if err != nil {
		t.Fatal(err)
	}
	prior, err := Load(ctx, cfg.CacheDir, "2023-24")
	if err != nil {
		t.Fatal(err)
	}

	// Build an opening squad, then revise it at a series of gameweeks.
	pe, _ := EngineAt(cur, prior, 0, SimConfig{Weights: cfg.Weights})
	opening, err := pe.Optimize(analysis.OptimizeRequest{
		Budget: analysis.DefaultBudget, MinMinutes: 600, MinExpectedMinutes: 55,
		BenchWeight: analysis.DefaultBenchWeight,
	})
	if err != nil {
		t.Fatal(err)
	}
	held := make([]int, 0, 15)
	for _, p := range opening.Players {
		held = append(held, p.ID)
	}

	fmt.Printf("\n%-4s %-3s %10s %10s %10s %9s\n",
		"GW", "k", "current", "swaps", "bounded", "delta")

	var wins, ties, losses int
	for _, gw := range []int{6, 10, 14, 18, 22, 26, 30} {
		e, _ := EngineAt(cur, prior, gw, SimConfig{Weights: cfg.Weights})

		byID := map[int]analysis.PlayerMetrics{}
		for _, m := range e.AllMetrics() {
			byID[m.ID] = m
		}
		squad := make([]analysis.PlayerMetrics, 0, len(held))
		for _, id := range held {
			if m, ok := byID[id]; ok {
				squad = append(squad, m)
			}
		}
		if len(squad) != 15 {
			continue
		}
		base := analysis.XIValue(squad)

		// Budget is the squad's own value plus the bank, not £100m.
		//
		// Optimize caps total cost at the budget it is given, which is right
		// when building from nothing and wrong for a squad you already own:
		// prices move all season, so a fifteen bought for £100m in August can be
		// worth more by Christmas, and capping it at £100m rejects every swap
		// that costs a penny more than it frees. RankSwaps has no such
		// constraint — it prices a move against the bank.
		value := 0
		for _, p := range squad {
			value += int(p.Price*10 + 0.5)
		}

		for _, k := range []int{1, 2, 3} {
			// What the existing transfer search reaches with k transfers.
			swapsBest := bestWithinK(e, squad, k)

			// What a bounded Optimize reaches with the same budget. Bench is
			// pinned to zero so both are judged on XIValue alone.
			// Matched candidate sets. RankSwaps is handed e.AllMetrics(), so
			// filtering Optimize's pool by minutes would measure the pool rather
			// than the search: the transfer side could buy players the squad
			// side was never shown. An unavailable player scores zero, so he is
			// unpickable on both sides regardless.
			got, err := e.Optimize(analysis.OptimizeRequest{
				Budget: value, MinMinutes: 0, MinExpectedMinutes: 0,
				BenchWeight: 0, CurrentSquad: held, MaxChanges: k,
			})
			if err != nil {
				t.Logf("GW%d k=%d: optimize: %v", gw, k, err)
				continue
			}
			bounded := analysis.XIValue(got.Players)

			// The bound must be honoured, or the comparison is meaningless.
			if n := changedFrom(held, got.Players); n > k {
				t.Errorf("GW%d k=%d: result changed %d players, over the budget",
					gw, k, n)
			}

			d := bounded - swapsBest
			switch {
			case d > 1e-6:
				wins++
			case d < -1e-6:
				losses++
			default:
				ties++
			}
			fmt.Printf("%-4d %-3d %10.4f %10.4f %10.4f %+9.4f\n",
				gw, k, base, swapsBest, bounded, d)
		}
	}
	fmt.Printf("\nbounded Optimize vs the transfer search: %d wins, %d ties, %d losses\n",
		wins, ties, losses)
}

// bestWithinK is the best XIValue the existing transfer search reaches using at
// most k transfers, taking the better of a funded pair and a run of single
// swaps — which is what the weekly policy actually does.
func bestWithinK(e *analysis.Engine, squad []analysis.PlayerMetrics, k int) float64 {
	best := analysis.XIValue(squad)
	cands := e.AllMetrics()

	if k >= 2 {
		pairs := analysis.RankPairs(analysis.NewSquadState(squad), cands, 0, k-1, 1)
		for _, p := range pairs {
			if p.Moves() <= k && base(squad)+p.Gain > best {
				best = base(squad) + p.Gain
			}
		}
	}

	// A run of single swaps, applied greedily as the weekly loop does.
	run := append([]analysis.PlayerMetrics(nil), squad...)
	for i := 0; i < k; i++ {
		sw := analysis.RankSwaps(analysis.NewSquadState(run), cands, 0)
		if len(sw) == 0 || sw[0].Gain <= 0 {
			break
		}
		for j := range run {
			if run[j].ID == sw[0].Out.ID {
				run[j] = sw[0].In
				break
			}
		}
		if v := analysis.XIValue(run); v > best {
			best = v
		}
	}
	return best
}

func base(squad []analysis.PlayerMetrics) float64 { return analysis.XIValue(squad) }

func changedFrom(held []int, got []analysis.PlayerMetrics) int {
	in := map[int]bool{}
	for _, id := range held {
		in[id] = true
	}
	n := 0
	for _, p := range got {
		if !in[p.ID] {
			n++
		}
	}
	return n
}
