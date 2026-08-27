package backtest

// What does FPL's pre-season strength rating mean in goals?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagStrengthToGoals -v
//
// A team-strength blend needs a prior available *live*, and last season's goals
// are not: the FPL API publishes no team history. What it does publish, for
// every club including a promoted one, is its own pre-season strength rating.
// So the rating is the prior — provided it can be turned into goals per match.

import (
	"context"
	"fmt"
	"math"
	"testing"
)

func TestDiagStrengthToGoals(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	ctx := context.Background()

	type obs struct{ sDef, sAtk, conceded, scored float64 }
	var all []obs
	for _, sn := range []string{"2022-23", "2023-24", "2024-25", "2025-26"} {
		s, err := Load(ctx, cfg.CacheDir, sn)
		if err != nil {
			t.Fatal(err)
		}
		gc, gf, n := map[int]float64{}, map[int]float64{}, map[int]float64{}
		for _, f := range s.Fixtures {
			if f.TeamHScore == nil || f.TeamAScore == nil {
				continue
			}
			gc[f.TeamH] += float64(*f.TeamAScore)
			gf[f.TeamH] += float64(*f.TeamHScore)
			gc[f.TeamA] += float64(*f.TeamHScore)
			gf[f.TeamA] += float64(*f.TeamAScore)
			n[f.TeamH]++
			n[f.TeamA]++
		}
		for _, tm := range s.Teams {
			if n[tm.ID] == 0 {
				continue
			}
			all = append(all, obs{
				sDef:     float64(tm.StrengthDefenceHome+tm.StrengthDefenceAway) / 2,
				sAtk:     float64(tm.StrengthAttackHome+tm.StrengthAttackAway) / 2,
				conceded: gc[tm.ID] / n[tm.ID],
				scored:   gf[tm.ID] / n[tm.ID],
			})
		}
	}

	fit := func(x, y func(obs) float64, label string) (a, b float64) {
		var mx, my float64
		for _, o := range all {
			mx += x(o)
			my += y(o)
		}
		mx /= float64(len(all))
		my /= float64(len(all))
		var num, den float64
		for _, o := range all {
			num += (x(o) - mx) * (y(o) - my)
			den += (x(o) - mx) * (x(o) - mx)
		}
		b = num / den
		a = my - b*mx
		var ssr, sst float64
		for _, o := range all {
			p := a + b*x(o)
			ssr += (y(o) - p) * (y(o) - p)
			sst += (y(o) - my) * (y(o) - my)
		}
		fmt.Printf("%-28s goals = %+.5f x strength %+.3f   R2 %.3f   rms %.3f   mean %.2f\n",
			label, b, a, 1-ssr/sst, math.Sqrt(ssr/float64(len(all))), my)
		return a, b
	}

	fmt.Printf("\nFPL's pre-season strength against what actually happened, %d club-seasons.\n\n", len(all))
	fit(func(o obs) float64 { return o.sDef }, func(o obs) float64 { return o.conceded }, "defence -> conceded")
	fit(func(o obs) float64 { return o.sAtk }, func(o obs) float64 { return o.scored }, "attack -> scored")

	// How good is the rating against simply guessing the league average? That is
	// the bar a prior has to clear to be worth carrying.
	var mc, ms float64
	for _, o := range all {
		mc += o.conceded
		ms += o.scored
	}
	mc /= float64(len(all))
	ms /= float64(len(all))
	var baseC, baseS float64
	for _, o := range all {
		baseC += (o.conceded - mc) * (o.conceded - mc)
		baseS += (o.scored - ms) * (o.scored - ms)
	}
	fmt.Printf("\nleague average alone: conceded rms %.3f, scored rms %.3f\n",
		math.Sqrt(baseC/float64(len(all))), math.Sqrt(baseS/float64(len(all))))
}
