package backtest

import (
	"fmt"
	"os"
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
//   - **MODEL40** — the forty highest-projected THIS gameweek.
//   - **PRICE40** — the forty most expensive this gameweek. ⚠️ **Model-free**, and
//     the point of it: the other sets condition on the model's own output, so the
//     model is scored on the population it selected. Price is set by FPL, is
//     per-gameweek in the archive, and is a fair proxy for "realistic pick".
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
func TestDiagCandidateSetAccuracy(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
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
	fmt.Printf("⚠️ PRICE40 is the only set the model did not choose.\n\n")
	fmt.Printf("  %-9s %6s %7s %8s %8s %8s %9s %9s %8s\n",
		"set", "players", "rows", "mae", "mae_base", "skill", "rho", "rho_base", "rho gap")

	for _, pr := range loadPairsOrSkip(t, cfg) {
		sc := SimConfig{Weights: cfg.Weights, StartGW: 1}
		sc.Weights.Horizon = 1
		global, mdl, price, opt, held := newAcc(), newAcc(), newAcc(), newAcc(), newAcc()

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
			sort.Slice(byPred, func(a, b int) bool { return byPred[a].pred > byPred[b].pred })
			byPrice := append([]row(nil), rows...)
			sort.Slice(byPrice, func(a, b int) bool { return byPrice[a].price > byPrice[b].price })
			top, pri := map[int]bool{}, map[int]bool{}
			for i := 0; i < 40 && i < len(byPred); i++ {
				top[byPred[i].id] = true
			}
			for i := 0; i < 40 && i < len(byPrice); i++ {
				pri[byPrice[i].id] = true
			}
			for _, r := range rows {
				x := obs{r.pred, r.base, r.act}
				add := func(a *acc) {
					a.o = append(a.o, x)
					a.byGW[gw] = append(a.byGW[gw], x)
					a.ids[r.id] = true
				}
				add(global)
				if top[r.id] {
					add(mdl)
				}
				if pri[r.id] {
					add(price)
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
		report("MODEL40", mdl)
		report("PRICE40", price)
		report("OPTIMUM", opt)
		report("HELD", held)
	}
	fmt.Printf("\n⚠️ A `skill` unchanged between `all` and the candidate sets means the\n")
	fmt.Printf("model is NOT weaker where it matters in any sense a decision cares about,\n")
	fmt.Printf("whatever the raw error does — and the wrong-loss argument fails.\n")
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
