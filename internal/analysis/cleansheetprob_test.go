package analysis

import (
	"math"
	"math/rand"
	"testing"
)

// TestCleanSheetProbIsBitIdenticalToTheExpressionItReplaced is the invariant the
// collapse of four call sites onto cleanSheetProb owes.
//
// The clean-sheet exponent was written out four times — baseXP90,
// cleanSheetSensitiveAt, fixtureSensitiveAt and CleanSheetTermFor. Two of those
// have already drifted apart once in this package's history: fixtureSensitiveAt
// carried exp(-XGC90) while baseXP90 carried the full expression, which was
// harmless only while the extra factors were 1 and stopped being harmless when
// DefConCleanCoupling shipped at 0.3.
//
// So the claim this test pins is narrow and total: **at the shipped defaults the
// helper returns exactly what the expression returned, bit for bit** — not
// "within epsilon", because a refactor that changes the last bit of a
// probability changes a squad hash somewhere and would read downstream as
// football.
//
// A differential test over thousands of inputs is deliberately preferred to a
// reviewer reading the diff, per the review gate's own first rule: an invariant
// is free, runs every time, and cannot be talked out of a disagreement.
func TestCleanSheetProbIsBitIdenticalToTheExpressionItReplaced(t *testing.T) {
	priorFactor, priorScale := CleanSheetState()
	defer func() {
		SetCleanSheetXGCFactor(priorFactor)
		SetCleanSheetScale(priorScale)
	}()
	SetCleanSheetXGCFactor(1.0)
	SetCleanSheetScale(1.0)

	// ⚠️ **This half is weak on its own and is kept only as the floor.** It
	// rebuilds `want` from the same variable names the function uses, so it
	// cannot see an argument TRANSPOSITION at a call site — and transposition is
	// not bit-neutral: (x*def)*cf differs from (x*cf)*def in about 35% of random
	// draws. TestEachCleanSheetCallSiteMatchesTheExpressionItReplaced is the test
	// that carries the claim, because it exercises the call sites.
	r := rand.New(rand.NewSource(20260815))
	for i := 0; i < 20000; i++ {
		// Ranges chosen to cover the real population and then some. XGC90 runs
		// about 0.7 to 2.3 across clubs; def is the fixture defensive
		// multiplier, roughly 0.70 to 1.40; cf is defconCleanFactor, which the
		// shipped coupling holds near 1.
		xgc := r.Float64() * 4.0
		def := 0.5 + r.Float64()*1.2
		cf := 0.5 + r.Float64()*1.0

		want := math.Exp(-cleanSheetXGCFactor * xgc * def * cf)
		got := cleanSheetProb(xgc, def, cf)
		if got != want {
			t.Fatalf("cleanSheetProb(%v, %v, %v) = %v, want %v (bit-exact) — the "+
				"collapse changed behaviour at the shipped defaults", xgc, def, cf, got, want)
		}
	}
}

// TestEachCleanSheetCallSiteMatchesTheExpressionItReplaced is the invariant that
// actually carries the collapse, and it exists because the test above does not.
//
// The test above rebuilds its expectation from the same variable names the
// function uses, so it is blind to the one mistake the collapse could plausibly
// introduce: **transposing two of the three factors at a call site**. That is not
// bit-neutral — (x*def)*cf and (x*cf)*def differ in roughly a third of random
// draws — and no existing test would catch it either, because every neutral-path
// check runs at def = 1 where the transposition is exact and the per-fixture
// comparison uses a 1e-9 tolerance. A transposition would therefore change squad
// hashes with a green suite.
//
// So this pins each SITE against a hand-written copy of the expression as it
// stood before the collapse, at a def deliberately away from 1.
func TestEachCleanSheetCallSiteMatchesTheExpressionItReplaced(t *testing.T) {
	priorFactor, priorScale := CleanSheetState()
	defer func() {
		SetCleanSheetXGCFactor(priorFactor)
		SetCleanSheetScale(priorScale)
	}()
	SetCleanSheetXGCFactor(1.0)
	SetCleanSheetScale(1.0)

	// ⚠️ **`&Engine{}` is not an engine.** Its `rules` are the zero value, so
	// every points map is nil — and a nil map read as a bare index returns 0 with
	// no error, which silently deleted the whole clean-sheet channel and made this
	// test compare 0 against 0.47. That is the bare-map-index defect one layer
	// over, inside the diagnostic for it. `ScoringRulesFor("")` is the live game's
	// table, which is what this test's `cleanSheetPoints[pos]` reads anyway.
	e := &Engine{rules: ScoringRulesFor("")}
	// DEF, because the clean sheet is worth 4 there and 0 to a forward, and
	// because defconCleanFactor is only non-neutral for a defender with a
	// defensive-contribution rate.
	const pos = 2
	csPts := cleanSheetPoints[pos]
	if csPts <= 0 {
		t.Fatalf("position %d has no clean-sheet points; this test is scoped to one that does", pos)
	}

	r := rand.New(rand.NewSource(20260816))
	for i := 0; i < 20000; i++ {
		m := PlayerMetrics{
			XGC90:    0.05 + r.Float64()*3.0,
			DefCon90: r.Float64() * 12.0,
		}
		// Away from 1 on purpose: at def = 1 a transposition is invisible.
		def := 0.60 + r.Float64()*0.85
		cf := e.defconCleanFactor(pos, m.DefCon90)

		// The expression exactly as the four sites carried it before the
		// collapse. Written out rather than shared, because a diagnostic must
		// never carry its own copy of the thing it is checking — here that rule
		// inverts: the copy IS the check, and it must not be refactored to call
		// cleanSheetProb.
		wantSensitive := math.Exp(-cleanSheetXGCFactor*m.XGC90*def*cf) * csPts
		if got := e.cleanSheetSensitiveAt(m, pos, def); got != wantSensitive {
			t.Fatalf("cleanSheetSensitiveAt: got %v want %v (xgc %v def %v cf %v)",
				got, wantSensitive, m.XGC90, def, cf)
		}

		// baseXP90's site is the neutral one: def is implicitly 1 there, and
		// that implicitness is exactly what the collapse had to preserve.
		wantNeutral := math.Exp(-cleanSheetXGCFactor*m.XGC90*cf) * csPts
		if got := e.cleanSheetSensitiveAt(m, pos, 1); got != wantNeutral {
			t.Fatalf("cleanSheetSensitiveAt at def=1: got %v want %v", got, wantNeutral)
		}

		// CleanSheetTermFor takes neither factor, so both are 1.
		wantTerm := math.Exp(-cleanSheetXGCFactor*m.XGC90) * csPts
		if got := CleanSheetTermFor(pos, m.XGC90); got != wantTerm {
			t.Fatalf("CleanSheetTermFor: got %v want %v", got, wantTerm)
		}
	}

	// And the transposition this test exists for really is detectable — if it
	// were not, the test would be passing vacuously.
	var transposed int
	for i := 0; i < 20000; i++ {
		x := 0.05 + r.Float64()*3.0
		def := 0.60 + r.Float64()*0.85
		cf := 0.70 + r.Float64()*0.6
		if math.Exp(-x*def*cf) != math.Exp(-x*cf*def) {
			transposed++
		}
	}
	if transposed == 0 {
		t.Error("no draw distinguished (x*def)*cf from (x*cf)*def, so this test " +
			"cannot detect a transposed call site and is passing vacuously")
	}
}

// TestBothCleanSheetKnobsReachTheProbability is the liveness half, and it is the
// check a sweep arm cannot make for itself.
//
// A knob that does not arrive returns a byte-identical null, which in a cells
// file is indistinguishable from a knob that does nothing — this record's
// signature failure. A sweep buys a canary arm to detect it at a cost of 36
// cells; this costs microseconds and is strictly more decisive, because it names
// the consumer rather than inferring it from a movement.
func TestBothCleanSheetKnobsReachTheProbability(t *testing.T) {
	priorFactor, priorScale := CleanSheetState()
	defer func() {
		SetCleanSheetXGCFactor(priorFactor)
		SetCleanSheetScale(priorScale)
	}()

	SetCleanSheetXGCFactor(1.0)
	SetCleanSheetScale(1.0)
	base := cleanSheetProb(1.35, 1, 1)

	SetCleanSheetXGCFactor(1.5)
	if moved := cleanSheetProb(1.35, 1, 1); moved == base {
		t.Error("SetCleanSheetXGCFactor did not reach cleanSheetProb: a knob that " +
			"does not arrive is indistinguishable from one that does nothing")
	}

	SetCleanSheetXGCFactor(1.0)
	SetCleanSheetScale(0.5)
	got := cleanSheetProb(1.35, 1, 1)
	if want := 0.5 * base; got != want {
		t.Errorf("SetCleanSheetScale = 0.5 gave %v, want %v — the scale must be a "+
			"plain multiplier on the probability, since that is the parameter the "+
			"calibration's intercept fits", got, want)
	}

	// The state accessor is what every sweep restores from, so a round-trip
	// failure would silently leave one arm's value installed for every later
	// diagnostic in the process.
	SetCleanSheetXGCFactor(1.234)
	SetCleanSheetScale(0.876)
	if f, s := CleanSheetState(); f != 1.234 || s != 0.876 {
		t.Errorf("CleanSheetState round-trip gave (%v, %v), want (1.234, 0.876)", f, s)
	}
}
