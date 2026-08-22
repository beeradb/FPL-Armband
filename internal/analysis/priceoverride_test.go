package analysis

import "testing"

// TestPriceOverrideReshapesTheSquad — the analysis layer can ask what a squad
// looks like after a price change that has not happened yet.
//
// The asymmetry measured in TestDiagBudgetJitter is what makes this worth
// having: taking money away reshapes a squad far more often than adding it,
// because the optimiser already spends everything it has. So the question the
// agent needs answered is "what can I no longer afford", and that needs
// per-player prices rather than a budget total.
func TestPriceOverrideReshapesTheSquad(t *testing.T) {
	e := testEngine(t)
	// This test's whole premise is that the budget BINDS: repricing the best
	// player upward only forces him out if the money he now costs was buying
	// something. During the live GW1 gap this package's engines cannot load a
	// prior season (see skipDuringLiveGW1Gap), so every player at a club that has
	// not kicked off scores exactly zero, there is very little in the field worth
	// paying for, and the optimiser leaves £12m in the bank — at which point a
	// £6.0m rise on the best defender in the pool costs nothing and he correctly
	// stays.
	//
	// It used to pass in that window, and for a bad reason: the pool was gutted
	// down to bench fodder by the unscaled minutes floor, so `best` was a £4.0m
	// body who left the pool ALTOGETHER once the reprice took him past
	// BenchFodderPrice. The assertion held because the bug held.
	skipDuringLiveGW1Gap(t, e)
	req := OptimizeRequest{
		Budget: DefaultBudget, MinMinutes: 600, MinExpectedMinutes: 55,
		BenchWeight: DefaultBenchWeight,
	}
	base, err := e.Optimize(req)
	if err != nil {
		t.Skipf("no squad: %v", err)
	}
	if len(base.Players) != 15 {
		t.Fatalf("got %d players", len(base.Players))
	}

	// Price the squad's best player far out of reach and he must leave.
	best := base.Players[0]
	for _, p := range base.Players {
		if p.Score > best.Score {
			best = p
		}
	}
	req.PriceOverride = map[int]int{best.ID: int(best.Price*10) + 60}
	after, err := e.Optimize(req)
	if err != nil {
		t.Fatalf("re-optimise: %v", err)
	}
	for _, p := range after.Players {
		if p.ID == best.ID {
			t.Errorf("%s kept at £%.1fm after being repriced to £%.1fm",
				p.Name, p.Price, float64(req.PriceOverride[best.ID])/10)
		}
	}

	// And the override must not leak: the engine is shared across concurrent
	// tool calls, so a per-request price cannot survive into the next request.
	again, err := e.Optimize(OptimizeRequest{
		Budget: DefaultBudget, MinMinutes: 600, MinExpectedMinutes: 55,
		BenchWeight: DefaultBenchWeight,
	})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range again.Players {
		if p.ID == best.ID {
			found = true
			if p.Price != best.Price {
				t.Errorf("%s priced £%.1fm after the scenario, want £%.1fm",
					p.Name, p.Price, best.Price)
			}
		}
	}
	if !found {
		t.Errorf("%s not restored to the squad once the scenario price is dropped", best.Name)
	}
}
