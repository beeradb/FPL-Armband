package analysis

import (
	"testing"

	"armband/internal/fpl"
)

// TestWildcardBuildAnticipatesANextWeekBenchBoost — a live wildcard rebuild
// must be told when a bench boost is chip-scheduled for the following
// gameweek, exactly as the replay's playWildcard already is
// (`cfg.wildcardBuildsForBoost() && cfg.plays(slotBenchBoost, gw+1)` in
// internal/backtest/simulate.go's playWildcard). Before this test, WeekViews'
// wildcard branch never set OptimizeRequest.BenchBoost at all, so the live
// build always used the ordinary, heavily-discounted bench weighting — even
// in the one week the chip sequence actually calls for a full-value bench.
//
// # Why the budget has to be exactly this tight
//
// A bench upgrade with any budget slack and no competing use for the money is
// taken regardless of bench weight — a positive gain at 10% weight is still a
// gain, so it is a poor discriminator. The only decision that genuinely
// depends on the bench weight is one where the SAME slack could instead buy a
// small STARTING-eleven upgrade, since a starter is always scored at full
// weight. So the pool offers exactly one discretionary upgrade of ONE cost (30
// in NowCost tenths) that can go to either side, and the budget allows exactly
// one of them, not both:
//
//   - a "superstar" MID (id 120) a little better than the fourth MID starter,
//     for the SAME +30 that
//   - a DEF "gem" bench option (id 8) costs over the DEF filler (id 7) it
//     replaces — worse than a DEF starter, so it can never simply displace one
//     outright, but a clear improvement over a scoreless bench filler.
//
// At the ordinary ~10% bench weight the superstar's fixed, fully-weighted
// gain beats the gem's heavily discounted one, so the budget goes there and
// the bench stays on the filler. Once the bench pays in full — because a
// boost is coming — the gem's raw gain (undiscounted) is the larger of the
// two, and the budget should flip to it instead. This was verified against
// the actual optimizer's bench-weight sweep before being written down: the
// flip happens between weight 0.10 (filler bench) and 0.20 (gem bench), which
// brackets both the shipped default (`DefaultBenchWeight` 0.10) and full
// bench-boost value.
func TestWildcardBuildAnticipatesANextWeekBenchBoost(t *testing.T) {
	e := loadEngine(t, nil, nil)
	e.Weights.Horizon = 5

	mk := func(id, club, pos int, cost int, xg, xa float64) fpl.Element {
		return fpl.Element{
			ID: id, Team: club, ElementType: pos, WebName: "P", NowCost: cost,
			Status: "a", Minutes: 2500, Starts: 30,
			ExpectedGoalsPer90: fpl.Num(xg), ExpectedAssistsPer90: fpl.Num(xa),
		}
	}

	var els []fpl.Element
	els = append(els, mk(1, 1, 1, 95, 1.0, 0.6)) // GK starter
	els = append(els, mk(2, 2, 1, 40, 0, 0))     // GK bench filler — fixed in every scenario

	els = append(els, mk(3, 3, 2, 95, 1.0, 0.6))  // DEF starter
	els = append(els, mk(4, 4, 2, 95, 1.0, 0.6))  // DEF starter
	els = append(els, mk(5, 5, 2, 95, 1.0, 0.6))  // DEF starter
	els = append(els, mk(6, 6, 2, 95, 1.0, 0.6))  // DEF starter
	els = append(els, mk(7, 7, 2, 40, 0, 0))      // DEF bench filler
	els = append(els, mk(8, 8, 2, 70, 0.7, 0.42)) // DEF bench "gem" — better than filler, worse than a starter

	els = append(els, mk(9, 1, 3, 95, 1.0, 0.6))       // MID starter
	els = append(els, mk(10, 2, 3, 95, 1.0, 0.6))      // MID starter
	els = append(els, mk(11, 3, 3, 95, 1.0, 0.6))      // MID starter
	els = append(els, mk(12, 4, 3, 95, 1.0, 0.6))      // MID starter (4th)
	els = append(els, mk(120, 4, 3, 125, 1.32, 0.792)) // MID "superstar" — the competing XI upgrade
	els = append(els, mk(14, 6, 3, 40, 0, 0))          // MID bench filler — fixed in every scenario

	els = append(els, mk(15, 7, 4, 95, 1.0, 0.6)) // FWD starter
	els = append(els, mk(16, 8, 4, 95, 1.0, 0.6)) // FWD starter
	els = append(els, mk(17, 1, 4, 40, 0, 0))     // FWD bench filler — fixed in every scenario
	e.Boot.Elements = els

	// Exactly enough over the all-filler, all-ordinary-starter baseline (£120.5m)
	// to buy ONE of the two +£3.0m upgrades above, never both.
	budget := 1235
	e.SquadValue, e.Bank = &budget, new(int)

	build := func(boostNextWeek bool) []PlayerMetrics {
		e.Chips = ChipSchedule{First: ChipPlan{Wildcard: 2}}
		if boostNextWeek {
			e.Chips.First.BenchBoost = 3 // gw+1
		}
		for _, v := range e.WeekViews(nil, 2, OptimizeRequest{}) {
			if v.Event == 2 && v.Rebuilt {
				return v.Bench
			}
		}
		t.Fatalf("no rebuilt wildcard squad came back at GW2")
		return nil
	}

	hasGem := func(bench []PlayerMetrics) bool {
		for _, p := range bench {
			if p.ID == 8 {
				return true
			}
		}
		return false
	}

	benchScore := func(bench []PlayerMetrics) float64 {
		var s float64
		for _, p := range bench {
			s += p.Score
		}
		return s
	}

	withoutBoost := build(false)
	withBoost := build(true)

	if hasGem(withoutBoost) {
		t.Fatalf("the DEF bench gem was bought even though no bench boost is planned; "+
			"the ordinary bench weight should prefer the MID starting upgrade instead. bench=%v",
			withoutBoost)
	}
	if !hasGem(withBoost) {
		t.Fatalf("the DEF bench gem was NOT bought even though a bench boost is planned for "+
			"the very next gameweek; the live wildcard build does not appear to be told about "+
			"the coming boost. bench=%v", withBoost)
	}

	scoreWithout, scoreWith := benchScore(withoutBoost), benchScore(withBoost)
	if scoreWith <= scoreWithout {
		t.Fatalf("wildcard bench score with a bench boost scheduled next week (%.2f) is not "+
			"higher than without one (%.2f)", scoreWith, scoreWithout)
	}
}
