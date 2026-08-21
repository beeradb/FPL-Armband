package analysis

import "testing"

// The bench boost chip pays all fifteen, so a squad built for it is a different
// squad: the four bench slots stop being a hedge priced by how often a
// substitute is used and become four more starters.
//
// These pin that the flag actually reaches the objective. A wired-but-inert
// option is the failure this codebase has hit repeatedly — see the engine that
// was given recency and the config field whose migration never fired — and it
// is especially easy here, because the ordinary bench weight is small enough
// that a broken flag still produces a plausible-looking fifteen.

// TestBenchBoostValuesTheWholeBench — under the chip, the objective must count
// every bench player at full value rather than at BenchWeight times a slot
// probability.
func TestBenchBoostValuesTheWholeBench(t *testing.T) {
	xi := make([]PlayerMetrics, 11)
	for i := range xi {
		xi[i] = PlayerMetrics{ID: i + 1, Position: "MID", Score: 5, StartShare: 0.9}
	}
	xi[0].Position = "GKP"
	bench := []PlayerMetrics{
		{ID: 101, Position: "DEF", Score: 4, StartShare: 0.9},
		{ID: 102, Position: "MID", Score: 3, StartShare: 0.9},
		{ID: 103, Position: "FWD", Score: 2, StartShare: 0.9},
		{ID: 104, Position: "GKP", Score: 1, StartShare: 0.9},
	}

	boosted := benchValue(xi, bench, DefaultBenchWeight, true)
	if want := 4.0 + 3 + 2 + 1; boosted != want {
		t.Errorf("boosted bench = %.4f, want %.4f (every bench player at full value)", boosted, want)
	}

	ordinary := benchValue(xi, bench, DefaultBenchWeight, false)
	if ordinary >= boosted {
		t.Errorf("ordinary bench %.4f is not below the boosted %.4f — the flag did nothing",
			ordinary, boosted)
	}
}

// TestBenchBoostBuildsARealBench is the one that matters. The ordinary
// objective converges on eleven good players and four who cannot cover, which
// is why measuring the chip on a normally-built squad measures a floor. Asking
// for a bench boost squad must actually spend money on the bench.
func TestBenchBoostBuildsARealBench(t *testing.T) {
	if testing.Short() {
		t.Skip("solves the squad twice")
	}
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	skipDuringLiveGW1Gap(t, e)

	ordinary, err := e.Optimize(OptimizeRequest{Budget: DefaultBudget})
	if err != nil {
		t.Fatalf("optimize: %v", err)
	}
	boosted, err := e.Optimize(OptimizeRequest{Budget: DefaultBudget, BenchBoost: true})
	if err != nil {
		t.Fatalf("optimize with bench boost: %v", err)
	}

	benchScore := func(sq *Squad) float64 {
		var s float64
		for _, p := range sq.Bench {
			s += p.Score
		}
		return s
	}
	benchCost := func(sq *Squad) float64 {
		var c float64
		for _, p := range sq.Bench {
			c += p.Price
		}
		return c
	}

	o, b := benchScore(ordinary), benchScore(boosted)
	t.Logf("ordinary bench: %.2f pts, £%.1fm  |  boosted bench: %.2f pts, £%.1fm",
		o, benchCost(ordinary), b, benchCost(boosted))

	if b <= o {
		t.Errorf("bench boost squad's bench scores %.2f against the ordinary %.2f — "+
			"the flag reached the objective but changed nothing", b, o)
	}
	// The chip is worth what the bench is worth, so a boosted build should be
	// spending real money there rather than shuffling fodder.
	if benchCost(boosted) <= benchCost(ordinary) {
		t.Errorf("boosted bench costs £%.1fm against the ordinary £%.1fm — expected it to buy quality",
			benchCost(boosted), benchCost(ordinary))
	}
}
