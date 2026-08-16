package analysis

import (
	"sort"

	"armband/internal/stats"
)

// What money is worth, and why that falls all season.
//
// The model treats budget as a constraint: a move is affordable or it is not.
// That is incomplete. Money has a value, and the value is whatever it can be
// converted into — points — which means it depends entirely on how many
// gameweeks remain to spend it in.
//
// £0.1m banked at GW2 can be turned into a marginally better player who then
// plays 37 times. The same £0.1m at GW38 buys a better player for one match, and
// after the final whistle it is worth exactly nothing. Any decision that trades
// points against money is therefore a different decision in August and in May,
// and the model previously could not tell them apart.
//
// This is what makes the opportunity cost of an idle asset computable. A £14m
// player returning nothing is not merely losing his own points: he is holding
// capital that could be converted, and the size of that loss shrinks as the
// season runs out.

// PointsPerTenth is what an extra £0.1m converts into, in points per gameweek.
//
// Measured from the pool rather than assumed, because it is a property of the
// market and moves: it is the slope of the price/score frontier, which flattens
// as the expensive end runs out of better players to buy.
//
// The frontier is used rather than a plain regression over everyone. Most
// players are not worth buying at any price, and including them measures how bad
// the average footballer is instead of what an upgrade costs.
func (e *Engine) PointsPerTenth() float64 {
	type point struct{ price, score float64 }
	best := map[string]map[int]float64{} // position -> tenths -> best score
	for _, m := range e.AllMetrics() {
		if m.Score <= 0 || m.Status != "available" {
			continue
		}
		if best[m.Position] == nil {
			best[m.Position] = map[int]float64{}
		}
		u := tenths(m.Price)
		if s, ok := best[m.Position][u]; !ok || m.Score > s {
			best[m.Position][u] = m.Score
		}
	}

	// Within each position, walk the frontier upward and average the gain per
	// tenth across the steps that actually improve on what came before.
	var gains []float64
	for _, byPrice := range best {
		var prices []int
		for u := range byPrice {
			prices = append(prices, u)
		}
		sort.Ints(prices)
		runningBest := 0.0
		lastPrice := 0
		for _, u := range prices {
			s := byPrice[u]
			if s <= runningBest {
				continue // paying more for less; not an upgrade
			}
			if lastPrice > 0 && u > lastPrice {
				gains = append(gains, (s-runningBest)/float64(u-lastPrice))
			}
			runningBest, lastPrice = s, u
		}
	}
	if len(gains) == 0 {
		return 0
	}
	// The median, not the mean. The frontier has a few enormous steps where one
	// exceptional player sits alone at a price point, and a mean would report
	// the value of finding Haaland rather than the value of a marginal upgrade.
	//
	// ⚠️ This used to be an inline upper median, and the estimator changed with
	// the consolidation. It is a slope, so averaging the two middle steps is
	// strictly the better answer — but it is still a behaviour change, and it is
	// on the replay's transfer path at simulate.go's moneyPts. That path is
	// gated on FPL_BUDGET_WEIGHT, which is 0 unless set, so the shipped replay
	// never reaches here and is byte-identical. **Any figure measured with that
	// env var set predates this and does not reproduce.**
	return stats.Median(gains)
}

// GameweeksRemaining is how many are left to spend money in, including the one
// about to be played. Zero once the season is over, which is the correct value:
// money after the final whistle buys nothing.
func (e *Engine) GameweeksRemaining() int {
	next := e.Boot.NextEvent()
	if next == nil {
		return 0
	}
	n := GameweeksPerSeason - next.ID + 1
	if n < 0 {
		return 0
	}
	return n
}

// BudgetValue is what an amount of money is worth in points, from here to the
// end of the season.
//
// It is deliberately not a scoring term. Nothing about a player's price belongs
// in his expected points. This exists to price the *other* side of a decision —
// what banking or freeing money is worth — so that trading points for money can
// be compared on one scale.
func (e *Engine) BudgetValue(tenths int) float64 {
	if tenths > budgetValueCap {
		tenths = budgetValueCap
	}
	return float64(tenths) * e.PointsPerTenth() * float64(e.GameweeksRemaining())
}

// # Marginal only
//
// BudgetValue is a local slope and is meaningless extrapolated. Asked what £14m
// is worth it will answer several hundred points, which is nonsense: a squad
// must field fifteen players, so selling a £14m asset does not free £14m — it
// frees the difference between his selling price and whatever replaces him. The
// frontier also flattens badly at the top, where money stops buying much.
//
// Use it for the amounts it is true for: a bank balance, the £0.3m between two
// otherwise equal moves, the value of a price rise. For "should I sell this
// injured premium", correct his expected minutes and let the transfer search
// answer it — the search already prices the replacement, which is the part this
// cannot see.

// budgetValueCap is the largest amount BudgetValue will price, in tenths.
//
// Beyond about a million the linear assumption stops holding, and a caller
// asking about more than that is almost certainly asking the wrong question —
// "what is my whole premium worth" rather than "what is this margin worth".
// Clamping is friendlier than a wrong answer and louder than silence.
const budgetValueCap = 10

// Discretionary budget: what is actually yours to allocate.
//
// A squad's headline value overstates its freedom badly. Fifteen slots must be
// filled, and every position has a price floor, so a large part of any budget is
// committed before a single choice is made. A £5.5m defender is not £5.5m of
// spending: with the floor at £4.0m he is £1.5m of *decision* and £4.0m of
// obligation. A £14.0m forward against a £4.5m floor is £9.5m.
//
// This is what makes an idle premium's cost computable. Selling him does not
// free his price — you still have to field someone in that slot — it frees the
// gap above the floor. That gap is the money genuinely redeployable, and it is
// the amount worth valuing over the remaining season.
//
// Floors are read from the pool rather than hardcoded. FPL has moved them
// before, and they differ by position: at the time of writing keepers and
// defenders start at £4.0m, midfielders and forwards at £4.5m.

// PositionFloors is the cheapest available player in each position, in tenths.
func (e *Engine) PositionFloors() map[string]int {
	floors := map[string]int{}
	for _, m := range e.AllMetrics() {
		u := tenths(m.Price)
		if u <= 0 {
			continue
		}
		if f, ok := floors[m.Position]; !ok || u < f {
			floors[m.Position] = u
		}
	}
	return floors
}

// Discretionary is what a player represents above his position's floor, in
// tenths — the part of his price that reflects a choice rather than an
// obligation.
//
// sellPrice is what he would actually raise, since that is the money that would
// come back. Pass his market price when purchase prices are unknown.
func (e *Engine) Discretionary(pos string, sellPrice int) int {
	floor, ok := e.PositionFloors()[pos]
	if !ok {
		return sellPrice
	}
	if d := sellPrice - floor; d > 0 {
		return d
	}
	return 0
}

// SquadDiscretionary is the total money a squad has genuinely allocated by
// choice: the bank, plus everything each player costs above his position's
// floor.
//
// This is the number to compare against another squad's, and it is much smaller
// than team value. A £100m squad with £0.0m banked typically has around £37m of
// discretionary spending; the rest is the cost of having eleven bodies and four
// substitutes at all.
func (e *Engine) SquadDiscretionary(squad []PlayerMetrics, sell map[int]int, bank int) int {
	total := bank
	for _, p := range squad {
		s := tenths(p.Price)
		if v, ok := sell[p.ID]; ok {
			s = v
		}
		total += e.Discretionary(p.Position, s)
	}
	return total
}
