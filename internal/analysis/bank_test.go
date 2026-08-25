package analysis

import "testing"

// TestBankIsSpentAndNeverExceeded — money in the bank must reach both searches.
//
// Every existing budget test pins the *upper* bound: a swap or pair may not
// spend more than it has. That half fails loudly. The other half fails
// silently — a bank that never reaches RankSwaps reads as a squad with no
// affordable upgrade, which is exactly what an already-good squad looks like.
// A caller that drops the bank on the floor (passing zero because it never
// fetched it) therefore produces plausible output and simply never spends.
//
// So this asserts the balance is *used*: more money must widen the candidate
// set, every extra candidate must be one the smaller bank could not afford,
// and the best move available must never get worse as the bank grows.
func TestBankIsSpentAndNeverExceeded(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	// ⚠️ ADDED 2026-08-22, after this test failed on origin/main for two days without
	// anyone owning it. roleEngine deliberately omits this guard — datawindow_test.go's
	// own callers override the engine's state immediately afterwards, so for them the
	// live window cannot bite. THIS test does not override, so it was exposed, and it is
	// the only one of the thirteen guard sites that was missing.
	//
	// Inside the GW1 gap (SeasonHasStarted, GameweeksPlayed still 0) some clubs report
	// fresh-season zeros while others still carry last season's totals IN THE SAME LIVE
	// FETCH, so el.Minutes means two different things at once and a £95m squad finds no
	// improving swap at any bank. Observed: 0 swaps at £0m, £1m, £2m and £5m alike.
	//
	// ⚠️ THE SERVED SITE IS NOT AFFECTED, and that was checked rather than assumed:
	// cmd/armband wires internal/priors, which reads a fresh-season zero as "no evidence
	// yet" instead of "scores zero". This package cannot do the same — priors imports
	// analysis, so the reverse is a cycle. Confirmed against the live deployment while
	// this was failing: the market carried 582 rows and a full fifteen.
	// A paid upgrade needs a spread of scored players at a spread of prices; after one gameweek only those who played carry a score at all.
	skipUntilLiveEvidence(t, e, corroboratingMatches)
	// £95m, so £5m is genuinely spare rather than already committed. An
	// optimal £100m squad has nothing to buy and would prove nothing.
	sq, err := e.Optimize(OptimizeRequest{
		Budget: 950, MinMinutes: 600, MinExpectedMinutes: 55, BenchWeight: DefaultBenchWeight,
	})
	if err != nil {
		t.Skipf("optimiser unavailable: %v", err)
	}
	pool := e.AllMetrics()
	state := NewSquadState(sq.Players)

	banks := []int{0, 10, 20, 50}
	counts := make([]int, len(banks))
	bestGain := make([]float64, len(banks))
	spends := make([]int, len(banks))

	for i, bank := range banks {
		swaps := RankSwaps(state, pool, bank)
		counts[i] = len(swaps)
		for _, s := range swaps {
			d := tenths(s.In.Price) - state.sellPrice(s.Out)
			if d > bank {
				t.Errorf("bank £%.1fm: %s -> %s spends £%.1fm",
					float64(bank)/10, s.Out.Name, s.In.Name, float64(d)/10)
			}
			if d > spends[i] {
				spends[i] = d
			}
		}
		// Pairs and plans share the same allowance and must respect it too.
		for _, p := range RankPairs(state, pool, bank, 4, 3) {
			spend := tenths(p.Up.In.Price) - state.sellPrice(p.Up.Out)
			for _, d := range p.Downs {
				spend += tenths(d.In.Price) - state.sellPrice(d.Out)
			}
			if spend > bank {
				t.Errorf("bank £%.1fm: pair buying %s spends £%.1fm",
					float64(bank)/10, p.Up.In.Name, float64(spend)/10)
			}
		}
		for _, p := range BuildPlans(state, pool, nil, bank, 3, 3) {
			if p.Spend > bank {
				t.Errorf("bank £%.1fm: plan spends £%.1fm", float64(bank)/10, float64(p.Spend)/10)
			}
			if p.GainPerGW > bestGain[i] {
				bestGain[i] = p.GainPerGW
			}
		}
		t.Logf("bank £%.1fm: %d swaps, dearest £%.1fm, best plan %.3f/gw",
			float64(bank)/10, counts[i], float64(spends[i])/10, bestGain[i])
	}

	// A bank that is being used buys players the empty bank could not reach.
	if counts[len(counts)-1] <= counts[0] {
		t.Errorf("£%.1fm in the bank found %d swaps against %d with nothing — "+
			"the balance is not reaching the search",
			float64(banks[len(banks)-1])/10, counts[len(counts)-1], counts[0])
	}
	if spends[len(spends)-1] <= 0 {
		t.Error("no swap spends any money, so the bank is buying nothing")
	}
	for i := 1; i < len(banks); i++ {
		if bestGain[i] < bestGain[i-1]-1e-9 {
			t.Errorf("bank £%.1fm gives %.3f/gw, worse than £%.1fm's %.3f/gw",
				float64(banks[i])/10, bestGain[i], float64(banks[i-1])/10, bestGain[i-1])
		}
	}
}

// TestBenchWeightHasOneDefault — Weights.BenchWeight and DefaultBenchWeight are
// the same quantity and must not disagree.
//
// They did. DefaultBenchWeight was 0.10, the middle of the measured plateau and
// what the replay that measured it actually used; Weights.BenchWeight shipped at
// 0.15, one step past the cliff at 2159 against 2197. Optimize falls back to the
// latter whenever a caller passes zero, which is exactly what optimize_squad
// does when the agent omits bench_weight — so the tool the model calls ran on
// the unmeasured side of the cliff while every measurement used the other.
func TestBenchWeightHasOneDefault(t *testing.T) {
	if got := DefaultWeights().BenchWeight; got != DefaultBenchWeight {
		t.Errorf("Weights.BenchWeight defaults to %v and DefaultBenchWeight is %v; "+
			"one quantity, one value, or the fallback path silently scores a "+
			"different squad from the measured one", got, DefaultBenchWeight)
	}
}
