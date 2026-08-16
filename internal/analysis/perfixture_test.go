package analysis

import (
	"math"
	"testing"

	"armband/internal/fpl"
)

// The fixture multipliers are applied to each fixture and the results averaged,
// rather than the multipliers being averaged and the estimate evaluated once.
//
// The distinction only exists because the clean sheet is exp(-lambda x def),
// which is *convex* in the defensive multiplier — the average of a convex
// function's values is at least its value at the average (Jensen's inequality).
// Goals and assists are linear in the attacking multiplier, so for them the two
// orderings agree exactly.
//
// The tests below pin both halves of that: no change at all for a player whose
// fixture-sensitive terms are all linear, and a specific, signed change for one
// whose are not.

// scoredFixtures is a run of fixtures at the given FPL difficulties. OpponentID
// is left at zero so the band adjustment is neutral (bands ship disabled) and the
// difficulty is the only thing varying.
func scoredFixtures(diffs ...int) []FixtureBrief {
	fx := make([]FixtureBrief, 0, len(diffs))
	for i, d := range diffs {
		fx = append(fx, FixtureBrief{Event: i + 1, Difficulty: d})
	}
	return fx
}

// averagedFixtureSensitive is the old implementation: average the multipliers
// over the run, then evaluate the estimate once at those averages. Kept here as
// the thing the new code must agree with for linear terms and beat for convex
// ones.
func averagedFixtureSensitive(e *Engine, m PlayerMetrics, pos int, fx []FixtureBrief) float64 {
	var atk, def float64
	for _, f := range fx {
		a, d := e.fixtureMultipliersFor(f)
		atk += a
		def += d
	}
	n := float64(len(fx))
	return e.fixtureSensitiveAt(m, pos, atk/n, def/n)
}

// perFixtureSensitive is what ships: score each fixture, then average.
func perFixtureSensitive(e *Engine, m PlayerMetrics, pos int, fx []FixtureBrief) float64 {
	var sum float64
	for _, f := range fx {
		a, d := e.fixtureMultipliersFor(f)
		sum += e.fixtureSensitiveAt(m, pos, a, d)
	}
	return sum / float64(len(fx))
}

// TestPerFixtureLeavesAttackersUntouched is the invariance check, and it is the
// sharp one. A forward has no clean sheet, no goals-conceded deduction and no
// saves, so every fixture-sensitive term he has is a rate multiplied by the
// attacking multiplier. Averaging the multiplier and averaging the results are
// then the same arithmetic, and his estimate must not move.
//
// The tolerance is 1e-9 rather than exact equality because the two orderings are
// identical in real arithmetic and differ in the last bits of floating point:
// `A + c x mean(a_i)` and `mean(A + c x a_i)` sum and divide in a different
// order. Anything larger than rounding means a term that is not linear has been
// swept into the attacking side.
func TestPerFixtureLeavesAttackersUntouched(t *testing.T) {
	e := testEngine(t)
	var fwd *fpl.Element
	for i := range e.Boot.Elements {
		el := &e.Boot.Elements[i]
		if el.ElementType == 4 && el.Minutes > 900 {
			fwd = el
			break
		}
	}
	if fwd == nil {
		t.Skip("no established forward in the pool")
	}
	m := e.Metrics(fwd)
	if m.XG90 <= 0 {
		t.Skip("no attacking rate to move")
	}
	// A deliberately lopsided run, which is where the two forms disagree most for
	// anyone they disagree for at all.
	for _, fx := range [][]FixtureBrief{
		scoredFixtures(1, 5, 1, 5, 3),
		scoredFixtures(1, 1, 1, 1, 5),
		scoredFixtures(2, 4),
	} {
		old := averagedFixtureSensitive(e, m, 4, fx)
		now := perFixtureSensitive(e, m, 4, fx)
		if math.Abs(now-old) > 1e-9 {
			t.Errorf("a forward's fixture-sensitive estimate moved from %.12f to %.12f "+
				"on %d fixtures; every term he has is linear in the multiplier, so it "+
				"must not move at all", old, now, len(fx))
		}
	}
}

// TestPerFixtureRaisesTheCleanSheetOnAMixedRun pins the direction and rough size
// of what the refactor actually changes. Jensen's inequality says the per-fixture
// mean is >= the value at the averaged multiplier for a convex term, so a
// defender's estimate must rise, strictly on a mixed run and not at all on a flat
// one.
func TestPerFixtureRaisesTheCleanSheetOnAMixedRun(t *testing.T) {
	e := testEngine(t)
	var def *fpl.Element
	for i := range e.Boot.Elements {
		el := &e.Boot.Elements[i]
		m := e.Metrics(el)
		if el.ElementType == 2 && el.Minutes > 900 && m.XGC90 > 0 {
			def = el
			break
		}
	}
	if def == nil {
		t.Skip("no established defender with a goals-conceded rate")
	}
	m := e.Metrics(def)

	mixed := scoredFixtures(1, 5, 1, 5, 3)
	old := averagedFixtureSensitive(e, m, 2, mixed)
	now := perFixtureSensitive(e, m, 2, mixed)
	if !(now > old) {
		t.Errorf("on a run split between difficulty 1 and 5 the defender scores %.4f "+
			"per fixture against %.4f at the averaged multiplier; the clean sheet is "+
			"convex so the per-fixture figure must be the larger", now, old)
	}
	// The clean sheet is worth 4 points, so no rearrangement of one term inside it
	// can be worth more than that. A larger move means something else changed.
	if now-old > 4 {
		t.Errorf("the two forms differ by %.4f points per 90, which is more than the "+
			"clean sheet is worth", now-old)
	}

	// A flat run has nothing to disagree about: every fixture carries the same
	// multiplier, which is Jensen's equality case.
	flat := scoredFixtures(3, 3, 3, 3, 3)
	if d := math.Abs(perFixtureSensitive(e, m, 2, flat) - averagedFixtureSensitive(e, m, 2, flat)); d > 1e-9 {
		t.Errorf("a run of identical fixtures differs by %.12f between the two forms", d)
	}
}

// TestThresholdAndAdjustedUseTheSameFixtureRule — thresholdXP90 is subtracted
// from FixtureAdjXP90 to separate the terms FPL pays as a step from the ones it
// pays as a rate. That subtraction only cancels if both apply the fixture
// multipliers the same way, so a change to one has to be made to the other. It is
// checked by rebuilding what the subtraction should leave: the rate part must
// carry no clean-sheet content at all, which is verifiable by zeroing the
// clean-sheet points for the position.
func TestThresholdAndAdjustedUseTheSameFixtureRule(t *testing.T) {
	if !thresholdSplit {
		t.Skip("FPL_NO_THRESHOLD_SPLIT is set")
	}
	e := testEngine(t)
	var el *fpl.Element
	for i := range e.Boot.Elements {
		c := &e.Boot.Elements[i]
		m := e.Metrics(c)
		if c.ElementType == 2 && c.Minutes > 900 && m.XGC90 > 0 {
			el = c
			break
		}
	}
	if el == nil {
		t.Skip("no established defender with a goals-conceded rate")
	}
	m := e.Metrics(el)
	fx := scoredFixtures(1, 5, 1, 5, 3)

	// Rebuild both sides on the same fixture list, since the engine's own horizon
	// is whatever the real calendar happens to offer.
	adj := e.fixtureAdjustedXP90(el, m, fx)
	thr := e.thresholdXP90(el, m, fx)

	// What the clean sheet contributes to each, obtained by taking the rate away:
	// the difference between the two must be the appearance points plus nothing,
	// because the rate part is what is left after the threshold pair is removed.
	rate := adj - thr
	if rate < 0 {
		t.Fatalf("the rate part came out negative (%.4f) — the threshold pair is "+
			"larger than the whole estimate, which means the two disagree about "+
			"fixtures", rate)
	}
	// The strong form: recompute the threshold pair through the adjusted path by
	// zeroing every non-threshold rate, and require the two to agree.
	bare := m
	bare.XG90, bare.XA90, bare.Bonus90, bare.Saves90 = 0, 0, 0, 0
	bare.Yellow90, bare.Red90, bare.DefCon90 = 0, 0, 0
	bare.BaseXP90 = e.baseXP90(el, bare)
	bare.SetPieceXP90 = 0
	// baseXP90 keeps the goals-conceded deduction, which is a rate term, so add
	// it back at the fixture-adjusted level the threshold path does not see.
	viaAdjusted := e.fixtureAdjustedXP90(el, bare, fx)
	var conceded float64
	if blk := concedeBlock[2]; blk > 0 && bare.XGC90 > 0 {
		w := clamp(e.Weights.FixtureWeight, 0, 1)
		flat := poissonFloorDiv(blk, bare.XGC90)
		var adjc float64
		for _, f := range fx {
			_, d := e.fixtureMultipliersFor(f)
			adjc += poissonFloorDiv(blk, bare.XGC90*d)
		}
		conceded = flat*(1-w) + adjc/float64(len(fx))*w
	}
	if got, want := viaAdjusted+conceded, e.thresholdXP90(el, bare, fx); math.Abs(got-want) > 1e-9 {
		t.Errorf("the adjusted path puts the threshold pair at %.6f and thresholdXP90 "+
			"at %.6f; the two must apply the fixture multipliers identically or the "+
			"subtraction in Score does not cancel", got, want)
	}
}
