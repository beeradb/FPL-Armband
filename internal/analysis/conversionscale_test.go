package analysis

import (
	"math"
	"testing"
)

// TestXPointsRequiresACalibration pins the zero-value trap shut.
//
// The scale is a second argument precisely so omitting it cannot compile, but a
// caller can still pass a zero-valued struct explicitly — and a zero scale prices
// xG and xA at NOTHING, which is not a small error. It is the "zero both attacking
// channels" arm, in the direction this repair argues for, produced silently and
// looking entirely plausible. That is this package's signature failure class.
//
// # The predicate is the underlying, NOT the realised counts
//
// Rows carrying goals with no xG exist in quantity — every pre-2022-23 season
// before the repair, and this package's own week fixtures build rows with no
// xG, xA or xGC at all. On those the scale cannot change the arithmetic, so
// demanding one would convert a no-op into a crash. The gate is `XG > 0 || XA > 0`
// and this test pins both halves of it.
func TestXPointsRequiresACalibration(t *testing.T) {
	var zero ConversionScale

	// A row with underlying to price and no scale to price it with: refuse.
	withXG := XPointsGW{
		Position: 4, Fixtures: 1, Minutes: 90, Points: 4, Goals: 1, XG: 0.5,
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("XPointsResidual accepted a zero conversion scale on a row " +
					"carrying xG. A zero scale prices the underlying at nothing, " +
					"which is the ceiling arm produced silently — it must panic")
			}
		}()
		_ = XPointsResidual(withXG, zero, modernRules)
	}()

	// The xA half of the predicate, so a gate written as `XG > 0` alone fails here.
	withXA := XPointsGW{
		Position: 3, Fixtures: 1, Minutes: 90, Points: 3, Assists: 1, XA: 0.4,
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("XPointsResidual accepted a zero scale on a row carrying xA " +
					"but no xG; the guard is reading XG alone")
			}
		}()
		_ = XPointsResidual(withXA, zero, modernRules)
	}()

	// And the other half: no underlying, so no scale is needed and none is demanded.
	// A guard keyed on Goals or Assists rather than on XG or XA would crash here.
	noUnderlying := XPointsGW{
		Position: 4, Fixtures: 1, Minutes: 90, Points: 10, Goals: 2, Assists: 1,
	}
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("XPointsResidual panicked on a row with goals and assists "+
					"but no xG or xA (%v). Those rows are most of the pre-repair "+
					"archive and the scale cannot change their arithmetic; the "+
					"guard must key on the UNDERLYING, not the realised counts", r)
			}
		}()
		// Two goals off no xG at 4 a goal, one assist off no xA at 3: 11, and the
		// scale is irrelevant to every term of it.
		if got := XPointsResidual(noUnderlying, zero, modernRules); math.Abs(got-11) > 1e-12 {
			t.Errorf("residual %v on a no-underlying row, want 11", got)
		}
	}()
}

// TestTheConversionScaleIsAppliedPerPosition is the mutation proof.
//
// The property that matters is not that *a* scale is applied — an implementation
// that multiplied by a single league-wide constant would satisfy that, and would
// reintroduce the cross-position bias this change exists to remove. What must hold
// is that the scale is keyed by the row's position and reaches both channels.
//
// So each assertion below is a contrast that a wrongly-keyed or partly-wired
// implementation fails: the same underlying, priced through two different scales,
// must give two different answers, and the difference must be exactly the point
// value times the change in expectation.
func TestTheConversionScaleIsAppliedPerPosition(t *testing.T) {
	// A forward: 1 goal off 1.0 xG, 1 assist off 1.0 xA. At a neutral scale both
	// channels are exactly level, which makes every deviation below attributable.
	fwd := XPointsGW{
		Position: 4, Fixtures: 1, Minutes: 90, Points: 8,
		Goals: 1, Assists: 1, XG: 1.0, XA: 1.0,
	}
	if got := XPointsResidual(fwd, ConversionScale{Goals: 1, Assists: 1}, modernRules); math.Abs(got) > 1e-12 {
		t.Fatalf("neutral scale gave residual %v, want 0; the fixture is not the "+
			"one the rest of this test reasons about", got)
	}

	// The goals channel. A forward's goal is 4 points, so doubling his expected
	// goals must move the residual by exactly -4.
	got := XPointsResidual(fwd, ConversionScale{Goals: 2, Assists: 1}, modernRules)
	if math.Abs(got+4) > 1e-12 {
		t.Errorf("with Goals scale 2 the residual is %v, want -4. Either the scale "+
			"is not reaching the goals channel or it is not multiplying xG", got)
	}

	// The assists channel, independently — a scale wired into goals only would pass
	// the line above and fail here. An assist is 3 points at every position.
	got = XPointsResidual(fwd, ConversionScale{Goals: 1, Assists: 2}, modernRules)
	if math.Abs(got+3) > 1e-12 {
		t.Errorf("with Assists scale 2 the residual is %v, want -3; the scale is "+
			"not reaching the assists channel", got)
	}

	// # Keyed by position
	//
	// The same underlying and the same scale, priced as a defender and as a
	// forward. A goal is 6 to a defender and 4 to a forward, so a scale change of
	// +1 on expected goals must move the two residuals by different amounts. An
	// implementation applying one league-wide constant, or reading the wrong
	// position's entry, returns the same delta for both.
	def := fwd
	def.Position = 2
	scaled := ConversionScale{Goals: 2, Assists: 1}
	neutral := ConversionScale{Goals: 1, Assists: 1}
	dFwd := XPointsResidual(fwd, scaled, modernRules) - XPointsResidual(fwd, neutral, modernRules)
	dDef := XPointsResidual(def, scaled, modernRules) - XPointsResidual(def, neutral, modernRules)
	if math.Abs(dFwd+4) > 1e-12 || math.Abs(dDef+6) > 1e-12 {
		t.Errorf("a unit change in the goals scale moved the forward by %v (want -4) "+
			"and the defender by %v (want -6); the scale is not being priced "+
			"through the position's own goal value", dFwd, dDef)
	}
	if math.Abs(dFwd-dDef) < 1e-12 {
		t.Error("the same scale change moved a forward and a defender identically; " +
			"the position key is not reaching the arithmetic")
	}

	// The clean sheet and the concede deduction must be UNTOUCHED by the scale —
	// they are the larger half of the cross-position bias and a separate open line,
	// and a change reaching them would be an unmeasured second change.
	keeper := XPointsGW{
		Position: 1, Fixtures: 1, Minutes: 90, Points: 6,
		CleanSheets: 1, GoalsConceded: 0, XGC: 0.8,
	}
	a := XPointsResidual(keeper, neutral, modernRules)
	b := XPointsResidual(keeper, ConversionScale{Goals: 3, Assists: 3}, modernRules)
	if math.Abs(a-b) > 1e-12 {
		t.Errorf("the conversion scale moved a keeper's clean-sheet and concede "+
			"residual from %v to %v; it must reach the attacking channels only", a, b)
	}
}
