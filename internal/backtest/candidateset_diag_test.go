package backtest

import (
	"fmt"
	"os"
	"sort"
	"testing"
)

// IS THE MODEL SCORED ON THE PLAYERS THAT MATTER?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagCandidateSetAccuracy -v -count=1 -timeout 120m
//
// # The argument
//
// **Only 30-40 players are ever realistic picks, so predictions for the rest are
// nearly meaningless.** The model's published accuracy is computed over every
// priced player — roughly 600 — so a mean absolute error of 1.98 is dominated by
// people no squad will ever hold. The decision-relevant number is the error on
// the set a transfer search actually chooses between, and it is measured nowhere.
//
// The owner's estimate is close to what this harness already reports from the
// other side: a season's point-in-time optima use **39-65 distinct starters**,
// with **8-10 supplying half** the starting slots. So the population is small and
// measurable rather than asserted.
//
// # Why the answer is likely to be UNFLATTERING
//
// The candidate set is the high-projection tail, and that is exactly where this
// model is weakest: the predicted-6-and-above band reads a ratio of 0.963, error
// spread rises from 1.0 on players who score nothing to 3.0 on those who haul,
// and the aggregate bias of -0.08 is near-perfect only because those errors
// cancel. **A global metric is flattered by the players who do not matter.**
//
// If error on the candidate set is materially worse than global, then every
// constant fitted to minimise global error was fitted against the wrong loss —
// which is the constructive form of this record's standing warning that "a better
// predictor is not automatically a better policy, because the optimiser consumes
// an ordering and lives in the tail".
//
// # Three definitions, because the definition IS the argument
//
// A single rule would make the result a property of that rule. If three
// independent constructions select roughly the same players, the set is real:
//
//   - **OPTIMUM**: every player appearing in any gameweek's point-in-time optimal
//     eleven. What the engine itself would ever field.
//   - **TOP40**: the forty highest-projected players in each gameweek, unioned.
//     What a transfer search looks at, without reference to squad feasibility.
//   - **HELD**: players a real squad could hold for a run — projected above a
//     floor in at least a quarter of gameweeks. Neither of the above requires
//     persistence, and a template is about persistence.
//
// ⚠️ **Raw MAE is NOT comparable across populations.** Candidates score more, so
// their errors are larger in absolute terms whatever the model does. The
// scale-free columns are the comparison: error relative to the target's own
// spread, and the within-gameweek rank correlation, which is what an argmax
// consumes.
func TestDiagCandidateSetAccuracy(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)

	type obs struct{ pred, act float64 }
	type acc struct {
		o    []obs
		byGW map[int][]obs
		ids  map[int]bool
	}
	newAcc := func() *acc { return &acc{byGW: map[int][]obs{}, ids: map[int]bool{}} }

	report := func(name string, a *acc) {
		if len(a.o) < 50 {
			return
		}
		var pred, act []float64
		for _, x := range a.o {
			pred = append(pred, x.pred)
			act = append(act, x.act)
		}
		mp, ma := meanOf(pred), meanOf(act)
		var mae, bias, sse float64
		for _, x := range a.o {
			mae += absOf(x.pred - x.act)
			bias += x.pred - x.act
		}
		mae /= float64(len(a.o))
		bias /= float64(len(a.o))
		for _, x := range a.o {
			d := (x.pred - x.act) - bias
			sse += d * d
		}
		esd := 0.0
		if len(a.o) > 1 {
			esd = sqrtOf(sse / float64(len(a.o)-1))
		}
		// Scale-free: the target's own spread, and the ordering an argmax reads.
		spread := sdOf(act)
		var rhos []float64
		for _, v := range a.byGW {
			if len(v) < 8 {
				continue
			}
			var p, q []float64
			for _, x := range v {
				p = append(p, x.pred)
				q = append(q, x.act)
			}
			rhos = append(rhos, corrOf(rankOf(p), rankOf(q)))
		}
		rho := 0.0
		if len(rhos) > 0 {
			rho = meanOf(rhos)
		}
		fmt.Printf("%-10s %6d %7d %8.2f %8.2f %8.3f %8.3f %8.3f %9.3f %8.3f\n",
			name, len(a.ids), len(a.o), mp, ma, mae, bias, esd, mae/spread, rho)
	}

	fmt.Printf("\n=== IS THE MODEL SCORED ON THE PLAYERS THAT MATTER?\n")
	fmt.Printf("One-gameweek-ahead prediction against realised points, over every\n")
	fmt.Printf("priced player and then restricted to three candidate sets.\n")
	fmt.Printf("⚠️ Compare `mae/sd` and `rank rho`, NOT raw mae — candidates score more,\n")
	fmt.Printf("so their absolute errors are larger whatever the model does.\n\n")
	fmt.Printf("%-10s %6s %7s %8s %8s %8s %8s %8s %9s %8s\n",
		"set", "players", "rows", "predMean", "actMean", "mae", "bias", "errSD", "mae/sd", "rank rho")

	for _, pr := range loadPairsOrSkip(t, cfg) {
		sc := SimConfig{Weights: cfg.Weights, StartGW: 1}
		sc.Weights.Horizon = 1

		// Pass 1: the three candidate sets.
		inOpt, inTop, above := map[int]bool{}, map[int]bool{}, map[int]int{}
		weeks := 0
		for gw := 1; gw <= 38; gw++ {
			ew, _ := EngineAt(pr.Cur, pr.Prior, gw-1, sc)
			if sq, ok := repairSquad(ew, nil, 1000, 0, sc); ok {
				xi, _, _, _ := pickXI(ew, sq)
				for _, id := range xi {
					inOpt[id] = true
				}
			}
			type ps struct {
				id int
				s  float64
			}
			var all []ps
			for id := range pr.Cur.Players {
				if el := ew.Boot.ElementByID(id); el != nil {
					s := ew.Metrics(el).Score
					all = append(all, ps{id, s})
					if s >= 3.0 {
						above[id]++
					}
				}
			}
			sort.Slice(all, func(a, b int) bool { return all[a].s > all[b].s })
			for i := 0; i < 40 && i < len(all); i++ {
				inTop[all[i].id] = true
			}
			weeks++
		}
		held := map[int]bool{}
		for id, n := range above {
			if float64(n) >= 0.25*float64(weeks) {
				held[id] = true
			}
		}

		// Pass 2: predictions against realised points.
		global, optA, topA, heldA := newAcc(), newAcc(), newAcc(), newAcc()
		for gw := 1; gw <= 38; gw++ {
			ew, _ := EngineAt(pr.Cur, pr.Prior, gw-1, sc)
			for id, p := range pr.Cur.Players {
				g, ok := p.GWs[gw]
				if !ok || g.Fixtures == 0 {
					continue
				}
				el := ew.Boot.ElementByID(id)
				if el == nil {
					continue
				}
				x := obs{ew.Metrics(el).Score, float64(g.Points)}
				for _, tgt := range []*acc{global} {
					tgt.o = append(tgt.o, x)
					tgt.byGW[gw] = append(tgt.byGW[gw], x)
					tgt.ids[id] = true
				}
				for _, pair := range []struct {
					in bool
					a  *acc
				}{{inOpt[id], optA}, {inTop[id], topA}, {held[id], heldA}} {
					if pair.in {
						pair.a.o = append(pair.a.o, x)
						pair.a.byGW[gw] = append(pair.a.byGW[gw], x)
						pair.a.ids[id] = true
					}
				}
			}
		}
		fmt.Printf("%s\n", pr.Name)
		report("  all", global)
		report("  OPTIMUM", optA)
		report("  TOP40", topA)
		report("  HELD", heldA)
		var overlap int
		for id := range inOpt {
			if inTop[id] {
				overlap++
			}
		}
		fmt.Printf("  sets: optimum %d, top40 %d, held %d | optimum∩top40 %d\n\n",
			len(inOpt), len(inTop), len(held), overlap)
	}
	fmt.Printf("⚠️ If the three sets disagree wildly the population is an artefact of\n")
	fmt.Printf("whichever rule made it, and no conclusion about 'the players that\n")
	fmt.Printf("matter' survives. Read the overlap before the accuracy.\n")
}

func absOf(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func sqrtOf(x float64) float64 {
	if x <= 0 {
		return 0
	}
	g := x
	for i := 0; i < 40; i++ {
		g = 0.5 * (g + x/g)
	}
	return g
}

func sdOf(v []float64) float64 {
	if len(v) < 2 {
		return 1
	}
	m := meanOf(v)
	var s float64
	for _, x := range v {
		s += (x - m) * (x - m)
	}
	return sqrtOf(s / float64(len(v)-1))
}
