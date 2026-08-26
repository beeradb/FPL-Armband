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
