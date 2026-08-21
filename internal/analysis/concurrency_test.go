package analysis

import (
	"context"
	"sync"
	"testing"
	"time"

	"armband/internal/fpl"
)

// The SDK's tool runner executes tool calls concurrently in an errgroup, so two
// searches in the same turn both drive Metrics over the whole player pool at
// once. Lazily building the name-lookup maps under a plain nil check made that a
// "concurrent map writes" fatal error — which is not recoverable, so it took the
// whole process down mid-run. Run with -race to see the underlying data race.
func TestEngineScoresConcurrently(t *testing.T) {
	c := fpl.New(t.TempDir(), 24*time.Hour, 24*time.Hour)
	ctx := context.Background()
	boot, err := c.Bootstrap(ctx)
	if err != nil {
		t.Skipf("FPL API unreachable: %v", err)
	}
	fx, err := c.Fixtures(ctx)
	if err != nil {
		t.Skipf("FPL API unreachable: %v", err)
	}

	w := DefaultWeights()
	rr := DefaultRoleRisk()
	// Both lists must be non-empty or the lazy builders never run and the test
	// passes without exercising anything.
	if len(w.RestPlayers) == 0 || len(rr.ConfirmedStarters) == 0 {
		rr.ConfirmedStarters = []string{"Morgan Rogers"}
		if len(w.RestPlayers) == 0 {
			w.RestPlayers = []string{"Declan Rice"}
		}
	}
	e := NewEngineFull(boot, fx, w, DefaultCongestion(), rr)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if got := len(e.AllMetrics()); got == 0 {
				t.Error("AllMetrics returned nothing")
			}
		}()
	}
	wg.Wait()
}

// update_competition_status rewrites the congestion model mid-run while other
// tools are still scoring players off it.
func TestCompetitionUpdateIsSafeDuringScoring(t *testing.T) {
	c := fpl.New(t.TempDir(), 24*time.Hour, 24*time.Hour)
	ctx := context.Background()
	boot, err := c.Bootstrap(ctx)
	if err != nil {
		t.Skipf("FPL API unreachable: %v", err)
	}
	fx, err := c.Fixtures(ctx)
	if err != nil {
		t.Skipf("FPL API unreachable: %v", err)
	}
	e := NewEngineFull(boot, fx, DefaultWeights(), DefaultCongestion(), DefaultRoleRisk())

	var wg sync.WaitGroup
	stop := make(chan struct{})

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					e.AllMetrics()
				}
			}
		}()
	}

	for i := 0; i < 20; i++ {
		e.SetCompetitionWindows("ARS", true, []CompetitionWindow{
			{Competition: "UCL", Start: "2026-09-08"},
		}, "2026-08-03")
		e.SetCompetitionWindows("ARS", true, nil, "2026-08-03")
	}
	close(stop)
	wg.Wait()
}
