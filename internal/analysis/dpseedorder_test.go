package analysis

import (
	"fmt"
	"testing"
)

// TestSeedOrderIsDeterministic pins the optimiser's non-determinism shut. It began
// as TestDiagDPSeedOrderIsNotDeterministic, which located the defect and was the
// positive control for every census of how often it bites.
//
// # Why a control was needed
//
// TestDiagOptimizerIsNotDeterministic (internal/backtest) recorded Optimize
// returning two different fifteens from byte-identical inputs. A census over 72
// replay landscapes then found none unstable — and a clean null from a harness
// never shown to detect the effect is exactly the failure this project records
// as "measuring nothing while looking like a real result". Worse, the recorded
// reproducer stopped reproducing when an unrelated pool change landed, so the
// obvious control evaporated.
//
// This one cannot evaporate, because it observes the defect itself rather than a
// consequence of it that a landscape may or may not carry.
//
// # The defect
//
// seedFor fills each DP seed's bench with `for pos, quota := range squadQuota`,
// and squadQuota is a map. Go randomises map iteration, so the four bench players
// are appended to the seed in a different order on every call. The *set* is
// unaffected — each position draws only from its own candidates — but the seed is
// a []PlayerMetrics and its order is read downstream:
//
//   - repairClubs scans the squad in slice order to pick which player to drop
//     from an over-limit club, so a different order can drop a different player
//     and return a genuinely different fifteen;
//   - the seed-ranking loop in Optimize compares objectiveWith(seed) across
//     formations with a bare `>`, and the objective sums scores in an order
//     derived from the squad slice — float addition is not associative, so the
//     same eleven summed two ways can land a ULP apart and flip which seed wins.
//
// Either way the local search then starts somewhere else and converges somewhere
// else. On 2023-24 pre-season that was worth 0.437 pts/gw, about 17 points a
// season, with 5 of 15 players differing.
//
// A second, independent site sits in repairClubs itself: `for team, n := range
// counts` picks which over-limit club to repair by map order, and instrumentation
// showed two clubs simultaneously over the limit on real seeds — so the choice is
// genuinely map-ordered rather than only nominally so. It is weaker than the seed
// order, because the loop keeps going until no club is over and repairing A then
// B usually lands where repairing B then A does; making the seed order
// deterministic left the reproducer stable in all six runs without touching it.
// Fix it anyway: "usually converges" is not determinism, and the exception is
// exactly the case where repairing one club changes what is affordable for the
// next.
//
// # What this asserts — INVERTED, now that the fix has landed
//
// It was written DIAG-gated, reporting rather than failing, because it documented
// a defect that was still present. The fix landed in the same commit that inverted
// it: `seedFor`'s bench fill iterates `posNames`, and `repairClubs` chooses the
// over-limit club by name. So this now runs unconditionally and FAILS if either the
// seed order or the seed set ever varies again.
//
// It observes the DEFECT (seed order) rather than a CONSEQUENCE of it (the final
// fifteen), and that distinction is the point. The original reproducer watched the
// squad, and it stopped reproducing mid-investigation when an unrelated pool fix
// reset the landscape — leaving a census that returned a clean 0/72 from a harness
// never shown to detect anything, which is this package's signature failure. Watch
// the cause and no future pool change can silence it.
func TestSeedOrderIsDeterministic(t *testing.T) {
	e, pool := seedPool(t)

	const runs = 12
	orderSeen := map[string]bool{}
	setSeen := map[string]bool{}
	for i := 0; i < runs; i++ {
		seeds := e.dpSeeds(pool, DefaultBudget, nil)
		if len(seeds) == 0 {
			t.Fatal("no DP seeds produced")
		}
		order, set := "", ""
		for _, s := range seeds {
			ids := make([]int, 0, len(s))
			for _, p := range s {
				ids = append(ids, p.ID)
			}
			order += fmt.Sprint(ids) + "|"
			sorted := append([]int(nil), ids...)
			for a := range sorted {
				for b := a + 1; b < len(sorted); b++ {
					if sorted[b] < sorted[a] {
						sorted[a], sorted[b] = sorted[b], sorted[a]
					}
				}
			}
			set += fmt.Sprint(sorted) + "|"
		}
		orderSeen[order] = true
		setSeen[set] = true
	}

	t.Logf("%d identical dpSeeds calls: %d distinct seed *orders*, %d distinct seed *sets*",
		runs, len(orderSeen), len(setSeen))

	if len(setSeen) != 1 {
		t.Errorf("the seed SETS vary across %d identical calls (%d distinct). That is a "+
			"stronger defect than the one this test was written for — it means the DP "+
			"itself is order-dependent, not only its output ordering.", runs, len(setSeen))
	}
	if len(orderSeen) != 1 {
		t.Errorf("seedFor returned the same fifteen in %d different slice orders across "+
			"%d identical calls, so `Optimize` is non-deterministic again. Nothing "+
			"downstream normalises that before repairClubs reads the slice or before "+
			"the seed-ranking loop scores it with a bare `>`.\n\n"+
			"Look first at seedFor's bench fill and at repairClubs' choice of "+
			"over-limit club: both iterated a map before this was fixed. The reason "+
			"this matters beyond one squad is that every byte-identical invariance in "+
			"the research record is only evidence if the search is deterministic.",
			len(orderSeen), runs)
	}
}
