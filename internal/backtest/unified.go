package backtest

import (
	"os"
	"strconv"

	"armband/internal/analysis"
)

// The unified transfer decision: one search for both jobs.
//
// The weekly policy runs a bespoke structure — one funded pair, then a run of
// independent single swaps — which cannot express two funded upgrades in the
// same week and duplicates a search that Optimize already performs. With
// OptimizeRequest.MaxChanges, "the best squad within k changes of the one I own"
// covers every one of those cases and several the bespoke structure cannot
// reach.
//
// # It loses, and it is kept so nobody rebuilds it
//
// Enable with FPL_UNIFIED_TRANSFERS=1. It ships off because it costs points:
//
//	              2023-24  2024-25  2025-26   mean
//	bespoke          2139     2344     2132   2205
//	unified          2080     2327     2090   2166
//
// All three seasons, -39 on the mean. The opening squad is identical either way
// -- both replays hold at 1702 -- so every point of that is transfer decisions:
// on 2023-24 the bespoke policy's moves were worth +437 for 8 points of hits,
// this one's +378 for 12.
//
// It is not churn, and it is not the search being unable to find good squads:
// TestDiagBoundedRevision has it winning at three changes on the same squads.
// It is the argmax problem again, now over the *number* of transfers. Choosing
// k by net value picks the k whose estimate is most flattering, and a larger k
// changes more players, so its gain compounds more estimation error. The
// bespoke structure -- one funded pair, then singles, each gated on its own --
// looks clumsier and is more conservative, and conservatism is what a search
// living in the tail of an error distribution needs.
//
// The lesson is narrow and worth keeping: a search that is better at finding the
// optimum of a noisy objective is not automatically better at deciding.
//
// # Charging more for doing several things at once helps here, and only here
//
// multiMoveSurcharge escalates the per-move charge with move count, on the
// argument that a three-way restructure is judged on one combined estimate and
// so compounds more error than three separate decisions would. It works, and it
// confirms the diagnosis above:
//
//	surcharge  2023-24  2024-25  2025-26   mean
//	        0     2080     2327     2090   2166
//	        2     2122     2330     2128   2193
//
// +27 of the 39-point gap, and it generalises — the value was chosen on 2023-24
// alone and improved both other seasons. It still does not reach 2205.
//
// Applied to the shipped policy it *loses*: 2139 at zero against 2070 at two and
// 2120 at three on 2023-24. That is not a contradiction, it is the same finding
// twice. The bespoke policy already prices this, structurally rather than as a
// constant: a funded pair must beat spending the free transfer on the best
// single move and keeping the four points, so the more moves it wants the more
// it has to prove. Adding a surcharge on top charges for it a second time.
//
// So: the intuition is right, the shipped policy already implements it, and it
// implements it better than a constant does.
var unifiedTransfers = os.Getenv("FPL_UNIFIED_TRANSFERS") != ""

// multiMoveSurcharge is charged on top of the per-move cost, escalating: the
// second move in a week costs one of these, the third two, and so on. It prices
// the fact that simultaneous moves share one estimate rather than each carrying
// its own. Swept via FPL_MULTI_SURCHARGE.
var multiMoveSurcharge = envFloat("FPL_MULTI_SURCHARGE")

// surchargeFor prefers the config value so a sweep does not need the env var.
func surchargeFor(cfg SimConfig) float64 {
	if cfg.UnifiedSurcharge != 0 {
		return cfg.UnifiedSurcharge
	}
	return multiMoveSurcharge
}

// budgetWeight scales what money freed by a transfer is worth. Zero reproduces
// the behaviour before money had a value at all, and zero is what ships.
//
//	weight  2023-24  2024-25  2025-26   mean  transfers  value
//	     0     2131     2288     2103   2174       32.3  +0.27m
//	  0.05     2116     2343     2103   2187       31.0  +0.57m
//	  0.15     2101     2232     2085   2139       28.0  +0.27m
//	   0.4     2052     2155     2033   2080       21.0  -1.00m
//
// The +13 at 0.05 is one season moving (+55 on 2024-25 while 2023-24 loses 15
// and 2025-26 does not move at all), it is the best of four swept values, and
// season swings here are ±60. It is noise.
//
// # It does not do what it was built to do
//
// The intent was to let the policy bank value early, when money still has time
// to be converted, and stop caring late. The predicted signature was more
// activity early and none late.
//
// Transfers fall monotonically instead: 32, 31, 28, 21. Almost every improving
// move is an *upgrade*, which spends money rather than freeing it, so the term
// is negative on the moves the policy actually wants to make. Adding a linear
// money term to the gain is therefore not a time-varying value signal — it is a
// flat brake on buying anyone more expensive, and it brakes hardest on exactly
// the moves worth making.
//
// Encouraging early value-banking would need something asymmetric: rewarding
// money freed without symmetrically punishing money spent. That is a different
// term, and it is not obviously a good idea either, since a policy that banks
// value it never spends has converted points into a number on a screen.
var budgetWeight = envFloat("FPL_BUDGET_WEIGHT")

// EnvFloat reads a float from the environment, returning 0 when unset or
// unparseable — so an absent switch and a mistyped one are the same "off", which is
// the behaviour every diagnostic here already assumed.
//
// Exported because `cmd/armband` had a byte-identical copy. Two implementations of
// "how this project reads a numeric switch" is small, but it is the shape that lets
// one of them start trimming whitespace or erroring while the other does not, and
// the switches it reads decide which arm a recorded figure was measured under.
func EnvFloat(name string) float64 { return envFloat(name) }

func envFloat(name string) float64 {
	v, err := strconv.ParseFloat(os.Getenv(name), 64)
	if err != nil {
		return 0
	}
	return v
}

// unifiedDecide picks this week's transfers by asking for the best squad at
// each affordable number of changes and charging for the ones it would use.
//
// The cost model is deliberately identical to the bespoke policy's, because
// every part of it was paid for: a hit costs four, a free transfer costs
// FreeCost because it could have bought something else, and both are charged
// per move rather than per week — charging the week once scored 2110 against
// 2151 and brought the churn back.
func unifiedDecide(e *analysis.Engine, s *Season, held []int, bank, free, limit, gw int,
	horizon float64, freeCost float64, cfg SimConfig, sell map[int]int) ([]Move, int) {

	if limit < 1 {
		return nil, 0
	}

	byID := map[int]analysis.PlayerMetrics{}
	for _, m := range e.AllMetrics() {
		byID[m.ID] = m
	}
	squad := make([]analysis.PlayerMetrics, 0, len(held))
	value := 0
	for _, id := range held {
		m, ok := byID[id]
		if !ok {
			return nil, 0
		}
		squad = append(squad, m)
		// Selling value, not market value: half of every rise is FPL's.
		if v, ok := sell[id]; ok {
			value += v
		} else {
			value += int(m.Price*10 + 0.5)
		}
	}
	if len(squad) != analysis.SquadSize {
		return nil, 0
	}
	base := analysis.XIValue(squad)

	// Budget is what the squad is worth plus what is in the bank. Capping at
	// £100m would be the pre-season constraint applied to a squad whose price
	// has moved since, and it rejects any move costing more than it frees.
	budget := value + bank

	var bestMoves []Move
	bestHits := 0
	bestNet := 0.0

	for k := 1; k <= limit; k++ {
		// The pool floor is a real asymmetry with the bespoke search, which
		// ranks over e.AllMetrics() and applies no floor at all. Handicapping
		// one arm with a filter the other does not have measures the filter, not
		// the search, so it is configurable and the comparison is run both ways.
		floor := cfg.UnifiedPoolFloor
		switch {
		case floor == 0:
			floor = 55
		case floor < 0:
			floor = 0
		}
		got, err := e.Optimize(analysis.OptimizeRequest{
			Budget: budget, MinMinutes: 600, MinExpectedMinutes: floor,
			BenchWeight: 0, CurrentSquad: held, MaxChanges: k,
		})
		if err != nil {
			continue
		}
		moves := diffSquads(held, byID, got.Players, gw)
		if len(moves) == 0 {
			continue
		}
		gain := analysis.XIValue(got.Players) - base
		n := len(moves)

		// The gain threshold is per *move*, not per decision.
		//
		// This was the sharpest asymmetry with the bespoke search, and it ran
		// the wrong way. Bespoke gates each swap at MinGain inside a loop, so a
		// three-move week has to clear 3 x MinGain in total. Unified applied the
		// same constant to the whole package, so its bar got weaker in
		// proportion to k — at exactly the point more evidence is needed, not
		// less.
		//
		// The correction is derived rather than swept: it reuses MinGain and
		// introduces no constant of its own.
		//
		// It was originally justified by a measured per-player over-rating of
		// about 0.53 pts/gw, so that a k-move revision inflates its own gain by
		// roughly k x 0.53. **That figure is retracted** — at shipped config the
		// buy side is −0.230 at the median and the predicted asymmetry is not
		// there at all. What survives is the consistency argument, which never
		// needed a size: two searches solving the same problem must not apply
		// the same constant at different granularities, and bespoke already
		// gates per move.
		//
		// Note this is a **no-op at the shipped settings** — per-move and
		// per-decision gating produce byte-identical seasons, because k-move
		// packages clear the bar either way at MinGain 0.4. It is kept because
		// it is the correct rule and would bind if that constant ever rises.
		//
		// Set UnifiedGainPerDecision to restore the old behaviour and measure
		// the difference.
		bar := cfg.MinGain * float64(n)
		if cfg.UnifiedGainPerDecision {
			bar = cfg.MinGain
		}
		if gain < bar {
			continue
		}

		hitsNeeded := 0
		if free < n {
			hitsNeeded = n - free
		}
		if hitsNeeded > cfg.MaxHits {
			continue
		}
		// Escalating charge for doing several things at once.
		//
		// Every move is priced the same today, which treats a three-way
		// restructure as three independent one-move decisions. It is not: the
		// three are evaluated together on a single combined estimate, and the
		// more players change the more estimation error that estimate
		// compounds. Charging move i an extra surcharge*(i-1) makes the search
		// demand progressively more evidence before acting on progressively
		// less reliable arithmetic. Zero reproduces flat per-move pricing.
		//
		// The bar check above is kept where it is rather than folded into the
		// proposal's GainBar, because it scales with the move count and the
		// proposal carries an absolute bar. Everything else routes through the
		// one gate — see gate.go for why four expressions of this rule was the
		// problem.
		p := transferProposal{
			Moves: moves, Gain: gain, Hits: hitsNeeded,
			Surcharge: surchargeFor(cfg) * float64(n*(n-1)) / 2,
			Strict:    true, GainBar: noGainBar,
			Horizon: horizon, FreeCost: freeCost, GW: gw,
		}
		net := p.value()
		if !acceptTransfer(cfg, s, p) {
			continue
		}
		// Choosing k by net value is a *selection* among accepted packages, not an
		// accept, so it stays outside the gate. It is also the thing this search
		// loses points to — an argmax over k picks whichever k has the most
		// flattering estimate — which is a reason to keep it visible rather than
		// buried in a predicate.
		if net <= bestNet {
			continue
		}

		for i := range moves {
			moves[i].Gain = 0
			if i == 0 {
				moves[i].Gain = gain // reported once, on the group
			}
			moves[i].Hit = i < hitsNeeded
		}
		bestMoves, bestHits, bestNet = moves, hitsNeeded, net
	}
	return bestMoves, bestHits
}

// diffSquads turns "squad before, squad after" into the moves between them.
//
// The search returns a destination rather than a path, so the pairing here is
// arbitrary beyond position: what leaves and what arrives is determined, which
// player is nominally swapped for which is not. Positions are matched so the
// report reads sensibly.
func diffSquads(held []int, byID map[int]analysis.PlayerMetrics,
	got []analysis.PlayerMetrics, gw int) []Move {

	after := map[int]bool{}
	for _, p := range got {
		after[p.ID] = true
	}
	before := map[int]bool{}
	for _, id := range held {
		before[id] = true
	}

	var out, in []analysis.PlayerMetrics
	for _, id := range held {
		if !after[id] {
			out = append(out, byID[id])
		}
	}
	for _, p := range got {
		if !before[p.ID] {
			in = append(in, p)
		}
	}
	if len(out) != len(in) {
		return nil
	}

	moves := make([]Move, 0, len(out))
	used := make([]bool, len(in))
	for _, o := range out {
		pick := -1
		for i, c := range in {
			if used[i] || c.Position != o.Position {
				continue
			}
			pick = i
			break
		}
		if pick < 0 {
			return nil // positions do not reconcile; not a legal set of moves
		}
		used[pick] = true
		moves = append(moves, Move{
			GW: gw, Out: o.Name, In: in[pick].Name,
			OutID: o.ID, InID: in[pick].ID,
			OutScore: o.Score, InScore: in[pick].Score,
		})
	}
	return moves
}
