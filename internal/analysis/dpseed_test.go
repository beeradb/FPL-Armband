package analysis

import (
	"testing"
)

func seedPool(t *testing.T) (*Engine, []PlayerMetrics) {
	t.Helper()
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	// Every squad-building test downstream needs enough of the league to carry a
	// score; after one gameweek only the players who happened to play do.
	skipUntilLiveEvidence(t, e, corroboratingMatches)
	var pool []PlayerMetrics
	for _, m := range e.AllMetrics() {
		if m.Status == "injured" || m.Status == "suspended" || m.Status == "unavailable" {
			continue
		}
		pool = append(pool, m)
	}
	if len(pool) < 100 {
		t.Skip("candidate pool too small")
	}
	return e, pool
}

// TestDPSeedsAreLegalAfterRepair checks the seeds are usable. The DP relaxes the
// club limit because a cost-indexed knapsack cannot express it, so repairClubs
// has to be able to fix what it produces.
func TestDPSeedsAreLegalAfterRepair(t *testing.T) {
	e, pool := seedPool(t)
	seeds := e.dpSeeds(pool, DefaultBudget, nil)
	if len(seeds) == 0 {
		t.Fatal("no DP seeds produced")
	}
	var legal int
	for i, s := range seeds {
		if len(s) != SquadSize {
			t.Errorf("seed %d has %d players, want %d", i, len(s), SquadSize)
		}
		if r := repairClubs(s, pool, DefaultBudget); r != nil {
			if !squadIsLegal(r, DefaultBudget) {
				t.Errorf("seed %d survived repair but is still illegal", i)
			}
			legal++
		}
	}
	if legal == 0 {
		t.Error("no seed could be repaired into a legal squad")
	}
	t.Logf("%d seeds, %d legal after repair", len(seeds), legal)
}

// TestDPSeedsCoverMultipleFormations guards the point of the exercise: the
// seeds must span formations, because the structural question the local search
// missed was a formation change, not a player swap.
func TestDPSeedsCoverMultipleFormations(t *testing.T) {
	e, pool := seedPool(t)
	forms := map[string]bool{}
	for _, s := range e.dpSeeds(pool, DefaultBudget, nil) {
		if r := repairClubs(s, pool, DefaultBudget); r != nil {
			_, _, f := bestXI(r)
			forms[f] = true
		}
	}
	if len(forms) < 3 {
		t.Errorf("seeds covered only %d formations (%v); expected the search to span the shape space", len(forms), forms)
	}
	t.Logf("formations covered: %v", forms)
}

// TestOptimizerIsNeverWorseThanAnExactSeed is the standing guarantee. The
// optimiser is a local search, so it has no optimality proof of its own — but
// it must never return less than an exact per-formation DP solution, which is
// what seeding it with those solutions buys.
func TestOptimizerIsNeverWorseThanAnExactSeed(t *testing.T) {
	e, pool := seedPool(t)
	sq, err := e.Optimize(OptimizeRequest{Budget: DefaultBudget})
	if err != nil {
		t.Fatalf("optimize: %v", err)
	}
	got := objective(sq.Players, e.Weights.BenchWeight, false)

	for i, seed := range e.dpSeeds(pool, DefaultBudget, nil) {
		r := repairClubs(seed, pool, DefaultBudget)
		if r == nil || !squadIsLegal(r, DefaultBudget) {
			continue
		}
		if s := objective(r, e.Weights.BenchWeight, false); s > got+1e-6 {
			t.Errorf("DP seed %d scores %.4f, optimiser returned %.4f — the seed was not used",
				i, s, got)
		}
	}
	t.Logf("optimiser objective %.4f", got)
}

func TestOptimizerResultIsLegal(t *testing.T) {
	e, _ := seedPool(t)
	sq, err := e.Optimize(OptimizeRequest{Budget: DefaultBudget})
	if err != nil {
		t.Fatalf("optimize: %v", err)
	}
	if !squadIsLegal(sq.Players, DefaultBudget) {
		t.Fatalf("optimiser returned an illegal squad: %d players, £%.1fm, clubs %v",
			len(sq.Players), sq.TotalCost, sq.ClubCounts)
	}
}

// TestLockedPlayersSurviveDPSeeding checks that a locked player is still in the
// squad after seeding. Seeds pre-place locks, but repairClubs and the local
// search can both move players afterwards, so the result is verified rather
// than assumed.
func TestLockedPlayersSurviveDPSeeding(t *testing.T) {
	e, pool := seedPool(t)
	var lock PlayerMetrics
	for _, p := range pool {
		if p.Position == "FWD" && p.Price >= 9.0 && p.Score > 0 {
			lock = p
			break
		}
	}
	if lock.ID == 0 {
		t.Skip("no expensive forward to lock")
	}
	sq, err := e.Optimize(OptimizeRequest{Budget: DefaultBudget, LockIDs: []int{lock.ID}})
	if err != nil {
		t.Fatalf("optimize with lock: %v", err)
	}
	for _, p := range sq.Players {
		if p.ID == lock.ID {
			return
		}
	}
	t.Errorf("locked player %s (£%.1fm) was dropped from the squad", lock.Name, lock.Price)
}

// TestFrontierKeepsTheBestAtEachPrice checks the pool trim cannot discard a
// player who is strictly better than one it keeps.
func TestFrontierKeepsTheBestAtEachPrice(t *testing.T) {
	_, pool := seedPool(t)
	var defs []PlayerMetrics
	for _, p := range pool {
		if p.Position == "DEF" {
			defs = append(defs, p)
		}
	}
	kept := frontier(defs, 2)
	best := map[int]float64{}
	for _, p := range defs {
		u := priceUnits(p)
		if p.Score > best[u] {
			best[u] = p.Score
		}
	}
	seen := map[int]bool{}
	for _, p := range kept {
		if p.Score == best[priceUnits(p)] {
			seen[priceUnits(p)] = true
		}
	}
	for u, s := range best {
		if s > 0 && !seen[u] {
			t.Errorf("frontier dropped the best defender at £%.1fm (score %.2f)", float64(u)/10, s)
		}
	}
}
