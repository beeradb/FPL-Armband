package backtest

// How far a squad is from ideal, in expected points on the starting eleven.
//
// # Why a count of changes is the wrong measure, and this is the right one
//
// `changesBetween` counts held players absent from a fresh optimum. It is what
// `repairChanges` reads, and it treats every player alike: a £4.0m bench-fodder
// swap scores exactly the same as replacing a captain. The user's objection is
// the whole reason this exists — *"switching a benched player is basically never
// worth a transfer"* — and it is correct on the code.
//
// A count also cannot say how BAD a squad is, only how DIFFERENT. Two squads a
// long way from the optimum in move-count terms can be worth almost the same
// points if the differences are all on the bench, and a squad one move away can
// be losing eight points a week if that move is a premium forward. **Distance in
// player-identity and distance in points are different quantities**, and only the
// second is what a manager is actually deciding about.
//
// # What this measures
//
// The expected points the fielded eleven gives up by being the squad it is rather
// than the squad the optimiser would build with the same money. Both sides are
// run through `analysis.BestXI`, so the comparison is eleven-against-eleven with
// each squad shown at its best.
//
// # ⚠️ The bench is deliberately excluded, and so is the captain
//
// **The bench**, because that is the objection this answers. A bench difference
// that never reaches the pitch is not a cost worth a transfer.
//
// ⚠️ It is excluded, not free: an eleven is drawn from fifteen, so a squad with a
// weak bench fields a worse eleven the moment anyone is injured or rotated. This
// measures the gap on a healthy week and **understates the cost of a thin squad**.
// Bench value has its own measured term and this is not a substitute for it.
//
// **The captain** because captaincy is a WEEKLY decision, not a property of the
// squad. Any squad can captain its own best player, so doubling one score on both
// sides would mostly cancel while adding the noisiest single term in the model to
// a quantity meant to describe standing quality. A wildcard trigger asking "is my
// team bad" must not fire because this week's captain pick is unlucky.
//
// # ⚠️ This is an ARGMAX distance and it is never zero
//
// `repairSquad` is an unconstrained optimum over the whole pool. The optimiser
// picking a different eleven than yours is the expected case, not evidence of
// damage, and the same argmax caveat that governs the wildcard-value reading
// governs this: **a standing positive drift is what a noisy ranking produces even
// against a squad nobody would change.** Read the SERIES, or read it against a
// control; do not read a single number as "points left on the table".

import "armband/internal/analysis"

// XIDrift is one squad's distance from the optimiser's, in expected points.
type XIDrift struct {
	// Held and Fresh are the two elevens' summed Score.
	Held, Fresh float64
	// Drift is Fresh - Held: what the optimum's eleven is worth above this one.
	// Positive means the held squad is behind, which is the ordinary case.
	Drift float64
	// Changes is the old move-count over all fifteen, carried alongside so the
	// two can be compared on the same cell rather than in separate runs. It is
	// the quantity this measure replaces, not a second opinion about the same
	// one.
	Changes int
}

// xiPoints is a squad's best eleven, summed on Score.
//
// It reuses `analysis.BestXI` — the same formation search the simulation fields a
// team with — rather than taking the top eleven by Score, which would field
// illegal formations and flatter squads that are strong in one position.
func xiPoints(e *analysis.Engine, squad []int) float64 {
	var ms []analysis.PlayerMetrics
	for _, id := range squad {
		if el := e.Boot.ElementByID(id); el != nil {
			ms = append(ms, e.Metrics(el))
		}
	}
	xi, _, _ := analysis.BestXI(ms)
	var sum float64
	for _, p := range xi {
		sum += p.Score
	}
	return sum
}

// xiDriftOf measures `held` against the unconstrained optimum at the same budget.
//
// Returns ok=false when the optimiser cannot build a squad, which is the same
// condition `repairChanges` reports and means "no reading", never "a drift of
// zero" — a zero would read as a perfect squad.
func xiDriftOf(e *analysis.Engine, held []int, budget int, minExp float64,
	cfg SimConfig) (XIDrift, bool) {

	fresh, ok := repairSquad(e, held, budget, minExp, cfg)
	if !ok {
		return XIDrift{}, false
	}
	h, f := xiPoints(e, held), xiPoints(e, fresh)
	return XIDrift{
		Held:    h,
		Fresh:   f,
		Drift:   f - h,
		Changes: changesBetween(held, fresh),
	}, true
}

// xiDriftSeries is the gap between the held eleven and the optimum's, PER
// GAMEWEEK over the next `n`, priced on a fixture-aware engine.
//
// # Why a series and not the average
//
// `xiDriftOf` answers "how far behind is this squad" with one number. That is the
// right quantity for "is my team bad" and the wrong one for **when to act** — and
// the wildcard decision is a timing decision. A squad three points behind now and
// eight behind in three weeks should wait; eight now and three later should act.
// **Averaging destroys exactly the signal the timing needs.**
//
// The user's framing, which is the clearest statement of it: *"it needs to be
// aware of xpoints at every step of its lookahead, because that affects the
// timing. The points are the signal, when they change is the timing."*
//
// # ⚠️ Horizon 1 is load-bearing, not a tuning choice
//
// `Engine.FixtureLoadInScore()` is true only at `Horizon == 1`. At the shipped
// horizon of 5, `Score` is a five-week average that does NOT carry `FixtureLoad`,
// so a club playing twice scores exactly like a club playing once and a blank
// scores like an ordinary week. A drift measure read there is **blind to the
// fixture run**, which is most of what makes a squad worth rebuilding. Pass a
// horizon-1 engine; `xiDriftOf` reads the averaged view and cannot see this.
//
// The gameweek is isolated by inverting `SetSkipGameweeks` — skip everything
// except `g`, so `TeamFixtures` returns only that week and `Metrics().Score` is
// that week's projection through the ordinary scored path. Same seam
// `BestCaptainXPByGameweek` uses, for the same reason: reassembling rates by hand
// would drift from the scored path the moment either side changed.
//
// The engine is left with an empty skip set on return, so pass one built for this
// rather than one a simulation is scoring against.
func xiDriftSeries(week *analysis.Engine, held, fresh []int, from, n int) []float64 {
	if week == nil || n < 1 {
		return nil
	}
	defer week.SetSkipGameweeks(nil)
	out := make([]float64, 0, n)
	for gw := from; gw < from+n && gw <= 38; gw++ {
		skip := make([]int, 0, 37)
		for other := 1; other <= 38; other++ {
			if other != gw {
				skip = append(skip, other)
			}
		}
		week.SetSkipGameweeks(skip)
		// A gameweek where neither eleven can be priced contributes nothing
		// rather than a spurious zero gap: both sides are read through the same
		// skip set, so a blank week shows up as both elevens scoring their
		// eleventh-best available player, which is the real cost of a blank.
		out = append(out, xiPoints(week, fresh)-xiPoints(week, held))
	}
	return out
}

// sumOf is the total of a drift series — the points the held eleven gives up over
// the whole lookahead by not rebuilding.
//
// ⚠️ A TOTAL, not a mean. The quantity a manager weighs a chip against is "how
// many points does this cost me before I could fix it another way", which
// accumulates; a mean would make a long lookahead look identical to a short one.
func sumOf(xs []float64) float64 {
	var s float64
	for _, x := range xs {
		s += x
	}
	return s
}

// WildcardValueSeries is what playing the wildcard is worth in each week of the
// lookahead, and which of them is the peak.
type WildcardValueSeries struct {
	// Value[k] is the chip's worth if played k weeks from now.
	Value []float64
	// PeakAt is the offset of the best week, 0 meaning now.
	PeakAt int
	// Now is Value[0], the reading a trigger fires on.
	Now float64
}

// wildcardValueOverNext prices the wildcard across a lookahead, composing the
// three things that move its worth.
//
// # The three terms, and which of them is real
//
//  1. **The drift it removes.** Playing at week k means suffering the gap for
//     weeks 0..k-1 and then not, so the chip saves `sum(drift[k:])` — read
//     per-gameweek and fixture-aware, because a squad three behind now and eight
//     behind in three weeks should wait while the reverse should act. See
//     `xiDriftSeries`.
//
//  2. **The hit cost it avoids, which DECLINES.** The alternative to the chip is
//     repairing by transfers, and every week waited accrues one free transfer —
//     so that alternative gets `HitCost` cheaper per week and the chip's edge
//     over it shrinks by the same amount. The user's framing: *"each week waiting
//     is -4 less. So you need to understand where the value peak is."*
//
//  3. ⚠️ **The value of waiting for information, which this CANNOT price.** More
//     weeks mean injuries revealed, rotations observed, bandwagons identified —
//     real value, and the user is right that it is hard to capture. Nothing here
//     models it. `analysis.ChipBarAt` is this project's existing stand-in, and it
//     is a generic option-decay curve fitted against a different reading: **the
//     shape carries over, the calibration does not.** Treat term 3 as absent
//     rather than as handled.
//
// ⚠️ **So the peak this finds is a peak in TERMS 1 AND 2 ONLY.** A rule that
// fires on it is systematically early to the extent term 3 is real, which is
// unknown and probably not small.
func wildcardValueOverNext(drift []float64, changes, free, bankUpTo int) WildcardValueSeries {
	out := WildcardValueSeries{Value: make([]float64, 0, len(drift))}
	best, bestAt := 0.0, 0
	for k := 0; k <= len(drift); k++ {
		// Drift still ahead of a chip played k weeks from now.
		var remaining float64
		for i := k; i < len(drift); i++ {
			remaining += drift[i]
		}
		// The allowance that week, capped the way the bank is: a free transfer
		// accrues weekly and does not accumulate past the limit, so the hit
		// saving stops falling once the cap is reached rather than going
		// negative.
		avail := free + k
		if bankUpTo > 0 && avail > bankUpTo {
			avail = bankUpTo
		}
		v := remaining + repairCostOf(changes, avail)
		out.Value = append(out.Value, v)
		if k == 0 || v > best {
			best, bestAt = v, k
		}
	}
	if len(out.Value) > 0 {
		out.Now = out.Value[0]
	}
	out.PeakAt = bestAt
	return out
}
