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
// fifteen and differs only in how it ranks them.
//
// # Two horizons, because these are two different decisions
//
// ⚠️ **The first version of this test built the squad at Horizon 1, and that was
// wrong.** Nobody picks a squad for one gameweek. A squad is bought to hold, the
// shipped `fixture_horizon` is 5, and a one-week optimum chases that week's
// fixtures into a shape no real squad ever has. The ordering was therefore being
// measured inside the wrong arena.
//
// ⚠️ Note what kind of error that was: **bias, not noise.** Averaging over 222
// season-weeks disposes of single-week variance, and it did — the headline gap
// cleared its own threshold either way. What it could not dispose of is measuring
// the right quantity on the wrong population. More data would not have helped.
//
// So the two decisions get the two horizons they actually have:
//
//   - **Squad — Horizon 5**, the shipped `fixture_horizon`, rebuilt from scratch
//     only at the start of each block of `block` gameweeks and then HELD. Rebuilt
//     from scratch rather than repaired, deliberately: repairing would drag
//     transfer policy into a measurement about ordering.
//   - **Ordering — Horizon 1.** Fielding an XI and choosing a captain really is a
//     one-week decision, so this one stays where it was.
//
// The table runs at `block` = 1, 5 and 10 rather than committing to a stretch,
// which turns "does the squad change over 5-10 games?" from an assumption into a
// measured row.
//
// ⚠️ block=1 is NOT the discarded first version. It rebuilds weekly but still at
// horizon 5, so it is "re-optimised constantly, for the right horizon" — the
// contrast that isolates HOLDING a squad from the horizon it was built at. The
// original horizon-1 squad is not an arm here at all, because it answers no
// question anyone has.
//
// The arms, all handed the same held fifteen:
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
// were chosen by the same engine MODEL then ranks them with, so within them the
// model's score is range-restricted and the baseline's is not. This is the right
// framing for the question asked — what ships is the model's squad, and the
// weekly XI call is made inside it — but it means MODEL−BASE is *this decision's*
// edge and not a general statement that the model out-ranks persistence. The
// population-neutral version of that comparison is PRICE40 in
// candidateset_diag_test.go, which the model did not choose.
//
// ⚠️ **Nor is it out-of-sample.** The engine's constants were swept on grids that
// include these very seasons, while the last-5 baseline is fitted to nothing at
// all. MODEL−BASE is therefore optimistic by an amount not measured here. A clean
// holdout is not cheaply available: the seasons outside the sweep grid are the
// pre-2022-23 ones, whose expected-goals columns are backfilled rather than
// native, so they change the data regime at the same time.
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
// budget, with no squad carried in and no transfer limit, is more uniform than
// any real squad, and ordering matters less inside a uniform set. Longer blocks
// shrink that bias without removing it. The two biases are not known to cancel
// and no claim here depends on their net.
func TestDiagReplayPointsFromOrdering(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)
	pairs := loadPairsOrSkip(t, cfg)

	fmt.Printf("\n=== WHAT IS A BETTER ORDERING ACTUALLY WORTH, IN POINTS?\n")
	fmt.Printf("Every arm is handed the SAME fifteen and differs only in how it ranks them\n")
	fmt.Printf("into an XI and a captain. Squad built at the shipped horizon 5 and HELD for\n")
	fmt.Printf("`block` gameweeks; ordering scored at horizon 1, which is the horizon the XI\n")
	fmt.Printf("decision actually has. Points per gameweek, captain doubled, no auto-subs.\n")

	blocks := []int{1, 5, 10}
	// Keyed by season, not appended positionally: a season dropped at one block
	// and kept at another would silently misalign two slices, and the paired
	// comparison below would then difference unrelated seasons.
	edgeBy := map[int]map[string]float64{}
	ceilBy := map[int]map[string]float64{}

	for _, block := range blocks {
		edgeBy[block] = map[string]float64{}
		ceilBy[block] = map[string]float64{}
		fmt.Printf("\n--- squad rebuilt every %d gameweek(s)\n", block)
		fmt.Printf("  %-11s %5s %7s %7s %8s %8s %7s %9s %9s\n",
			"season", "gws", "BASE", "MODEL", "MODEL+OC", "ORCXI+MC", "ORACLE",
			"mdl-base", "orcl-mdl")

		var gapVsBase, gapVsOracle, gapCaptain, gapXI []float64

		for _, pr := range pairs {
			// Two configs, because these are two decisions. The squad is a
			// multi-week commitment and is built at the shipped horizon; the XI
			// and captain are this week's call and are scored at one.
			squadCfg := SimConfig{Weights: cfg.Weights, StartGW: 1}
			squadCfg.Weights.Horizon = 5
			orderCfg := SimConfig{Weights: cfg.Weights, StartGW: 1}
			orderCfg.Weights.Horizon = 1

			var base, model, modelOC, oracleXIMC, oracle float64
			weeks := 0
			var held []int

			for gw := 1; gw <= 38; gw++ {
				if held == nil || (gw-1)%block == 0 {
					// From scratch, not repaired: repairing from the carried
					// squad would import transfer policy into a measurement
					// about ordering.
					es, _ := EngineAt(pr.Cur, pr.Prior, gw-1, squadCfg)
					if sq, ok := repairSquad(es, nil, 1000, 0, squadCfg); ok && len(sq) == 15 {
						held = sq
					}
				}
				if len(held) != 15 {
					continue
				}
				ew, _ := EngineAt(pr.Cur, pr.Prior, gw-1, orderCfg)

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
					// gameweek where any of the fifteen has none is dropped
					// rather than handed the baseline a zero it did not earn —
					// that zero would flatter MODEL against BASE for a reason
					// unrelated to ordering.
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

				byBase := func(p replayPick) float64 { return p.base }
				byPred := func(p replayPick) float64 { return p.pred }
				byAct := func(p replayPick) float64 { return p.act }

				baseXI, okB := bestXIBy(picks, byBase)
				modelXI, okM := bestXIBy(picks, byPred)
				oracleXI, okO := bestXIBy(picks, byAct)
				if !okB || !okM || !okO {
					continue
				}

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
			edgeBy[block][pr.Name] = m - b
			ceilBy[block][pr.Name] = o - m
			gapCaptain = append(gapCaptain, moc-m)
			gapXI = append(gapXI, oxi-m)
		}

		if len(gapVsOracle) < 2 {
			fmt.Printf("  (too few seasons to summarise)\n")
			continue
		}
		fmt.Printf("\n  %-24s %8s %8s %10s\n", "gap (pts/gw)", "mean", "se", "threshold")
		summarise("MODEL - BASE", gapVsBase)
		summarise("ORACLE - MODEL", gapVsOracle)
		summarise("  captaincy alone", gapCaptain)
		summarise("  XI selection alone", gapXI)
	}

	fmt.Printf("\n--- does the block length matter? (PAIRED on season)\n")
	fmt.Printf("The same six seasons appear at every block, so an unpaired comparison of\n")
	fmt.Printf("two block means throws away the pairing and understates the precision of\n")
	fmt.Printf("their difference. These rows difference each season against ITSELF.\n\n")
	fmt.Printf("  %-24s %8s %8s %10s\n", "paired difference", "mean", "se", "threshold")
	for i := 1; i < len(blocks); i++ {
		lo, hi := blocks[i-1], blocks[i]
		var de, dc []float64
		for name, v := range edgeBy[hi] {
			if u, ok := edgeBy[lo][name]; ok {
				de = append(de, v-u)
			}
		}
		for name, v := range ceilBy[hi] {
			if u, ok := ceilBy[lo][name]; ok {
				dc = append(dc, v-u)
			}
		}
		if len(de) > 1 {
			summarise(fmt.Sprintf("edge:    block %d - %d", hi, lo), de)
		}
		if len(dc) > 1 {
			summarise(fmt.Sprintf("ceiling: block %d - %d", hi, lo), dc)
		}
	}
	fmt.Printf("\n⚠️ A paired difference below its own threshold means THIS measurement\n")
	fmt.Printf("cannot distinguish the two block lengths — not that they are the same.\n")

	fmt.Printf("\n⚠️ MODEL - BASE is the solid number: the model's ordering edge over naive\n")
	fmt.Printf("persistence, in points, on the players the optimiser actually fields. Read it\n")
	fmt.Printf("as a SCALE, not as a bound — it says what beating persistence is worth and\n")
	fmt.Printf("nothing about how much is left. It is also NOT out-of-sample: the engine's\n")
	fmt.Printf("constants were swept on these seasons and the baseline was fitted to nothing.\n")
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
	fmt.Printf("⚠️ block=1 still builds at horizon 5 — it re-optimises every week rather\n")
	fmt.Printf("than holding. It isolates the cost of HOLDING a squad from the horizon it\n")
	fmt.Printf("was built at, and is not the discarded horizon-1 version, which is gone.\n")
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
