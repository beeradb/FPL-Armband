package analysis

import (
	"fmt"
	"math"
	"testing"
)

// fragileXI builds an eleven every member of which blanks with the given
// probability.
//
// It is specified by the blank rate rather than by a start share because blankRate
// no longer reads one: it is one minus the single appearance estimator, in mean
// minutes. See appearance.go. Specifying the eleven by the quantity these tests are
// actually about also makes them independent of which statistic the estimator
// consults, which is what they were always trying to say.
func fragileXI(blank float64) []PlayerMetrics {
	xi := make([]PlayerMetrics, 11)
	for i := range xi {
		xi[i] = metricsWithBlankRate(blank)
		xi[i].Position = "MID"
		xi[i].Score = 4
	}
	xi[0].Position = "GKP"
	return xi
}

// TestSlotProbabilitiesAreATailDistribution pins the arithmetic against a case
// small enough to check by hand.
//
// With ten outfielders each blanking with probability b, the chance none blanks
// is (1-b)^10 and the chance exactly one does is 10b(1-b)^9. Everything the
// slot weights say follows from those, so an error here is invisible
// downstream — the weights would still be ordered and still sum to about four.
func TestSlotProbabilitiesAreATailDistribution(t *testing.T) {
	const b = 0.10
	gk, out := slotProbabilities(fragileXI(b))

	if math.Abs(gk-b) > 1e-12 {
		t.Errorf("reserve keeper priced at %.6f, want %.6f — he covers exactly one "+
			"player, so his slot is that player's blank probability and nothing else",
			gk, b)
	}
	none := math.Pow(1-b, 10)
	one := 10 * b * math.Pow(1-b, 9)
	want := [3]float64{1 - none, 1 - none - one, 0}
	for i := 0; i < 2; i++ {
		if math.Abs(out[i]-want[i]) > 1e-9 {
			t.Errorf("slot %d is %.6f, want %.6f", i+1, out[i], want[i])
		}
	}
	if !(out[0] > out[1] && out[1] > out[2]) {
		t.Errorf("slots are not a decreasing tail: %.4f, %.4f, %.4f",
			out[0], out[1], out[2])
	}
}

// TestBenchIsWorthMoreBehindAFragileEleven is the whole reason for deriving the
// weights instead of fixing them.
//
// A tuple gives the same credit to a bench behind eleven ever-presents and a
// bench behind a side that loses someone most weeks. Those are not the same
// asset, and the optimiser should be willing to pay for depth only in the
// second case. If this ever fails, the derivation has been normalised per-squad
// somewhere and has become an expensive way to reproduce a constant.
func TestBenchIsWorthMoreBehindAFragileEleven(t *testing.T) {
	total := func(ss float64) float64 {
		out, gk := benchSlotWeightsFor(fragileXI(ss))
		return out[0] + out[1] + out[2] + gk
	}
	// Blank rates, not start shares: a sound eleven loses a starter rarely.
	sound, shaky := total(0.03), total(0.20)
	if !(shaky > sound) {
		t.Errorf("fragile eleven gives its bench %.3f and a sound one %.3f; "+
			"depth must be worth more where it is more likely to be needed",
			shaky, sound)
	}
	// And an eleven that never blanks needs no bench at all.
	if got := total(0); got > 1e-9 {
		t.Errorf("an eleven of ever-presents credits its bench %.6f, want 0", got)
	}
}

// TestBenchSlotScaleKeepsBenchWeightMeaningWhatItMeant — the derivation must
// change the shape without also changing the scale.
//
// BenchWeight was swept on the basis that the four slot weights average one, so
// a version whose weights sum to 0.7 would quietly cut the bench's whole
// contribution by six and attribute the result to the new shape. Two changes at
// once is how a measurement stops meaning anything.
func TestBenchSlotScaleKeepsBenchWeightMeaningWhatItMeant(t *testing.T) {
	out, gk := benchSlotWeightsFor(fragileXI(referenceBlankRate))
	if total := out[0] + out[1] + out[2] + gk; math.Abs(total-4) > 1e-9 {
		t.Errorf("a reference eleven's slots sum to %.4f, want 4", total)
	}
}

// TestBlankRateConstantsAreMeasured pins the two numbers behind the derivation
// so a change has to be deliberate. Both come from TestDiagBlankRate.
func TestBlankRateConstantsAreMeasured(t *testing.T) {
	if referenceBlankRate != 0.09 {
		t.Errorf("referenceBlankRate is %v; measured blank rates are 0.066 for the "+
			"0.85-0.95 start-share band and 0.122 for 0.75-0.85", referenceBlankRate)
	}
	// A player on the pitch for every minute cannot blank, and one who has never
	// played must not be credited with any chance of appearing.
	if got := blankRate(PlayerMetrics{ExpectedMinutes: 90}); got != 0 {
		t.Errorf("an ever-present blanks with probability %v", got)
	}
	if got := blankRate(PlayerMetrics{ExpectedMinutes: 0}); got != 1 {
		t.Errorf("a player with no minutes blanks with probability %v, want exactly 1 — "+
			"this is the other side of the property research_targets depends on", got)
	}
	for _, mm := range []float64{0, 5, 30, 60, 90} {
		if got := blankRate(PlayerMetrics{ExpectedMinutes: mm}); got < 0 || got > 1 {
			t.Errorf("blank rate %v at %.0f mean minutes is not a probability", got, mm)
		}
	}
}

// SetBenchSlots and FPL_BENCH_SLOTS must renormalise identically.
//
// They are the two ways a sweep can pin the tuple, and this package's recorded
// signature failure is one quantity with two implementations — four instances,
// including a bench constant. They now share normaliseBenchSlots; this fails if
// either grows its own arithmetic.
func TestTheTwoBenchSlotPathsAgree(t *testing.T) {
	restoreOut, restoreGK := benchOutfieldWeights, benchGKWeight
	defer func() { benchOutfieldWeights, benchGKWeight = restoreOut, restoreGK }()

	for _, raw := range []struct {
		env string
		out [3]float64
		gk  float64
	}{
		{"1,1,1,1", [3]float64{1, 1, 1}, 1},
		{"2.4,1.0,0.4,0.2", [3]float64{2.4, 1.0, 0.4}, 0.2},
		{"1.9,1.2,0.6,0.3", [3]float64{1.9, 1.2, 0.6}, 0.3},
		// Deliberately not summing to four, which is the case the
		// renormalisation exists for.
		{"5,3,2,1", [3]float64{5, 3, 2}, 1},
	} {
		t.Setenv("FPL_BENCH_SLOTS", raw.env)
		wantOut, wantGK := benchSlotWeights()

		SetBenchSlots(raw.out, raw.gk)
		if benchOutfieldWeights != wantOut || benchGKWeight != wantGK {
			t.Errorf("%s: SetBenchSlots gave %v/%v, env path gave %v/%v",
				raw.env, benchOutfieldWeights, benchGKWeight, wantOut, wantGK)
		}
		if sum := wantOut[0] + wantOut[1] + wantOut[2] + wantGK; sum < 4-1e-9 || sum > 4+1e-9 {
			t.Errorf("%s: renormalised tuple sums to %v, want 4", raw.env, sum)
		}
	}
}

// The fixed tuple is read when derived slots are off, and not when they are on.
//
// ⚠️ The first version called benchSlotWeightsFor directly, which cannot read the
// package tuple *by construction* — so it passed for reasons unrelated to the
// guard it claimed to pin. Code review proved it by mutation: replacing
// squad.go's `if derivedBenchSlots` with `if false && derivedBenchSlots`, which
// makes the fixed tuple authoritative always, left it green.
//
// It now goes through benchValue, which is where the branch actually lives, and
// asserts both halves. The positive control is the half that was missing:
// without it, making SetBenchSlots an outright no-op also passed.
func TestTheFixedTupleIsReadOnlyWhenSlotsAreNotDerived(t *testing.T) {
	restoreOut, restoreGK, restoreDerived := BenchSlotState()
	defer func() {
		SetDerivedBenchSlots(restoreDerived)
		SetBenchSlots(restoreOut, restoreGK)
	}()

	xi := make([]PlayerMetrics, 11)
	for i := range xi {
		xi[i] = metricsWithBlankRate(referenceBlankRate)
		xi[i].Score = 5
	}
	xi[0].Position = "GKP"

	bench := []PlayerMetrics{
		{Position: "GKP", Score: 4},
		{Position: "DEF", Score: 3},
		{Position: "MID", Score: 2},
		{Position: "FWD", Score: 1},
	}
	value := func() float64 {
		var sc xiScratch
		return sc.benchValue(xi, bench, 0.10, false)
	}

	// The negative half: with derived slots on, the tuple must not be consulted.
	SetDerivedBenchSlots(true)
	SetBenchSlots([3]float64{2.4, 1.0, 0.4}, 0.2)
	before := value()
	SetBenchSlots([3]float64{1, 1, 1}, 1)
	if after := value(); before != after {
		t.Errorf("derived slots consulted the fixed tuple: %v then %v", before, after)
	}

	// The positive control: with derived slots off, the tuple must reach the
	// value. Without this the test passes when SetBenchSlots does nothing.
	SetDerivedBenchSlots(false)
	SetBenchSlots([3]float64{2.4, 1.0, 0.4}, 0.2)
	shaped := value()
	SetBenchSlots([3]float64{1, 1, 1}, 1)
	if flat := value(); shaped == flat {
		t.Fatalf("the fixed tuple never reached benchValue: shaped and flat both %v", shaped)
	}
}

// A refused tuple installs the shipped default, not the previous arm's.
//
// The failure pinned here is a sweep arm of {0,0,0},0 — the obvious zero control
// for a bench shape sweep — silently replaying whatever the preceding arm
// installed and reporting a byte-identical null. Both entry points must refuse
// the same inputs: sharing the arithmetic but not the validation is the same
// defect as two implementations, and it is how this pair shipped first.
func TestARefusedBenchTupleFallsBackToTheDefault(t *testing.T) {
	restoreOut, restoreGK, restoreDerived := BenchSlotState()
	defer func() {
		SetDerivedBenchSlots(restoreDerived)
		SetBenchSlots(restoreOut, restoreGK)
	}()

	for _, bad := range []struct {
		out [3]float64
		gk  float64
		why string
	}{
		{[3]float64{0, 0, 0}, 0, "all zero"},
		{[3]float64{-1, 5, 0}, 0, "a negative component"},
		{[3]float64{math.NaN(), 1, 1}, 1, "NaN"},
		{[3]float64{math.Inf(1), 1, 1}, 1, "Inf"},
	} {
		// Install a distinctive tuple first, so a no-op is distinguishable from
		// a correct fallback.
		SetBenchSlots([3]float64{1.9, 1.2, 0.6}, 0.3)
		SetBenchSlots(bad.out, bad.gk)

		gotOut, gotGK, _ := BenchSlotState()
		if gotOut != defaultBenchOutfield || gotGK != defaultBenchGK {
			t.Errorf("%s: setter left %v/%v, want the shipped default %v/%v",
				bad.why, gotOut, gotGK, defaultBenchOutfield, defaultBenchGK)
		}

		t.Setenv("FPL_BENCH_SLOTS", fmt.Sprintf("%g,%g,%g,%g",
			bad.out[0], bad.out[1], bad.out[2], bad.gk))
		envOut, envGK := benchSlotWeights()
		if envOut != defaultBenchOutfield || envGK != defaultBenchGK {
			t.Errorf("%s: env path gave %v/%v, want the shipped default",
				bad.why, envOut, envGK)
		}
	}
}
