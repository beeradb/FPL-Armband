package backtest

import (
	"fmt"
	"sort"
	"testing"

	"armband/internal/stats"
)

// WHEN SHOULD THE FIRST-HALF WILDCARD BE PLAYED?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagFirstHalfWildcardWeek -v -count=1 -timeout 180m
//
// # PRE-REGISTERED PREDICTION, recorded 2026-08-26 BEFORE the first run
//
// **The owner predicts GW4-8, and expects it to be EARLIER than a symmetric model
// would say.** Recorded here before any result existed so it cannot be retrofitted
// to whatever comes out. The reasoning is that the optimum is set by when "the
// template" is known — the consensus set of must-own players — rather than by
// halfway through the window.
//
// A symmetric drift model predicts the MIDPOINT, about GW10. It gets there by
// assuming a rebuild resets the gap to zero whenever it happens, so the only thing
// that matters is splitting the window evenly. **That assumption is what this test
// exists to break.**
//
// # Why the symmetric model is wrong
//
// A wildcard at GW3 does not buy the right fifteen. It buys the fifteen that looks
// right after three gameweeks of noise — before promoted sides are sorted, new
// signings have settled, or the bandwagons have formed. That squad then drifts
// FAST, because it was built for a template that did not exist yet.
//
// So the trade is not symmetric:
//
//   - **Reset early**: stop bleeding sooner, but buy the wrong squad.
//   - **Reset late**: bleed longer, but buy a squad that stays right.
//
// That has an interior optimum with no reason to sit at the midpoint.
//
// # The measurement, which assumes none of the above
//
// A wildcard at week t hands you exactly the point-in-time optimal squad at t, so
// `wc(t)` is the same object this harness already computes as the ideal. The cost
// of choosing t is then entirely measurable:
//
//	total(t) = Σ(s<t) gap(opening squad, s)  +  Σ(s≥t) gap(wc(t), s)
//
// The first term is what you bleed holding the opener until you act; the second is
// what the REBUILT squad bleeds afterwards, which is where "the template was not
// known yet" shows up without being assumed. Nothing here models information
// arrival — if the effect is real it appears as a rebuilt squad that decays faster
// the earlier it was built.
//
// ⚠️ **The window ends at GW%d, not GW38.** There are two wildcards, one a half,
// and the second is a CALENDAR decision aimed at doubles and blanks. The first
// chip's runway therefore ends when the second arrives, because the second resets
// the drift anyway. Scoring the first to GW38 credits it with weeks the second was
// always going to fix.
//
// ⚠️ **The oracle is hindsight** and is an upper bound no live rule can reach. It
// is here to size the prize, not to be a target.
func TestDiagFirstHalfWildcardWeek(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	last := ChipResetGW - 1

	type cell struct {
		season  string
		start   int
		total   []float64 // by candidate week
		fwd     []float64 // the rebuilt squad's mean gap over its first 5 weeks
		oracle  int
		best    float64
		atFixed map[int]float64
	}
	var cells []cell

	for _, pr := range loadPairsOrSkip(t, cfg) {
		for _, start := range sweepStarts() {
			if last-start < 8 {
				continue // no window to decide in
			}
			sc := SimConfig{Weights: cfg.Weights, StartGW: start, BankUpTo: 5}
			sc.Weights.Horizon = 1

			e0, _ := EngineAt(pr.Cur, pr.Prior, start-1, sc)
			opening, ok := repairSquad(e0, nil, 1000, 0, sc)
			if !ok {
				continue
			}
			w := newWallet(1000)
			for _, id := range opening {
				w.buy(id, marketPrice(pr.Cur, id, start-1))
			}

			// Pass 1: the squad a wildcard at each week would hand you, which is
			// exactly the point-in-time optimum at that week.
			ideal := map[int][]int{}
			for gw := start; gw <= last; gw++ {
				ew, _ := EngineAt(pr.Cur, pr.Prior, gw-1, sc)
				sq, ok := repairSquad(ew, nil, w.value(pr.Cur, opening, gw-1), 0, sc)
				if !ok {
					break
				}
				ideal[gw] = sq
			}
			if len(ideal) < 9 {
				continue
			}

			// Pass 2: score every candidate squad in every week, one engine per
			// week rather than one per (candidate, week).
			gapFrozen := map[int]float64{}
			gap := map[int]map[int]float64{}
			for gw := start; gw <= last; gw++ {
				if ideal[gw] == nil {
					continue
				}
				ew, _ := EngineAt(pr.Cur, pr.Prior, gw-1, sc)
				base := xiPoints(ew, ideal[gw])
				gapFrozen[gw] = base - xiPoints(ew, opening)
				for tt := start; tt <= gw; tt++ {
					if ideal[tt] == nil {
						continue
					}
					if gap[tt] == nil {
						gap[tt] = map[int]float64{}
					}
					gap[tt][gw] = base - xiPoints(ew, ideal[tt])
				}
			}

			c := cell{season: pr.Name, start: start, atFixed: map[int]float64{}}
			for tt := start; tt <= last; tt++ {
				if gap[tt] == nil {
					c.total, c.fwd = append(c.total, 0), append(c.fwd, 0)
					continue
				}
				var tot float64
				for s := start; s < tt; s++ {
					tot += gapFrozen[s]
				}
				var fwd float64
				var n int
				for s := tt; s <= last; s++ {
					tot += gap[tt][s]
					if s-tt < 5 {
						fwd += gap[tt][s]
						n++
					}
				}
				c.total = append(c.total, tot)
				if n > 0 {
					c.fwd = append(c.fwd, fwd/float64(n))
				} else {
					c.fwd = append(c.fwd, 0)
				}
			}
			c.best, c.oracle = c.total[0], start
			for i, v := range c.total {
				if v < c.best {
					c.best, c.oracle = v, start+i
				}
			}
			for _, f := range []int{4, 6, 8, 10, 12} {
				if i := f - start; i >= 0 && i < len(c.total) {
					c.atFixed[f] = c.total[i]
				}
			}
			cells = append(cells, c)
		}
	}
	if len(cells) < 4 {
		t.Skip("not enough cells")
	}

	fmt.Printf("\n=== WHEN TO PLAY THE FIRST-HALF WILDCARD (window ends GW%d)\n", last)
	fmt.Printf("PRE-REGISTERED: the owner predicted GW4-8, earlier than the GW10 a\n")
	fmt.Printf("symmetric drift model gives. Recorded before this was first run.\n\n")
	fmt.Printf("%-9s %6s %8s %10s", "season", "entry", "oracle", "carried")
	for _, f := range []int{4, 6, 8, 10, 12} {
		fmt.Printf(" %8s", fmt.Sprintf("GW%d", f))
	}
	fmt.Printf("\n")
	var oracles []float64
	regret := map[int][]float64{}
	for _, c := range cells {
		fmt.Printf("%-9s %6d %8d %10.1f", c.season, c.start, c.oracle, c.best)
		for _, f := range []int{4, 6, 8, 10, 12} {
			if v, ok := c.atFixed[f]; ok {
				fmt.Printf(" %8.1f", v-c.best)
				regret[f] = append(regret[f], v-c.best)
			} else {
				fmt.Printf(" %8s", "-")
			}
		}
		fmt.Printf("\n")
		oracles = append(oracles, float64(c.oracle))
	}
	sort.Float64s(oracles)
	fmt.Printf("\ncells %d | oracle week: min %.0f median %.0f max %.0f\n",
		len(cells), oracles[0], stats.Median(oracles), oracles[len(oracles)-1])
	fmt.Printf("(columns after `carried` are REGRET against that cell's own oracle)\n\n")
	fmt.Printf("%8s %10s %8s\n", "fixed at", "mean regret", "n")
	for _, f := range []int{4, 6, 8, 10, 12} {
		if v := regret[f]; len(v) > 0 {
			fmt.Printf("%8d %10.1f %8d\n", f, meanOf(v), len(v))
		}
	}

	fmt.Printf("\n=== DOES A LATER REBUILD DECAY SLOWER? (the template, measured)\n")
	fmt.Printf("Mean gap the REBUILT squad carries over its first five weeks. If the\n")
	fmt.Printf("template is the mechanism, this FALLS with the week of the rebuild and\n")
	fmt.Printf("flattens once the template is known. If it is flat throughout, an early\n")
	fmt.Printf("rebuild is no worse than a late one and the symmetric model was right.\n\n")
	fmt.Printf("%8s %10s %8s\n", "rebuilt", "fwd gap", "n")
	for gw := 2; gw <= last; gw++ {
		var v []float64
		for _, c := range cells {
			if i := gw - c.start; i >= 0 && i < len(c.fwd) && c.fwd[i] > 0 {
				v = append(v, c.fwd[i])
			}
		}
		if len(v) >= 4 {
			fmt.Printf("%8d %10.2f %8d\n", gw, meanOf(v), len(v))
		}
	}
	fmt.Printf("\n⚠️ This column is the whole test of the template mechanism, and it is\n")
	fmt.Printf("measured rather than modelled — nothing here represents information\n")
	fmt.Printf("arrival. If it does not fall, the mechanism is not present in this data.\n")
}
