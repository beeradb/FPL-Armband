package backtest

import (
	"fmt"
	"sort"
	"testing"

	"armband/internal/stats"
)

// DOES THE MODEL'S EDGE OVER NAIVE PERSISTENCE SURVIVE ON THE PLAYERS THAT MATTER?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagCandidateSetAccuracy -v -count=1 -timeout 120m
//
// # The question, and why the first version of this test could not answer it
//
// **Only a few dozen players are ever realistic picks, so predictions for the
// rest are nearly meaningless** — and published accuracy is computed over every
// priced player, 706 to 861 a season. If the model is weaker where decisions are
// made, then every constant fitted to global error was fitted against the wrong
// loss.
//
// ⚠️ **An earlier version reported a 27-45% fall in rank correlation on candidate
// sets and that was NOT evidence of anything.** Selecting the top 7-24% on the
// predictor attenuates rank correlation by range restriction alone: a truncation
// model puts the no-defect expectation at roughly 0.22-0.30, and the measurement
// read 0.28-0.39 — at or above it. A fall against the global figure is not a
// finding, because the global figure was never the right reference.
//
// # So this version compares against a BASELINE inside each set
//
// The decision-relevant question is not "how large is the error" — that rises
// with the population's own spread whatever the model does — but **"does the
// model still beat naive persistence here?"** The last-five-gameweek mean is
// OpenFPL's baseline and the one the published record already uses globally, so
// `skill = MAE_model / MAE_baseline` is anchored at 1.0 in every population:
// below 1 the model adds value, at 1 it adds none.
//
// ⚠️ **This is the column that can collapse the whole argument.** If the model's
// edge over persistence is unchanged inside the candidate sets, then it is not
// weaker where it matters in any sense a decision cares about, whatever the
// absolute error levels do.
//
// # Four sets, and one the model did not choose
//
//   - **MODEL-n** — the n highest-projected THIS gameweek, for n in
//     `candidateSetSizes`.
//   - **PRICE-n** — the n most expensive this gameweek. ⚠️ **Model-free**, and
//     the point of it: the other sets condition on the model's own output, so the
//     model is scored on the population it selected. Price is set by FPL, is
//     per-gameweek in the archive, and is a fair proxy for "realistic pick".
//
// # Why n is a LADDER and not one number
//
// n was 40 in both sets, hardcoded, so nothing here could say whether the answer
// depends on it. "Is the model worse where it matters" is not one question: the
// set a transfer search really chooses between is closer to ten than to fifty, and
// a skill ratio that is flat from 10 to 50 refutes the tail-defect worry far more
// cleanly than one that is measured at a single arbitrary width.
//
// ⚠️ **Read the ladder as a SHAPE, and do not take its best rung.** Picking the n
// with the most flattering skill is an argmax over four correlated estimates, which
// is the winner's curse this record names as its most load-bearing idea. Flat is a
// result; monotone is a result; the minimum is not.
//
// ⚠️ **PRICE-n is the ladder to tune against, never MODEL-n.** MODEL-n conditions
// on the model's own output, so a constant refitted against it can improve the loss
// by moving easy players into the set rather than by predicting better. Price is
// set by FPL and does not move when a constant moves.
//
// ⚠️ At n = 10 the per-gameweek rank correlation is computed on ten points, so the
// rho columns are much noisier at the short rungs than at the long ones. The `rows`
// column carries the evidence; read it before reading a rho gap.
//   - **OPTIMUM** — this gameweek's point-in-time optimal eleven.
//   - **HELD** — projected above 3.0, a squad-holdable floor.
//
// ⚠️ **Membership is PER GAMEWEEK, not a season union.** The union version scored
// a player in every week of the season once he entered the set in any of them, so
// most of its rows came from weeks he was NOT a candidate. Per-gameweek
// membership removes that dilution and is also forward-only: a live system knows
// its own top-40 at decision time, where it cannot know a season union.
//
// ⚠️ **Report per season, never pooled.** Expected-assist coverage is 40-42% in
// the Understat-backfilled seasons against 69-70% in the natively-published ones,
// so a pooled mean mixes two provider regimes. (Contrary to one review's claim,
// no season lacks xG — all seven carry it on 46-50% of rows.)
// candidateSetSizes is the ladder of candidate-set widths. 40 is kept so the
// figures already on the record reproduce; 10 and 20 bracket the set a transfer
// search realistically chooses between, and 50 is the long rung that says whether
// the answer is still moving.
//
// ⚠️ Membership stays "the first n of the sorted rows", so a gameweek with fewer
// than n usable rows yields a truncated set rather than being dropped — exactly
// what the single-width version did. The n = 40 rungs therefore run the same
// construction over the same rows as the single-width version did, which is a
// statement about the code and not a comparison of two runs: no banked
// per-gameweek PRICE40 exists to check the output against. The `players` and
// `rows` columns make any truncation visible.
var candidateSetSizes = []int{10, 20, 40, 50}

// candidateBands are DISJOINT rank windows, 1-indexed and inclusive, and they are
// the ladder a comparison across widths may actually be read from.
//
// ⚠️ **The cumulative rungs above are NESTED**, so PRICE10 ⊂ PRICE20 ⊂ PRICE40 ⊂
// PRICE50 share most of their rows and are not four independent estimates. Two
// consequences, and the second is the one that bites:
//
//   - "Flat across rungs" can only ever mean "the players added between rungs
//     perform like the ones already there", which is a weaker claim than four
//     distinct populations agreeing.
//   - **Flatness at the long end is partly mechanical.** Adding ten players to a
//     1,850-row set moves its aggregate error far less than adding ten to a
//     370-row set, whatever those players do. So the cumulative ladder is damped
//     toward flat exactly where it has the most rows, and its sharpest contrast
//     (10 against 20) is also its noisiest.
//
// Disjoint bands remove both. Each band is a different set of player-weeks, so
// comparing bands compares populations rather than a sequence with a shared
// prefix, and the row counts are comparable across bands rather than growing.
//
// ⚠️ They still are not independent DRAWS — a gameweek's outcome moves every
// band's baseline at once, because all of them are scored against the same
// five-week means over the same weeks. Season stays the clustering axis.
var candidateBands = [][2]int{{1, 10}, {11, 20}, {21, 40}, {41, 50}}

// priceRankOrder ranks player ids by price, most expensive first, and it is the
// one implementation of that ordering.
//
// # Why this is a function and not two sorts
//
// Price rank decides membership in two different places — this file's PRICE
// rungs and bands, and the rank-band populations the prediction benchmark emits
// to `FPL_PREDICTION_CSV`. Both need the same answer to "who are the ten most
// expensive players this gameweek", and one quantity with two implementations is
// this project's signature failure.
//
// ⚠️ **The tie-break is the load-bearing part, not tidiness.** Callers build
// their rows by ranging a MAP, so input order is randomised per run, and
// `sort.Slice` is NOT stable. Prices move in 0.1 steps so ties are everywhere,
// and the top-n by price genuinely changed between runs: two runs of the
// candidate-set diagnostic disagreed by up to 0.007 of skill on the same
// population, which is the size of the differences these tables are read for.
// Breaking on the lower element id makes the ordering total, and therefore the
// output reproducible.
//
// ⚠️ Predictions are floats and tie far less often, which is why the
// model-ranked sets were nearly stable while the price-ranked ones were not. The
// asymmetry is what made the defect visible; it is not a reason to leave a
// model-ranked sort untotalled.
func priceRankOrder(ids []int, prices []float64) []int {
	if len(ids) != len(prices) {
		panic(fmt.Sprintf("priceRankOrder: %d ids against %d prices — these are "+
			"parallel slices and a caller that lets them diverge is ranking "+
			"players by other players' prices", len(ids), len(prices)))
	}
	idx := make([]int, len(ids))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool {
		x, y := idx[a], idx[b]
		if prices[x] != prices[y] {
			return prices[x] > prices[y]
		}
		return ids[x] < ids[y]
	})
	out := make([]int, len(idx))
	for i, j := range idx {
		out[i] = ids[j]
	}
	return out
}

// rankWindow is the set of ids occupying ranks [lo, hi] of an order, 1-indexed
// and inclusive.
//
// A window running past the available rows contributes the rows it has rather
// than being dropped, which is the same convention the cumulative rungs use: a
// gameweek with fewer than n usable rows yields a truncated set. The `players`
// and `rows` columns downstream make any truncation visible.
func rankWindow(order []int, lo, hi int) map[int]bool {
	out := map[int]bool{}
	for i := lo - 1; i < hi && i < len(order); i++ {
		if i >= 0 {
			out[order[i]] = true
		}
	}
	return out
}

func TestDiagCandidateSetAccuracy(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)

	type obs struct{ pred, base, act float64 }
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
		var mm, mb float64
		for _, x := range a.o {
			mm += absOf(x.pred - x.act)
			mb += absOf(x.base - x.act)
		}
		n := float64(len(a.o))
		mm, mb = mm/n, mb/n
		var rm, rb []float64
		for _, v := range a.byGW {
			if len(v) < 8 {
				continue
			}
			p, bs, q := make([]float64, 0, len(v)), make([]float64, 0, len(v)), make([]float64, 0, len(v))
			for _, x := range v {
				p, bs, q = append(p, x.pred), append(bs, x.base), append(q, x.act)
			}
			rm = append(rm, stats.Spearman(p, q))
			rb = append(rb, stats.Spearman(bs, q))
		}
		mr, br := 0.0, 0.0
		if len(rm) > 0 {
			mr, br = meanOf(rm), meanOf(rb)
		}
		skill := 0.0
		if mb > 0 {
			skill = mm / mb
		}
		fmt.Printf("  %-9s %6d %7d %8.3f %8.3f %8.3f %9.3f %9.3f %8.3f\n",
			name, len(a.ids), len(a.o), mm, mb, skill, mr, br, mr-br)
	}

	fmt.Printf("\n=== DOES THE MODEL'S EDGE SURVIVE ON THE PLAYERS THAT MATTER?\n")
	fmt.Printf("Per-gameweek membership. `skill` is MAE_model / MAE_baseline, anchored at\n")
	fmt.Printf("1.0 in every population: below 1 the model beats naive persistence.\n")
	fmt.Printf("⚠️ Read `skill` and `rho gap`, NOT the raw error or the raw rho — both rise\n")
	fmt.Printf("or fall with the population's own spread whatever the model does.\n")
	fmt.Printf("⚠️ The PRICE ladder is the only family the model did not choose, and it\n")
	fmt.Printf("is the one to tune against: MODEL-n moves when a constant moves.\n")
	fmt.Printf("⚠️ MODEL-n and PRICE-n are CUMULATIVE and therefore NESTED, so their\n")
	fmt.Printf("rungs share most of their rows and are not independent estimates. They\n")
	fmt.Printf("are also damped toward flat at the long end, where ten added players move\n")
	fmt.Printf("an 1,850-row average very little. Read them as a description.\n")
	fmt.Printf("⚠️ Mband/Pband are DISJOINT rank windows and are the ladder a comparison\n")
	fmt.Printf("across widths may be read from: different bands are different players.\n")
	fmt.Printf("⚠️ Taking the best rung or band is an argmax over correlated estimates.\n")
	fmt.Printf("No standard error is computed anywhere on this table, so nothing here\n")
	fmt.Printf("carries a detection threshold and no rung difference is tested.\n\n")
	fmt.Printf("  %-9s %6s %7s %8s %8s %8s %9s %9s %8s\n",
		"set", "players", "rows", "mae", "mae_base", "skill", "rho", "rho_base", "rho gap")

	for _, pr := range loadPairsOrSkip(t, cfg) {
		sc := SimConfig{Weights: cfg.Weights, StartGW: 1}
		sc.Weights.Horizon = 1
		global, opt, held := newAcc(), newAcc(), newAcc()
		mdl, price := map[int]*acc{}, map[int]*acc{}
		for _, n := range candidateSetSizes {
			mdl[n], price[n] = newAcc(), newAcc()
		}
		mdlBand, priceBand := map[int]*acc{}, map[int]*acc{}
		for bi := range candidateBands {
			mdlBand[bi], priceBand[bi] = newAcc(), newAcc()
		}

		for gw := 1; gw <= 38; gw++ {
			ew, _ := EngineAt(pr.Cur, pr.Prior, gw-1, sc)
			// This week's optimum, for the OPTIMUM set.
			inOpt := map[int]bool{}
			if sq, ok := repairSquad(ew, nil, 1000, 0, sc); ok {
				if xi, _, _, _ := pickXI(ew, sq); xi != nil {
					for _, id := range xi {
						inOpt[id] = true
					}
				}
			}
			type row struct {
				id          int
				pred, price float64
				base, act   float64
				ok          bool
			}
			var rows []row
			for id, p := range pr.Cur.Players {
				g, has := p.GWs[gw]
				if !has || g.Fixtures == 0 {
					continue
				}
				el := ew.Boot.ElementByID(id)
				if el == nil {
					continue
				}
				// The baseline: the mean of the last five gameweeks the player
				// actually featured in, which is what the published record uses.
				var last []float64
				for b := gw - 1; b >= 1 && len(last) < 5; b-- {
					if q, ok := p.GWs[b]; ok && q.Fixtures > 0 {
						last = append(last, float64(q.Points))
					}
				}
				if len(last) == 0 {
					continue
				}
				rows = append(rows, row{id, ew.Metrics(el).Score, float64(g.Value),
					meanOf(last), float64(g.Points), true})
			}
			if len(rows) < 20 {
				continue
			}
			byPred := append([]row(nil), rows...)
			// ⚠️ Ties break on element id, and that is not tidiness — it is the
			// difference between a reproducible table and a noisy one. `rows` is
			// built by ranging a MAP, so its order is randomised per run, and
			// `sort.Slice` is NOT stable, so equal keys came out in whatever order
			// the map happened to produce. Prices move in 0.1 steps and ties are
			// everywhere, so the top-n BY PRICE genuinely changed between runs:
			// two runs of this diagnostic disagreed by up to 0.007 of skill on the
			// same population, which is the size of the differences this table is
			// read for. Predictions are floats and tie far less often, so the
			// model-ranked sets were nearly stable and the price-ranked ones were
			// not — the asymmetry is what made it visible.
			sort.Slice(byPred, func(a, b int) bool {
				if byPred[a].pred != byPred[b].pred {
					return byPred[a].pred > byPred[b].pred
				}
				return byPred[a].id < byPred[b].id
			})
			// Ranked through the shared helper rather than sorted here, so the
			// prediction benchmark's rank-band populations and this table's PRICE
			// rungs cannot drift apart. The id tie-break lives there; see its
			// comment for the run-to-run defect it fixes.
			ids, prices := make([]int, len(rows)), make([]float64, len(rows))
			for i, r := range rows {
				ids[i], prices[i] = r.id, r.price
			}
			byPrice := priceRankOrder(ids, prices)
			top, pri := map[int]map[int]bool{}, map[int]map[int]bool{}
			for _, n := range candidateSetSizes {
				tn, pn := map[int]bool{}, map[int]bool{}
				for i := 0; i < n && i < len(byPred); i++ {
					tn[byPred[i].id] = true
				}
				for i := 0; i < n && i < len(byPrice); i++ {
					pn[byPrice[i]] = true
				}
				top[n], pri[n] = tn, pn
			}
			// Disjoint bands: ranks [lo, hi] inclusive, 1-indexed. A band whose
			// window runs past the available rows contributes the rows it has.
			topBand, priBand := map[int]map[int]bool{}, map[int]map[int]bool{}
			for bi, b := range candidateBands {
				tn := map[int]bool{}
				for i := b[0] - 1; i < b[1] && i < len(byPred); i++ {
					tn[byPred[i].id] = true
				}
				topBand[bi], priBand[bi] = tn, rankWindow(byPrice, b[0], b[1])
			}
			for _, r := range rows {
				x := obs{r.pred, r.base, r.act}
				add := func(a *acc) {
					a.o = append(a.o, x)
					a.byGW[gw] = append(a.byGW[gw], x)
					a.ids[r.id] = true
				}
				add(global)
				for _, n := range candidateSetSizes {
					if top[n][r.id] {
						add(mdl[n])
					}
					if pri[n][r.id] {
						add(price[n])
					}
				}
				for bi := range candidateBands {
					if topBand[bi][r.id] {
						add(mdlBand[bi])
					}
					if priBand[bi][r.id] {
						add(priceBand[bi])
					}
				}
				if inOpt[r.id] {
					add(opt)
				}
				if r.pred >= 3.0 {
					add(held)
				}
			}
		}
		fmt.Printf("%s\n", pr.Name)
		report("all", global)
		for _, n := range candidateSetSizes {
			report(fmt.Sprintf("MODEL%d", n), mdl[n])
		}
		for _, n := range candidateSetSizes {
			report(fmt.Sprintf("PRICE%d", n), price[n])
		}
		for bi, b := range candidateBands {
			report(fmt.Sprintf("Mband%d-%d", b[0], b[1]), mdlBand[bi])
		}
		for bi, b := range candidateBands {
			report(fmt.Sprintf("Pband%d-%d", b[0], b[1]), priceBand[bi])
		}
		report("OPTIMUM", opt)
		report("HELD", held)
	}
	fmt.Printf("\n⚠️ A `skill` unchanged between `all` and the candidate sets is evidence\n")
	fmt.Printf("the model is not weaker where it matters, whatever the raw error does.\n")
	fmt.Printf("⚠️ But it is SUGGESTIVE AGAINST the wrong-loss argument, not a refutation\n")
	fmt.Printf("of it. Nothing on this table has a standard error, the cumulative rungs\n")
	fmt.Printf("are nested, and every set is scored against the same baseline over the\n")
	fmt.Printf("same weeks — so the sets are correlated with each other as well. Season\n")
	fmt.Printf("is the clustering axis, which makes six the honest count and not sixty.\n")
	fmt.Printf("⚠️ `rho gap` is the model's ordering minus the baseline's ON THE SAME ROWS,\n")
	fmt.Printf("so range restriction hits both and cancels. That is the comparison range\n")
	fmt.Printf("restriction cannot fake.\n")
}

func absOf(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
