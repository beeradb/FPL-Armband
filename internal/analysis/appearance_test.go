package analysis

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"armband/internal/fpl"
)

// TestPlaysAtAllIsZeroWithNoMinutes is the property `research_targets` is built on.
//
// A player with no Premier League minutes must score exactly 0.00. The appearance
// term is the one place that is easy to break, because a logistic in mean minutes
// reads 0.186 at the bottom band where the truth is 0.095 — it would quietly pay a
// footballer who has never kicked a ball. The identity form used here is exactly
// zero at zero by construction rather than by a bound bolted on top, and this pins
// that.
func TestPlaysAtAllIsZeroWithNoMinutes(t *testing.T) {
	for _, m := range []float64{0, -1, -90} {
		if got := playsAtAll(m); got != 0 {
			t.Errorf("playsAtAll(%v) = %v, want exactly 0 — a player with no minutes "+
				"must score 0.00, which research_targets depends on", m, got)
		}
	}
	if got := appearanceFactor(0); got != 0 {
		t.Errorf("appearanceFactor(0) = %v, want exactly 0", got)
	}
}

// TestPlaysAtAllNeverBelowPlaysSixty pins an ordering that is true by definition:
// reaching sixty minutes implies appearing at all.
//
// It is worth a test because the two are INDEPENDENT fits — a logistic for the
// sixty-minute probability, an identity for the appearance probability — and
// nothing about fitting them separately guarantees the ordering. They cross by up
// to 0.002 in a narrow band around 79 mean minutes, so this is a real crossing that
// playsAtAll corrects rather than a hypothetical one.
func TestPlaysAtAllNeverBelowPlaysSixty(t *testing.T) {
	for m := 0.0; m <= 90.0; m += 0.25 {
		atAll, sixty := playsAtAll(m), playsSixty(m)
		if atAll < sixty-1e-12 {
			t.Fatalf("at %.2f mean minutes P(appears) = %.6f is below P(60+) = %.6f; "+
				"reaching the hour implies appearing", m, atAll, sixty)
		}
	}
}

// TestPlaysAtAllMatchesTheMeasuredBands checks the fit against the measurement it
// came from: 2,217 player-seasons, four seasons, single-fixture gameweeks only.
//
// The tolerance is deliberately loose, and the reason is not that the fit is bad.
// Its rms is 0.1112 where the sixty-minute curve manages 0.045 — a comparison that
// is **retracted** as evidence the fit is poor, because it measured the two curves
// against different floors. The target is a binomial proportion over ~36 gameweeks,
// so the sampling floor is 0.065 for the appearance rate against 0.062 for the
// sixty-minute one, and most of the apparent misfit is noise in the target rather
// than error in the curve. The start-aware replacement that comparison motivated was
// built and measured; it buys 0.4%. See appearance.go and TestDiagStartShare.
//
// So the point of the test is that the curve tracks the measured bands, and the
// tolerance is loose because a band of a few hundred player-seasons is itself noisy.
func TestPlaysAtAllMatchesTheMeasuredBands(t *testing.T) {
	cases := []struct{ meanMinutes, measured, tol float64 }{
		{2.5, 0.095, 0.06}, // fit under-credits here, which is the safe direction
		{20.0, 0.446, 0.05},
		{50.0, 0.746, 0.05},
		{88.0, 0.988, 0.06},
	}
	for _, c := range cases {
		got := playsAtAll(c.meanMinutes)
		if math.Abs(got-c.measured) > c.tol {
			t.Errorf("playsAtAll(%.1f) = %.3f, measured %.3f, tolerance %.2f",
				c.meanMinutes, got, c.measured, c.tol)
		}
	}
}

// TestAppearanceFactorPaysTheShortPlayPoint is the arithmetic of the fix.
//
// FPL pays 1 for turning up and 2 at the hour, so the expectation is
// P(appears) + P(60+). The per-90 term carries the full 2, so the factor is that
// expectation halved. The old behaviour was P(60+) alone, which pays a fifty-minute
// appearance nothing.
func TestAppearanceFactorPaysTheShortPlayPoint(t *testing.T) {
	for _, m := range []float64{5, 15, 30, 45, 60, 75, 90} {
		want := (playsAtAll(m) + playsSixty(m)) / appearancePoints
		if got := appearanceFactor(m); math.Abs(got-want) > 1e-12 {
			t.Fatalf("appearanceFactor(%.0f) = %.9f, want %.9f", m, got, want)
		}
		// And it must exceed the old single-branch value everywhere a player can
		// appear without reaching the hour, which is the whole point.
		if m > 0 && appearanceFactor(m) <= playsSixty(m)-1e-12 {
			t.Errorf("at %.0f mean minutes the corrected factor %.4f is not above the "+
				"old P(60+) of %.4f", m, appearanceFactor(m), playsSixty(m))
		}
	}
}

// TestShortPlayCreditOffReproducesTheOldBehaviour pins the escape hatch.
//
// FPL_NO_SHORT_PLAY=1 must reproduce the single-branch behaviour exactly, not
// approximately — otherwise the flag cannot be used to re-measure the change, which
// is the only reason it exists.
func TestShortPlayCreditOffReproducesTheOldBehaviour(t *testing.T) {
	orig := shortPlayCredit
	shortPlayCredit = false
	defer func() { shortPlayCredit = orig }()

	for _, m := range []float64{0, 5, 30, 60, 90} {
		if got, want := appearanceFactor(m), playsSixty(m); got != want {
			t.Errorf("with the credit off, appearanceFactor(%.0f) = %v, want exactly "+
				"playsSixty = %v", m, got, want)
		}
	}
}

// TestThresholdPartsSumToTheWhole is the invariant that keeps the rest of the model
// correct, and it is easy to lose.
//
// `fixtureAdjustedXP90` subtracts the threshold total from `baseXP90` to leave the
// rate remainder, and TestThresholdAndAdjustedUseTheSameFixtureRule pins that both
// use the same fixture rule so the subtraction cancels. Splitting the threshold into
// two halves must therefore be **exactly** additive: a split that were merely close
// would break the remainder silently, and the remainder is what every rate term is
// scaled by.
//
// Note this must hold at a real fixture list, not only at the neutral multiplier,
// because only the clean-sheet half is averaged per fixture — appearance points do
// not depend on the opponent.
func TestThresholdPartsSumToTheWhole(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	var checked int
	for _, el := range e.Boot.Elements {
		el := el
		m := e.Metrics(&el)
		fx := m.Fixtures
		appear, clean := e.thresholdParts(&el, m, fx)
		whole := e.thresholdXP90(&el, m, fx)
		if math.Abs(appear+clean-whole) > 1e-12 {
			t.Fatalf("%s: appearance %.9f + clean sheet %.9f = %.9f, but thresholdXP90 "+
				"says %.9f — the split must be exactly additive or the rate remainder "+
				"breaks", el.WebName, appear, clean, appear+clean, whole)
		}
		// Appearance points carry no opponent dependence, so the appearance half is
		// the flat constant however many fixtures are in the horizon.
		if appear != appearancePoints {
			t.Fatalf("%s: appearance half is %.6f, want the flat %.1f — it must not "+
				"pick up any fixture dependence", el.WebName, appear, appearancePoints)
		}
		checked++
	}
	if checked < 100 {
		t.Skipf("only %d players available", checked)
	}
	t.Logf("additivity held for %d players", checked)
}

// TestBlankRateIsTheAppearanceEstimator is the cheapest guard available against the
// bug this file was written to close, and it is the one the coordinator asked for.
//
// P(appears) was computed twice — playsAtAll(ExpectedMinutes) for the appearance
// point, and 1 - blankFromNotStarting x (1 - StartShare) for the bench slots and
// the defcon exposure — and nothing required the two to agree. Over the archive
// they disagreed by 0.1015 on average and by 0.3751 at worst, and the second one
// was biased upward by eight points of probability because its constant was fitted
// only over start share 0.70 and up.
//
// One disagreeing player fails this. That asymmetry is the point: confirming an
// effect on this project's replay needs tens of points a season and can need 232,
// while refuting an identity needs a single row.
func TestBlankRateIsTheAppearanceEstimator(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	var checked int
	for _, el := range e.Boot.Elements {
		el := el
		m := e.Metrics(&el)
		// blankRate is defined as one minus the estimator, so the forward direction
		// is exact; comparing the other way round is only equal to within a couple
		// of ulp, because 1-(1-p) is not p in binary floating point. The tolerance
		// is 1e-12 rather than 0 for that reason alone — a genuine second estimator
		// disagreed here by up to 0.3751, not by 1e-16.
		// Both directions are EXACT, because appearanceOdds returns the pair from
		// one place: the complement is computed once rather than derived twice.
		if got, want := blankRate(m), 1-appearsInGameweek(m); got != want {
			t.Fatalf("%s: blankRate = %.17g but one minus the appearance estimator is "+
				"%.17g — the two must come from appearanceOdds, not be computed apart",
				el.WebName, got, want)
		}
		if got, want := appearsInGameweek(m), playsAtAll(m.ExpectedMinutes); got != want {
			t.Fatalf("%s (%.1f expected minutes, start share %.3f): the appearance "+
				"estimator says %.17g and playsAtAll says %.17g. Under the shipped rule "+
				"they are the same expression and must agree bit for bit",
				el.WebName, m.ExpectedMinutes, m.StartShare, got, want)
		}
		checked++
	}
	if checked < 100 {
		t.Skipf("only %d players available", checked)
	}
	t.Logf("one estimator, agreeing exactly for %d players", checked)
}

// TestBlankFromNotStartingIsConfinedToThisFile is the structural count, in the
// style of TestEveryScoringEngineGetsRecency.
//
// The behavioural test above cannot catch the failure that actually matters: a
// *later* file quietly adding a third estimator, which agrees with nothing and is
// noticed by no one. The most likely way that happens is someone reaching for
// blankFromNotStarting again, because it looks like a general constant and is not
// — it was fitted over start share 0.70 and up, the regime an eleven occupies, and
// applying it to the whole pool is exactly the bug that was fixed.
//
// So the constant may appear only in appearance.go, which holds its declaration
// and the FPL_NO_UNIFIED_APPEARANCE branch that is the sole reason it survives.
func TestBlankFromNotStartingIsConfinedToThisFile(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var offenders []string
	for _, f := range files {
		if f == "appearance.go" || strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "blankFromNotStarting") {
			offenders = append(offenders, f)
		}
	}
	if len(offenders) > 0 {
		t.Errorf("blankFromNotStarting is referenced in %v. It was fitted over start "+
			"share 0.70+ only and is retained solely for FPL_NO_UNIFIED_APPEARANCE; a "+
			"second consumer is a second estimator of P(appears), which is the bug "+
			"appearance.go exists to prevent. Read appearsInGameweek instead.", offenders)
	}
}

// TestBenchSlotScaleSurvivesTheUnification pins the constant that would otherwise
// have changed meaning silently.
//
// benchSlotScale converts slot probabilities onto BenchWeight's scale by pricing a
// reference eleven, and BenchWeight was swept on the basis that such an eleven's
// four slots sum to four. Changing which estimator blankRate consults would move
// that scale — so the reference is specified by the blank rate it must have rather
// than by a start share, and the scale comes back identical under either rule.
//
// Without this, unifying the estimators would have been two changes at once: a new
// blank rate AND a quiet rescaling of everything the bench is worth.
func TestBenchSlotScaleSurvivesTheUnification(t *testing.T) {
	ref := metricsWithBlankRate(referenceBlankRate)
	if got := blankRate(ref); math.Abs(got-referenceBlankRate) > 1e-9 {
		t.Fatalf("the reference player blanks with probability %.12f, want %.12f — "+
			"benchSlotScale is calibrated on him", got, referenceBlankRate)
	}
	// The legacy rule must produce the same scale, which is the whole claim.
	orig := unifiedAppearance
	scaleFor := func(u bool) float64 {
		unifiedAppearance = u
		r := make([]PlayerMetrics, 11)
		for i := range r {
			r[i] = metricsWithBlankRate(referenceBlankRate)
		}
		r[0].Position = "GKP"
		gk, out := slotProbabilities(r)
		return 4 / (gk + out[0] + out[1] + out[2])
	}
	uni, legacy := scaleFor(true), scaleFor(false)
	unifiedAppearance = orig
	if math.Abs(uni-legacy) > 1e-9 {
		t.Errorf("benchSlotScale is %.9f unified and %.9f legacy; it must not move, or "+
			"BenchWeight stops meaning what it was swept at", uni, legacy)
	}
	if math.Abs(uni-benchSlotScale) > 1e-9 {
		t.Errorf("the shipped benchSlotScale is %.9f but recomputing gives %.9f",
			benchSlotScale, uni)
	}
}

// TestUnifiedAppearanceOffRestoresTheSecondEstimator pins the escape hatch.
//
// FPL_NO_UNIFIED_APPEARANCE=1 must reproduce the old two-estimator behaviour
// exactly, not approximately, because that is the only way the change can be
// re-measured — and a replay arm that is nearly the old behaviour measures nothing.
func TestUnifiedAppearanceOffRestoresTheSecondEstimator(t *testing.T) {
	orig := unifiedAppearance
	unifiedAppearance = false
	defer func() { unifiedAppearance = orig }()

	for _, ss := range []float64{0, 0.25, 0.5, 0.75, 0.856, 1} {
		m := PlayerMetrics{StartShare: ss, ExpectedMinutes: 45}
		want := clamp(blankFromNotStarting*(1-ss), 0, 1)
		// Bit-exact, not close. The old behaviour has to be reproducible exactly or
		// the flag cannot re-measure the change, and this model's response surface is
		// a step function — a rounding difference can flip a squad slot.
		if got := blankRate(m); got != want {
			t.Errorf("with the unification off, blankRate at start share %.3f = %.17g, "+
				"want exactly %.17g", ss, got, want)
		}
	}
}

// TestTheSwitchReachesEveryConsumer is the test that would have caught the bug this
// change shipped once during development, and it is the reason to prefer counting
// consumers over checking one.
//
// P(appears) has two consumers: the derived bench slot weights, through blankRate,
// and defconPerGameweek's exposure. The first version of the unification routed
// defconPerGameweek at the new estimator DIRECTLY rather than through the switch, so
// FPL_NO_UNIFIED_APPEARANCE restored the old bench weights and left the defcon
// exposure on the new rule — a hatch that reproduced neither behaviour, which is
// exactly the "Simulate builds three engines and a patch wired two" failure recorded
// in AGENTS.md.
//
// The prediction benchmark found it as an unexplained zero: the arm that was supposed
// to restore the old behaviour moved not one of 40,611 predictions. **If you add a
// third consumer of P(appears), add it here.**
func TestTheSwitchReachesEveryConsumer(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	orig := unifiedAppearance
	defer func() { unifiedAppearance = orig }()

	// A player the two rules disagree about, with defensive contributions so the
	// defcon consumer is live. Start share well below 0.70 is the regime the legacy
	// constant was never fitted in and the whole reason for the change.
	m := PlayerMetrics{ExpectedMinutes: 40, StartShare: 0.15, DefCon90: 9,
		Position: "DEF", XGC90: 1.3}

	consumers := map[string]func() float64{
		"bench slot weights, via blankRate": func() float64 {
			return blankRate(m)
		},
		"defconPerGameweek exposure": func() float64 {
			return e.defconPerGameweek(2, m)
		},
	}
	for name, f := range consumers {
		unifiedAppearance = false
		off := f()
		unifiedAppearance = true
		on := f()
		if off == on {
			t.Errorf("%s: reads %.9f under both rules, so FPL_NO_UNIFIED_APPEARANCE does "+
				"not reach it. Every consumer of P(appears) must go through "+
				"appearanceOdds, or the escape hatch reproduces neither behaviour.",
				name, on)
		}
	}
	if len(consumers) != 2 {
		t.Errorf("this test knows about %d consumers of P(appears); update it when you "+
			"add one", len(consumers))
	}
}

// TestTheTwoEstimatorsGenuinelyDisagreed is the evidence that the unification was
// worth making rather than a tidy-up.
//
// It reproduces the worked case from the top of appearance.go: a player who never
// starts but comes on for forty-five minutes every week appears in *every*
// gameweek, and the start-share estimator calls him a 62.4% blanker. Neither
// estimator is right about him — mean minutes cannot tell him apart from someone
// who starts half the season and plays ninety — but they are wrong by 0.336, in
// opposite directions, about a player the model routinely holds.
func TestTheTwoEstimatorsGenuinelyDisagreed(t *testing.T) {
	m := PlayerMetrics{ExpectedMinutes: 45, StartShare: 0}
	orig := unifiedAppearance
	defer func() { unifiedAppearance = orig }()

	unifiedAppearance = false
	legacy := 1 - blankRate(m)
	unifiedAppearance = true
	unified := 1 - blankRate(m)

	if gap := math.Abs(unified - legacy); gap < 0.30 {
		t.Errorf("the two estimators differ by only %.4f for a player who never starts "+
			"and plays 45 minutes; the recorded gap is 0.336, so either the fit moved or "+
			"this case stopped being the one that separates them", gap)
	}
	if legacy >= unified {
		t.Errorf("legacy says %.4f and unified says %.4f; the start-share estimator "+
			"under-credits appearance here, which is the direction that matters",
			legacy, unified)
	}
}

// TestTheAppearanceFitReachesEveryConsumer is TestTheSwitchReachesEveryConsumer's
// sibling, for the other escape hatch on this file.
//
// FPL_APPEARANCE_FIT moves the two curves themselves rather than choosing between
// two estimators, so it reaches strictly more: the appearance points and the clean
// sheet's sixty-minute scaling in Score, on top of the bench slot weights and the
// defensive-contribution exposure the switch already touched. A refit that reached
// only some of them would measure a model nobody can ship — the failure the switch
// test exists for, arriving through a different door.
//
// The last consumer is the one that makes this worth writing. The four above it are
// the functions the fit is made of, and a test of those is nearly a tautology;
// scoring a real element end to end is the check that Score has not stopped calling
// them. **If you add a consumer of either curve, add it here.**
func TestTheAppearanceFitReachesEveryConsumer(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	defer SetAppearanceFit(ShippedAppearanceFit())

	// A rotation player, which is where the two curves are steepest and where any
	// refit has to earn its keep. NOTE the band errors quoted historically here
	// (-0.020 to -0.039, +0.013 at the top) are the SUPERSEDED reliability proxy's,
	// not this model's — see TestDiagSixtyMinutes. The shipped curve's error crosses
	// zero near fifty: about -0.043 at 20-30 mean minutes and +0.050 at 60-70, which
	// is why 45 is the sensitive point rather than an arbitrary one.
	m := PlayerMetrics{ExpectedMinutes: 45, StartShare: 0.5, DefCon90: 9,
		Position: "DEF", XGC90: 1.3}

	// An element in the same band, so the end-to-end row is sensitive for the same
	// reason the synthetic one is. A positive score is part of the requirement
	// rather than an assumption: availabilityFactor zeroes an unavailable player
	// outright, and such a row reads 0.000 under every fit — which would look
	// exactly like a consumer that is not wired.
	var subject *fpl.Element
	for i := range e.Boot.Elements {
		el := &e.Boot.Elements[i]
		if mm := e.Metrics(el); mm.ExpectedMinutes > 25 && mm.ExpectedMinutes < 70 &&
			mm.Score > 0.5 {
			subject = el
			break
		}
	}
	if subject == nil {
		t.Skip("no scoring player in the 25-70 expected-minutes band this week")
	}

	consumers := map[string]func() float64{
		"appearance points, via appearanceFactor": func() float64 {
			return appearanceFactor(m.ExpectedMinutes)
		},
		"the clean sheet's sixty-minute scaling, via playsSixty": func() float64 {
			return playsSixty(m.ExpectedMinutes)
		},
		"bench slot weights, via blankRate": func() float64 {
			return blankRate(m)
		},
		"defconPerGameweek exposure": func() float64 {
			return e.defconPerGameweek(2, m)
		},
		"PlayerMetrics.Score, end to end": func() float64 {
			return e.Metrics(subject).Score
		},
	}
	for name, f := range consumers {
		SetAppearanceFit(ShippedAppearanceFit())
		shipped := f()
		// Deliberately far from shipped. A refit worth screening will be much
		// closer than this, and a test that only fires on a large perturbation
		// still catches a consumer that is not wired at all, which is the failure
		// mode being guarded.
		SetAppearanceFit(0.05, 55, 24, 0.85)
		moved := f()
		if shipped == moved {
			t.Errorf("%s: reads %.9f under both fits, so FPL_APPEARANCE_FIT does not "+
				"reach it. Every consumer of the two curves must read the package "+
				"variables, not a private copy of the constants.", name, shipped)
		}
	}
	if len(consumers) != 5 {
		t.Errorf("this test knows about %d consumers of the appearance curves; update "+
			"it when you add one", len(consumers))
	}
}

// TestTheShippedFitIsWhatRuns is the invariant the override owes.
//
// Making four constants into four variables is exactly the shape of change that can
// alter a shipped number by accident — the initialiser is the only thing holding the
// live curves at the measured values, and nothing else in the package would notice if
// it passed something else. This is cheaper than any reviewer and runs every time,
// which is the trade this project's review guard makes in its own comment.
//
// It asserts the values *and* the resulting curves, because the four numbers reaching
// the right variables in the wrong order would satisfy a check on the set of them.
func TestTheShippedFitIsWhatRuns(t *testing.T) {
	if os.Getenv("FPL_APPEARANCE_FIT") != "" {
		t.Skip("FPL_APPEARANCE_FIT is set, so the shipped fit is deliberately not running")
	}
	if sixtySlope != shippedSixtySlope || sixtyMidpoint != shippedSixtyMidpoint ||
		condMinutesIntercept != shippedCondMinutesIntercept ||
		condMinutesSlope != shippedCondMinutesSlope {
		t.Fatalf("the live fit is %v/%v/%v/%v and the shipped constants are %v/%v/%v/%v; "+
			"with no override set these must be identical, or the model is scoring "+
			"something nobody measured",
			sixtySlope, sixtyMidpoint, condMinutesIntercept, condMinutesSlope,
			shippedSixtySlope, shippedSixtyMidpoint,
			shippedCondMinutesIntercept, shippedCondMinutesSlope)
	}

	// Four points spanning the range the two curves are consulted over. A
	// transposition of the pairs passes the check above and fails here.
	//
	// Each is the shipped fit evaluated by hand, not a captured output — capturing
	// what the code prints would pin whatever it currently does, including a bug:
	//
	//	playsSixty(15)  1/(1+exp(-0.065(15-48)))    = 0.104799, Markov cap 0.25 idle
	//	playsAtAll(15)  15/(28.15+0.779x15)         = 0.376553
	//	playsSixty(85)  1/(1+exp(-0.065(85-48)))    = 0.917208, floor 0.833 idle
	//	playsAtAll(85)  85/90, the conditional mean clamped at ninety = 0.944444
	//
	// The last one is the clamp doing its job: the raw conditional mean is 94.4
	// minutes, which is not a thing, and without the clamp an ever-present would read
	// 0.901 rather than 0.944.
	for _, c := range []struct{ mins, sixty, atAll float64 }{
		{15, 0.104799, 0.376553},
		{45, 0.451404, 0.711969},
		{65, 0.751196, 0.825030},
		{85, 0.917208, 0.944444},
	} {
		if got := playsSixty(c.mins); math.Abs(got-c.sixty) > 1e-6 {
			t.Errorf("playsSixty(%v) = %.6f, want %.6f", c.mins, got, c.sixty)
		}
		if got := playsAtAll(c.mins); math.Abs(got-c.atAll) > 1e-6 {
			t.Errorf("playsAtAll(%v) = %.6f, want %.6f", c.mins, got, c.atAll)
		}
	}
}

// TestTheAppearanceFitOverrideIsAllOrNothing pins the refusal in appearanceFit.
//
// The two curves are coupled — playsAtAll takes the max of its identity and
// playsSixty, so P(appears) can never fall below P(60+) — and a half-applied
// override can therefore spend its entire effect on that max rather than on the
// term it was aimed at, which is a silent wrong answer rather than an error.
func TestTheAppearanceFitOverrideIsAllOrNothing(t *testing.T) {
	a, b, c, d := 0.065, 48.0, 28.15, 0.779

	for _, bad := range []string{
		"", "0.05", "0.05,55", "0.05,55,24", "0.05,55,24,0.85,1", "0.05,55,24,banana",
	} {
		t.Setenv("FPL_APPEARANCE_FIT", bad)
		w, x, y, z := appearanceFit(a, b, c, d)
		if w != a || x != b || y != c || z != d {
			t.Errorf("FPL_APPEARANCE_FIT=%q was accepted as %v/%v/%v/%v; a partial or "+
				"unparseable fit must fall back to the shipped one entirely",
				bad, w, x, y, z)
		}
	}

	t.Setenv("FPL_APPEARANCE_FIT", " 0.05 , 55 , 24 , 0.85 ")
	if w, x, y, z := appearanceFit(a, b, c, d); w != 0.05 || x != 55 || y != 24 || z != 0.85 {
		t.Errorf("a complete fit read as %v/%v/%v/%v, want 0.05/55/24/0.85 with the "+
			"spaces trimmed", w, x, y, z)
	}
}
