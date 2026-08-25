package analysis

import (
	"sort"
	"testing"
)

// TestNoPremiumSquadBeatsTheOptimum is the guard against the search failing to
// reach an expensive player.
//
// Buying a £15.5m striker is not a swap. It is a restructure — sell three
// players, buy one, downgrade a goalkeeper — and every intermediate step is
// worse than where you started, so a hill-climber will not take it however many
// swaps it is allowed. The DP seeding exists to jump straight there, and this
// test checks that it works rather than assuming it.
//
// Method: lock each expensive player in turn and re-solve. If any locked squad
// beats the unconstrained one, the unconstrained search failed to find a squad
// it should have found.
func TestNoPremiumSquadBeatsTheOptimum(t *testing.T) {
	if testing.Short() {
		t.Skip("re-solves the squad once per premium")
	}
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	skipDuringLiveGW1Gap(t, e)

	free, err := e.Optimize(OptimizeRequest{Budget: DefaultBudget})
	if err != nil {
		t.Fatalf("optimize: %v", err)
	}
	best := objective(free.Players, e.Weights.BenchWeight, false)
	t.Logf("unconstrained: %.4f  (%s, captain %s £%.1fm)",
		best, free.Formation, free.Captain.Name, free.Captain.Price)

	// The most expensive players in the game — the ones a local search is least
	// able to walk to.
	var prem []PlayerMetrics
	for _, m := range e.AllMetrics() {
		if m.Price >= 9.0 && m.Score > 0 && m.Status == "available" {
			prem = append(prem, m)
		}
	}
	sort.SliceStable(prem, func(i, j int) bool { return prem[i].Price > prem[j].Price })
	if len(prem) > 8 {
		prem = prem[:8]
	}
	if len(prem) == 0 {
		t.Skip("no premium players in the pool")
	}

	owned := map[int]bool{}
	for _, p := range free.Players {
		owned[p.ID] = true
	}

	for _, p := range prem {
		sq, err := e.Optimize(OptimizeRequest{Budget: DefaultBudget, LockIDs: []int{p.ID}})
		if err != nil {
			t.Errorf("optimize locking %s: %v", p.Name, err)
			continue
		}
		if !squadIsLegal(sq.Players, DefaultBudget) {
			t.Errorf("locking %s produced an illegal squad", p.Name)
			continue
		}
		got := objective(sq.Players, e.Weights.BenchWeight, false)
		mark := ""
		if owned[p.ID] {
			mark = "  (already owned)"
		}
		t.Logf("  lock %-14s £%5.1fm -> %.4f  (%+.4f vs free)%s", p.Name, p.Price, got, got-best, mark)

		if got > best+1e-6 {
			t.Errorf("forcing %s (£%.1fm) scores %.4f, beating the unconstrained optimum %.4f — "+
				"the search cannot reach a squad built around him", p.Name, p.Price, got, best)
		}
	}
}

// TestDPSeedsReachAPremiumWhenItDominates is the complementary direction. The
// test above proves no premium squad beats the answer; this proves the seeding
// can actually construct one, so that result is a real comparison rather than
// two searches failing the same way.
//
// The most expensive player's score is rewritten so that omitting him is
// obviously wrong, and the seeds must then contain him.
func TestDPSeedsReachAPremiumWhenItDominates(t *testing.T) {
	e, pool := seedPool(t)

	target := -1
	for i, p := range pool {
		if p.Score > 0 && (target < 0 || p.Price > pool[target].Price) {
			target = i
		}
	}
	if target < 0 || pool[target].Price < 9.0 {
		t.Skip("no premium player in the pool")
	}

	boosted := append([]PlayerMetrics(nil), pool...)
	boosted[target].Score = 40.0
	want := boosted[target]

	seeds := e.dpSeeds(boosted, DefaultBudget, nil)
	if len(seeds) == 0 {
		t.Fatal("no seeds produced")
	}
	for _, s := range seeds {
		for _, p := range s {
			if p.ID == want.ID {
				t.Logf("seeded a squad containing %s (£%.1fm)", want.Name, want.Price)
				return
			}
		}
	}
	t.Errorf("%s (£%.1fm) was worth 40.0 points a gameweek and no DP seed included him — "+
		"the seeding cannot construct a premium-built squad", want.Name, want.Price)
}

// TestSeedBudgetLeavesRoomForThePremiums pins the arithmetic that broke once:
// the bench reservation must take the cheapest players who could fill those
// slots, not the most expensive. Reserving from the wrong end silently cut tens
// of millions off the XI budget and made every seed too poor to buy a premium,
// with no visible failure — the seeds were simply never the best answer.
func TestSeedBudgetLeavesRoomForThePremiums(t *testing.T) {
	e, pool := seedPool(t)
	// The ONE seedPool caller with an evidence dependency, so the guard lives
	// here rather than in the helper — see seedPool's own note. This asserts the
	// best-funded seed SPENDS its budget, and it cannot until enough of the
	// league carries a score worth buying: measured 2026-08-25, one gameweek in,
	// the best-funded seed reached only £88.5m of £100.0m.
	skipUntilLiveEvidence(t, e, corroboratingMatches)
	var dearest float64
	for _, p := range pool {
		if p.Score > 0 && p.Price > dearest {
			dearest = p.Price
		}
	}

	budget := float64(DefaultBudget) / 10
	var most float64
	var canAffordDearest bool
	for _, s := range e.dpSeeds(pool, DefaultBudget, nil) {
		var spend float64
		for _, p := range s {
			spend += p.Price
			if p.Price == dearest {
				canAffordDearest = true
			}
		}
		if spend > most {
			most = spend
		}
	}

	// A seed leaving a couple of million unspent is fine — the DP stops paying
	// when nothing better is available at the price. Leaving a fifth of the
	// budget on the table is the signature of the bug: the bench reservation ate
	// it before the eleven could be chosen.
	if most < budget*0.95 {
		t.Errorf("the best-funded seed spends only £%.1fm of £%.1fm — the bench reservation "+
			"is eating the budget, which starves the seeds of any premium", most, budget)
	}
	t.Logf("best-funded seed spends £%.1fm of £%.1fm; dearest scoring player £%.1fm (in a seed: %v)",
		most, budget, dearest, canAffordDearest)
}
