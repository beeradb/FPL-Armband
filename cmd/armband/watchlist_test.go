package main

import (
	"testing"

	"armband/internal/analysis"
)

// TestWatchlistForFiltersThenSorts pins the equivalence this file's fix to
// watchlistFor depends on: filtering the whole pool before sorting must
// produce the same rows, in the same order, as the old sort-then-filter did.
// It cannot replay the old code (deleted), so it instead checks the two
// properties whose failure would mean the equivalence broke:
//
//  1. Every non-owned, non-excluded player in the pool appears exactly once —
//     nothing was silently capped. A future change that truncates or
//     early-returns from the filter loop (the regression the code comment
//     warns against) would shrink w.Rows below the pool's own count and this
//     catches it.
//  2. w.Rows is sorted by Score, non-increasing — the property the sort
//     exists to guarantee, checked on the actual (usually non-trivial) score
//     spread of the fixture pool rather than on hand-built ties.
func TestWatchlistForFiltersThenSorts(t *testing.T) {
	s := fixtureServer(t)
	e := s.engine

	sq, err := e.Optimize(analysis.OptimizeRequest{Budget: analysis.DefaultBudget})
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}

	w := watchlistFor(e, *sq, nil, nil, 0)

	owned := map[int]bool{}
	for _, p := range sq.Players {
		owned[p.ID] = true
	}
	wantCount := 0
	for _, m := range e.AllMetrics() {
		if !owned[m.ID] {
			wantCount++
		}
	}
	if w.Count != wantCount {
		t.Fatalf("watchlist has %d rows, want %d (every non-owned player in the pool, uncapped)",
			w.Count, wantCount)
	}
	if got := len(w.Rows); got != wantCount {
		t.Fatalf("len(w.Rows) = %d, want %d", got, wantCount)
	}

	for i := 1; i < len(w.Rows); i++ {
		if w.Rows[i].Player.Score > w.Rows[i-1].Player.Score {
			t.Fatalf("w.Rows not sorted by Score descending at index %d: %.3f > %.3f",
				i, w.Rows[i].Player.Score, w.Rows[i-1].Player.Score)
		}
	}
}
