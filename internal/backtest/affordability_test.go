package backtest

// Does team value compound, and does losing that race price you out?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagAffordability -v -timeout 1h
//
// The theory: FPL pays you **half of any price rise, rounded down**, while
// everyone else's purchase price moves in full. So a player you own has to rise
// £0.2m before your selling price moves £0.1m, and a squad that does not
// accumulate value steadily loses access to the best players — who are exactly
// the players that rise hardest.
//
// If that is right, two things should be visible over a season: team value
// should grow, and the share of the league's best players a squad can actually
// reach should *fall*. If it is wrong, affordability is flat and team value is
// a number on a screen rather than a constraint.
//
// # Affordability is not "can I cover the price"
//
// A fifteen is always full, so buying anyone means selling someone. The honest
// test is therefore whether a player is reachable by a *legal single transfer*:
// sell the worst player in his position, add the bank, and see whether that
// covers his market price. That is the constraint a manager actually faces, and
// it is much tighter than comparing a price against total squad value.
//
// # The counterfactual that isolates the rule
//
// Every figure is computed twice: once with FPL's real selling prices, and once
// pricing sales at market. The gap between them is precisely what the
// half-of-any-rise rule costs in reach, which is the quantity the theory is
// about — as distinct from prices simply inflating for everyone, which would
// show up in both arms equally.

import (
	"fmt"
	"sort"
	"testing"
)

func TestDiagAffordability(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)

	pairs := sweepPairNames()
	// Entry at GW1 only. Team value compounds over the whole season, so a late
	// entry has too few gameweeks for the effect to accumulate and would dilute
	// exactly the signal being looked for.
	checkpoints := []int{5, 10, 15, 20, 25, 30, 35}

	type row struct {
		value, bank         int
		reachReal, reachMkt float64
		topOwned            float64
		// The rule's cost measured on the squad directly, rather than inferred
		// from whichever player funds a buy. risersHeld counts the fifteen worth
		// more than was paid; ruleCost is what the half-of-any-rise rule takes
		// across all of them, in tenths.
		risersHeld int
		ruleCost   int
		// Whether the *worst* player in each position has himself risen. This is
		// the assumption worth measuring rather than asserting: five defender
		// and five midfielder slots leave room for cheap bodies, and the second
		// keeper is conventionally a £4.0m player who never appears — but only
		// three forward slots exist, so the worst forward is usually a real
		// footballer who may well have risen.
		worstRisen map[int]bool
	}
	byGW := map[int][]row{}

	for _, pair := range pairs {
		prior := loadSeason(t, cfg, pair[0])
		cur := loadSeason(t, cfg, pair[1])
		sc := sweepConfig(cfg, 1, false)
		res, err := Simulate(cur, prior, sc)
		if err != nil {
			t.Fatal(err)
		}

		// The twenty players who actually scored most this season. Hindsight on
		// purpose: the question is not who the model wanted but whether the best
		// players stayed reachable at all.
		type scored struct {
			id, pts, typ int
		}
		var all []scored
		for id, p := range cur.Players {
			var pts int
			for _, g := range p.GWs {
				pts += g.Points
			}
			all = append(all, scored{id: id, pts: pts, typ: p.Type})
		}
		sort.Slice(all, func(i, j int) bool { return all[i].pts > all[j].pts })
		top := all
		if len(top) > 20 {
			top = top[:20]
		}

		for _, wk := range res.Weeks {
			if len(wk.Squad) == 0 || wk.Sell == nil {
				continue
			}
			hit := false
			for _, c := range checkpoints {
				if wk.GW == c {
					hit = true
				}
			}
			if !hit {
				continue
			}

			// Cheapest disposable player per position, by real and by market
			// price. Selling the worst in a position is the move that funds a
			// buy in it.
			worstReal := map[int]int{}
			worstMkt := map[int]int{}
			for _, id := range wk.Squad {
				p := cur.Players[id]
				if p == nil {
					continue
				}
				mkt := priceAt(p, wk.GW-1)
				real := wk.Sell[id]
				if v, ok := worstReal[p.Type]; !ok || real < v {
					worstReal[p.Type] = real
				}
				if v, ok := worstMkt[p.Type]; !ok || mkt < v {
					worstMkt[p.Type] = mkt
				}
			}

			owned := map[int]bool{}
			for _, id := range wk.Squad {
				owned[id] = true
			}

			var reachR, reachM, own float64
			for _, tp := range top {
				if owned[tp.id] {
					own++
					reachR++
					reachM++
					continue
				}
				p := cur.Players[tp.id]
				if p == nil {
					continue
				}
				price := priceAt(p, wk.GW-1)
				if wr, ok := worstReal[tp.typ]; ok && wr+wk.Bank >= price {
					reachR++
				}
				if wm, ok := worstMkt[tp.typ]; ok && wm+wk.Bank >= price {
					reachM++
				}
			}
			// What the rule actually costs this squad, measured on every held
			// player rather than inferred from whoever funds a buy. A rise is
			// only shared if the player is worth more than was paid; falls are
			// taken in full and cost nothing.
			risers, cost := 0, 0
			for _, id := range wk.Squad {
				mkt := marketPrice(cur, id, wk.GW-1)
				real := wk.Sell[id]
				if mkt > real {
					risers++
					cost += mkt - real
				}
			}
			// And whether the *funding* player — the worst in a position — is
			// himself a riser, which is the assumption that needs checking
			// rather than asserting. Only 3 forward and 2 keeper slots exist, so
			// "the worst forward" is often a real player, not bench fodder.
			fundRisen := map[int]bool{}
			for typ, wr := range worstReal {
				if wm, ok := worstMkt[typ]; ok && wm > wr {
					fundRisen[typ] = true
				}
			}
			n := float64(len(top))
			byGW[wk.GW] = append(byGW[wk.GW], row{
				value: wk.Value, bank: wk.Bank,
				reachReal: reachR / n, reachMkt: reachM / n, topOwned: own / n,
				risersHeld: risers, ruleCost: cost, worstRisen: fundRisen,
			})
		}
	}

	fmt.Printf("\n%s, entry at GW1. 'reachable' is the share of the season's\n", seasonsLabel(len(pairs)))
	fmt.Printf("top-20 scorers obtainable in one legal transfer: sell the worst player\n")
	fmt.Printf("in that position, add the bank, cover his price. Already-owned counts.\n\n")
	fmt.Printf("%-5s %9s %7s %10s %9s %8s %9s   %s\n",
		"GW", "team val", "bank", "reachable", "owned", "risers", "rule cost",
		"worst-in-pos has risen")
	fmt.Printf("%-5s %9s %7s %10s %9s %8s %9s   %4s %4s %4s %4s\n",
		"", "(£m)", "(£m)", "(real)", "of top20", "held /15", "(£m)",
		"GKP", "DEF", "MID", "FWD")

	var gws []int
	for gw := range byGW {
		gws = append(gws, gw)
	}
	sort.Ints(gws)
	for _, gw := range gws {
		rows := byGW[gw]
		var v, b, rr, ow, ri, rc float64
		byPos := map[int]float64{}
		for _, r := range rows {
			v += float64(r.value)
			b += float64(r.bank)
			rr += r.reachReal
			ow += r.topOwned
			ri += float64(r.risersHeld)
			rc += float64(r.ruleCost)
			for typ := range r.worstRisen {
				byPos[typ]++
			}
		}
		n := float64(len(rows))
		fmt.Printf("%-5d %9.1f %7.1f %9.0f%% %8.0f%% %8.1f %9.2f   %3.0f%% %3.0f%% %3.0f%% %3.0f%%\n",
			gw, v/n/10, b/n/10, 100*rr/n, 100*ow/n, ri/n, rc/n/10,
			100*byPos[1]/n, 100*byPos[2]/n, 100*byPos[3]/n, 100*byPos[4]/n)
	}

	fmt.Printf("\n'risers held' counts the fifteen worth more than was paid; 'rule cost' is\n")
	fmt.Printf("what the half-of-any-rise rule takes across all of them.\n\n")
	fmt.Printf("The last four columns test the assumption that funding a buy means selling\n")
	fmt.Printf("cheap fodder whose price never moved. That is not safe to assume: five defender\n")
	fmt.Printf("and midfielder slots leave room for cheap bodies, and the second keeper is\n")
	fmt.Printf("conventionally a £4.0m player who never appears — but only three forward slots\n")
	fmt.Printf("exist, so the worst forward is usually a real footballer who may well have risen.\n")
}
