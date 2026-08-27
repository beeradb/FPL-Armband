package backtest

import (
	"fmt"
	"math"
	"os"
	"sort"
	"testing"
)

// TestDiagReplayPointsFromOrdering prices an ordering defect in the only unit a
// decision spends: realised points.
//
// # Why a rank statistic could not answer this
//
// The model's Spearman inside its own top forty is 0.140, and two nulls
// disagree about whether that is a defect or the ceiling (see
// restrictionnull_diag_test.go — suggestive, NOT established). That argument
// cannot be settled by a better null, because "no tail weakness" is undefinable
// without assuming a dependence family.
//
// But it does not have to be settled to be PRICED. The optimiser is a knapsack.
// A mis-ordering inside the candidate set costs nothing at all unless it changes
// which players are actually fielded — and rank correlation is blind to that,
// because it weights a swap between the 38th and 39th best exactly as heavily as
// a swap between the 1st and the 2nd.
//
// # The design: hold the squad fixed, vary only the ordering
//
// Squad construction and weekly ordering are separate decisions, and mixing them
// would leave any gap unattributable. So every arm here is handed the SAME
// fifteen — the model's own optimum for that gameweek — and differs only in how
// it ranks those fifteen:
//
//   - BASE     — XI and captain by last-5-appearance mean, the same naive
//     persistence baseline the published record uses elsewhere.
//   - MODEL    — XI and captain by the engine's score. What ships.
//   - MODEL+OC — the model's XI, but the captain chosen with hindsight.
//   - ORCXI+MC — the hindsight XI, but the captain still chosen by the model.
//   - ORACLE   — XI and captain both by realised points. The CEILING: every point
//     available from perfect ordering within the squad.
//
// The middle two are a 2x2 against MODEL and ORACLE, and they exist because a
// single lumped ceiling would be worthless here. Captaincy and XI selection are
// wildly unequal in leverage — the captain is the only pick worth two, and
// hindsight on it alone is enormous — so ORACLE−MODEL is dominated by a term
// that has nothing to do with the disputed rank statistic. Splitting them is
// what makes the bound readable.
//
// ⚠️ **ORACLE−MODEL is the only real upper bound here.** It bounds not just the
// disputed tail weakness but EVERY conceivable ordering improvement, including
// ones nobody has thought of. It is also LOOSE, for the reason above and for the
// auto-substitution reason below.
//
// MODEL−BASE is the solid, decision-relevant number: the model's ordering edge
// over naive persistence, in points, on the very players the optimiser fields.
// ⚠️ **Read it as a scale, NOT as a bound.** It says what beating persistence is
// worth; it says nothing about how much is left. An earlier draft of this comment
// asserted the disputed tail effect was "a fraction of that edge" — that was a
// hand-wave with no derivation and it is withdrawn. A rank-correlation shortfall
// against a null is not commensurable with a points gap over a baseline.
//
// ⚠️ **The hindsight arms are clairvoyance, not headroom, and MODEL+OC is the one
// that will be misread.** Its captain is picked by REALISED points, so most of
// what it earns is irreducible single-week variance nobody could have held —
// finishing, penalties won, a deflection. It is NOT "what a better captain model
// would be worth", and it may not be quoted as the value of any reachable
// captaincy change. The reachable share is unknown and much smaller. No arm here
// is a target, none may be tuned toward, and none leaves this diagnostic.
//
// ⚠️ **The arena is the model's own squad, and that is not neutral.** The fifteen
// were chosen by the same score MODEL then ranks them with, so within them the
// model's score is range-restricted and the baseline's is not. This is the right
// framing for the question asked — what ships is the model's squad, and the
// weekly XI call is made inside it — but it means MODEL−BASE is *this decision's*
// edge and not a general statement that the model out-ranks persistence. The
// population-neutral version of that comparison is PRICE40 in
// candidateset_diag_test.go, which the model did not choose.
//
// # What is deliberately NOT modelled, and which way it bends the answer
//
// No auto-substitutions and no vice-captain fallback: a player who did not
// feature scores zero, and a captain who did not feature doubles zero.
//
// ⚠️ **An earlier version of this comment claimed that omitting auto-subs "moves
// every arm in the same direction and changes no gap". That is FALSE and the
// correction matters for how the table is read.** An auto-sub only rescues an arm
// that STARTED a player who blanked. ORACLE never starts one, so it gains
// nothing, while BASE and MODEL both gain — auto-subs are partial insurance
// against exactly the mistake the hindsight arms never make. Modelling them would
// therefore COMPRESS every gap against ORACLE, by an amount not measured here. So
// the ceiling columns are biased UPWARD. The same argument applies to the vice
// captain, which is the identical insurance on the doubled pick.
//
// The one bias pointing the other way is the arena: a fifteen rebuilt at full
// budget every week is more uniform than any real squad, and ordering matters
// less inside a uniform set. The two biases are not known to cancel and no claim
// here depends on their net.
func TestDiagReplayPointsFromOrdering(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)

	fmt.Printf("\n=== WHAT IS A BETTER ORDERING ACTUALLY WORTH, IN POINTS?\n")
	fmt.Printf("Every arm is handed the SAME fifteen — the model's own optimum that week —\n")
	fmt.Printf("and differs only in how it ranks them into an XI and a captain.\n")
	fmt.Printf("Points per gameweek, captain doubled, no auto-subs.\n\n")
	fmt.Printf("  %-11s %5s %7s %7s %8s %8s %7s %9s %9s\n",
		"season", "gws", "BASE", "MODEL", "MODEL+OC", "ORCXI+MC", "ORACLE",
		"mdl-base", "orcl-mdl")

	var gapVsBase, gapVsOracle, gapCaptain, gapXI []float64

	for _, pr := range loadPairsOrSkip(t, cfg) {
		sc := SimConfig{Weights: cfg.Weights, StartGW: 1}
		sc.Weights.Horizon = 1

		var base, model, modelOC, oracleXIMC, oracle float64
		weeks := 0

		for gw := 1; gw <= 38; gw++ {
			ew, _ := EngineAt(pr.Cur, pr.Prior, gw-1, sc)
			held, ok := repairSquad(ew, nil, 1000, 0, sc)
			if !ok || len(held) != 15 {
				continue
			}

			picks := make([]replayPick, 0, 15)
			complete := true
			for _, id := range held {
				el := ew.Boot.ElementByID(id)
				p, seen := pr.Cur.Players[id]
				if el == nil || !seen {
					complete = false
					break
				}
				m := ew.Metrics(el)
				// The baseline needs a history to be persistence at all. A
				// gameweek where any of the fifteen has none is dropped rather
				// than handed the baseline a zero it did not earn — that zero
				// would flatter MODEL against BASE for a reason unrelated to
				// ordering.
				var last []float64
				for b := gw - 1; b >= 1 && len(last) < 5; b-- {
					if q, had := p.GWs[b]; had && q.Fixtures > 0 {
						last = append(last, float64(q.Points))
					}
				}
				if len(last) == 0 {
					complete = false
					break
				}
				act := 0.0
				if g, had := p.GWs[gw]; had && g.Fixtures > 0 {
					act = float64(g.Points)
				}
				picks = append(picks, replayPick{
					pos: m.Position, pred: m.Score, base: meanOf(last), act: act,
				})
			}
			if !complete || len(picks) != 15 {
				continue
			}

			baseXI, okB := bestXIBy(picks, func(p replayPick) float64 { return p.base })
			modelXI, okM := bestXIBy(picks, func(p replayPick) float64 { return p.pred })
			oracleXI, okO := bestXIBy(picks, func(p replayPick) float64 { return p.act })
			if !okB || !okM || !okO {
				continue
			}

			byBase := func(p replayPick) float64 { return p.base }
			byPred := func(p replayPick) float64 { return p.pred }
			byAct := func(p replayPick) float64 { return p.act }
			base += scoreXI(baseXI, byBase)
			model += scoreXI(modelXI, byPred)
			modelOC += scoreXI(modelXI, byAct)
			oracleXIMC += scoreXI(oracleXI, byPred)
			oracle += scoreXI(oracleXI, byAct)
			weeks++
		}

		if weeks < 10 {
			continue
		}
		n := float64(weeks)
		b, m, moc, oxi, o := base/n, model/n, modelOC/n, oracleXIMC/n, oracle/n
		fmt.Printf("  %-11s %5d %7.2f %7.2f %8.2f %8.2f %7.2f %9.2f %9.2f\n",
			pr.Name, weeks, b, m, moc, oxi, o, m-b, o-m)
		gapVsBase = append(gapVsBase, m-b)
		gapVsOracle = append(gapVsOracle, o-m)
		gapCaptain = append(gapCaptain, moc-m)
		gapXI = append(gapXI, oxi-m)
	}

	if len(gapVsOracle) < 2 {
		t.Skip("too few seasons to summarise")
	}
	fmt.Printf("\n  %-24s %8s %8s %10s\n", "gap (pts/gw)", "mean", "se", "threshold")
	summarise("MODEL - BASE", gapVsBase)
	summarise("ORACLE - MODEL", gapVsOracle)
	summarise("  captaincy alone", gapCaptain)
	summarise("  XI selection alone", gapXI)

	fmt.Printf("\n⚠️ MODEL - BASE is the solid number: the model's ordering edge over naive\n")
	fmt.Printf("persistence, in points, on the players the optimiser actually fields. Read it\n")
	fmt.Printf("as a SCALE, not as a bound — it says what beating persistence is worth and\n")
	fmt.Printf("nothing about how much is left.\n")
	fmt.Printf("⚠️ ORACLE - MODEL is the only real upper bound, covering the disputed\n")
	fmt.Printf("top-forty tail weakness and every other ordering idea at once. It is LOOSE\n")
	fmt.Printf("twice over: hindsight dwarfs any reachable ordering, and omitting auto-subs\n")
	fmt.Printf("and the vice captain inflates it further, since those insure only the arms\n")
	fmt.Printf("that start a blanker and ORACLE never does.\n")
	fmt.Printf("⚠️ The hindsight arms are CLAIRVOYANCE, not headroom. `captaincy alone` picks\n")
	fmt.Printf("the captain by realised points, so most of it is single-week variance nobody\n")
	fmt.Printf("could hold. It is NOT the value of a better captain model and may not be\n")
	fmt.Printf("quoted as one.\n")
	fmt.Printf("⚠️ The two components do NOT add to the total: XI and captaincy interact,\n")
	fmt.Printf("since perfecting the XI changes which players the captaincy is chosen from.\n")
	fmt.Printf("⚠️ `threshold` is t_crit x se at df n-1, so a gap smaller than its own\n")
	fmt.Printf("threshold is not a measurement — read it as a ceiling, not as an effect.\n")
}

// replayPick is one of the fifteen, carrying the three orderings under test and
// the realised points every arm is finally scored on.
type replayPick struct {
	pos             string
	pred, base, act float64
}

// scoreXI totals an XI's REALISED points and doubles its captain, where the
// captain is the XI's best player under that arm's own ordering — not under
// hindsight. Passing the ordering in rather than reading a field is what keeps
// MODEL+OC honest: it differs from MODEL in this argument alone.
func scoreXI(xi []replayPick, by func(replayPick) float64) float64 {
	total, best, capt := 0.0, math.Inf(-1), 0.0
	for _, p := range xi {
		total += p.act
		if v := by(p); v > best {
			best, capt = v, p.act
		}
	}
	return total + capt
}

// bestXIBy picks the highest-ranked legal XI out of a fifteen under an arbitrary
// ordering. FPL's formation rules: exactly one keeper, three to five defenders,
// two to five midfielders, one to three forwards.
//
// The search is exhaustive over the nine legal formations rather than greedy,
// because a greedy pick by rank alone can strand a formation — it will happily
// take six midfielders and then have no legal shape to put them in.
func bestXIBy(squad []replayPick, by func(replayPick) float64) ([]replayPick, bool) {
	pool := map[string][]replayPick{}
	for _, p := range squad {
		pool[p.pos] = append(pool[p.pos], p)
	}
	for k := range pool {
		v := pool[k]
		sort.SliceStable(v, func(a, b int) bool { return by(v[a]) > by(v[b]) })
	}
	if len(pool["GKP"]) < 1 || len(pool["DEF"]) < 3 ||
		len(pool["MID"]) < 2 || len(pool["FWD"]) < 1 {
		return nil, false
	}

	var best []replayPick
	bestVal := math.Inf(-1)
	for d := 3; d <= 5; d++ {
		for m := 2; m <= 5; m++ {
			f := 10 - d - m
			if f < 1 || f > 3 {
				continue
			}
			if len(pool["DEF"]) < d || len(pool["MID"]) < m || len(pool["FWD"]) < f {
				continue
			}
			xi := []replayPick{pool["GKP"][0]}
			xi = append(xi, pool["DEF"][:d]...)
			xi = append(xi, pool["MID"][:m]...)
			xi = append(xi, pool["FWD"][:f]...)
			v := 0.0
			for _, p := range xi {
				v += by(p)
			}
			if v > bestVal {
				bestVal, best = v, xi
			}
		}
	}
	return best, best != nil
}

// summarise prints a per-season gap as a mean against its own detection
// threshold, so a number too small to distinguish from zero cannot be read as
// an effect.
func summarise(name string, xs []float64) {
	n := float64(len(xs))
	mean := meanOf(xs)
	sd := 0.0
	for _, x := range xs {
		sd += (x - mean) * (x - mean)
	}
	sd = math.Sqrt(sd / (n - 1))
	se := sd / math.Sqrt(n)
	fmt.Printf("  %-24s %8.3f %8.3f %10.3f\n", name, mean, se, tCrit95(len(xs)-1)*se)
}
