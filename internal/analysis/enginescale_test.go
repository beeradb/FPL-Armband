package analysis

import (
	"fmt"
	"math"
	"sort"
	"testing"

	"armband/internal/fpl"
)

// TestTheEngineFitCountsTheExposedElements pins the arm the shipped scoring path
// resolves, so that a measurement of the exposure is a measurement of something.
//
// An **exposed element** is one carrying a realised return whose season-to-date
// expectation is zero: it enters `CalibrationRatio`'s numerator with nothing behind
// it in the denominator, so the fitted scale comes out higher, and a higher scale
// credits more to every OTHER player of that position through `baseXP90`.
//
// `calibrateExpectedStats` applies no coverage gate and no exposure gate, so it
// counts them — and that is a choice rather than an oversight. The function's own
// comment says FPL pays an assist "for winning a penalty that is scored, for a shot
// parried to a team-mate, and for deflected passes — none of which an
// expected-assists model counts", which is a description of the exposed population.
// Pricing that gap is what the term is for.
//
// ⚠️ **This test does not say the shipped behaviour is right.** It says which
// behaviour ships, which is the thing a diagnostic needs pinned before it can
// measure a difference against it. `enginescale_diag_test.go` in `internal/backtest`
// sizes the difference over the archive; a fixture cannot say anything about the
// real population, and that file cannot say the fixture's property holds when no
// archive is reachable.
func TestTheEngineFitCountsTheExposedElements(t *testing.T) {
	// A midfield population whose expected total clears the thin-sample floor with
	// room to spare, so the ratio is the plain one and neither bound is in play.
	//
	// ⚠️ The two channels must not fit the SAME ratio. They were both exactly 1.2
	// once, and a `Goals`/`Assists` swap inside calibrateExpectedStats left this test
	// green — the one mutation it exists to catch. 24 goals off 25 xG against 30
	// assists off 30 xA keeps them apart, and the check below refuses to run if a
	// later edit makes them coincide again.
	backed := fpl.Element{
		ID: 1, ElementType: 3, Team: 1, Minutes: 3000,
		GoalsScored: 24, ExpectedGoals: fpl.Num(25),
		Assists: 30, ExpectedAssists: fpl.Num(30),
	}
	// The exposed one: realised returns, no expectation at all.
	exposed := fpl.Element{
		ID: 2, ElementType: 3, Team: 1, Minutes: 900,
		GoalsScored: 5, ExpectedGoals: fpl.Num(0),
		Assists: 6, ExpectedAssists: fpl.Num(0),
	}

	with := scaleEngine(t, backed, exposed).scaleFor(3)
	without := scaleEngine(t, backed).scaleFor(3)

	// Without the exposed element each channel fits its own backed ratio, so any
	// departure is the exposed element's realised return and nothing else.
	if math.Abs(without.Assists-1) > 1e-12 || math.Abs(without.Goals-24.0/25.0) > 1e-12 {
		t.Fatalf("the backed population alone fits %.6f/%.6f, want %.6f/1.0 — the "+
			"fixture no longer isolates what it is supposed to",
			without.Goals, without.Assists, 24.0/25.0)
	}
	wantA, wantG := 36.0/30.0, 29.0/25.0
	if wantA == wantG {
		t.Fatal("the two channels fit the same ratio, so a Goals/Assists swap in " +
			"calibrateExpectedStats would leave this test green. Change one fixture " +
			"number rather than deleting this check")
	}
	if math.Abs(with.Assists-wantA) > 1e-12 {
		t.Errorf("with an exposed element the assist scale fits %.6f, want %.6f. "+
			"The shipped fit counts a realised assist whose expectation is zero, and "+
			"every measurement of that exposure is measured against this",
			with.Assists, wantA)
	}
	if math.Abs(with.Goals-wantG) > 1e-12 {
		t.Errorf("with an exposed element the goal scale fits %.6f, want %.6f",
			with.Goals, wantG)
	}
}

// TestTheEngineScaleFloorAbsorbsExposureEntirely pins the bound that makes most of
// the archive's exposure unreachable, and pins that it is the FLOOR doing it rather
// than the clamp.
//
// The two are distinguishable by arithmetic and not merely by likelihood: a
// population with realised returns and zero expectation would fit an unbounded ratio,
// which the clamp would return as **3.0**. Getting exactly **1.0** identifies the
// thin-sample guard uniquely. That distinction matters because the two absorb
// differently — the floor removes the term, the clamp saturates it at triple.
//
// ⚠️ **The zero-expectation fixture pins the guard's EXISTENCE, not its level**, and
// on its own it is satisfied by any positive threshold: dropping the constant to
// 1e-6 left it green while flipping the archive census wholesale. So a second,
// bracketing pair runs below — one population just under the shipped threshold and
// one just over — which is what makes the *level* load-bearing here rather than
// somewhere else in the tree.
func TestTheEngineScaleFloorAbsorbsExposureEntirely(t *testing.T) {
	// A forward population that is nothing but exposure: every realised return, no
	// expectation anywhere.
	all := fpl.Element{
		ID: 1, ElementType: 4, Team: 1, Minutes: 3000,
		GoalsScored: 40, ExpectedGoals: fpl.Num(0),
		Assists: 25, ExpectedAssists: fpl.Num(0),
	}
	got := scaleEngine(t, all).scaleFor(4)
	if got.Goals != 1 || got.Assists != 1 {
		t.Fatalf("a position with realised returns and no expectation fits %+v. "+
			"Want the neutral 1/1: below the thin-sample guard the scale is not fitted "+
			"at all, which is what makes an unbounded exposure unreachable", got)
	}
	if got.Goals == 3 || got.Assists == 3 {
		t.Errorf("the scale saturated at the clamp rather than falling back to " +
			"neutral, which absorbs the exposure at TRIPLE rather than removing it")
	}
}

// TestTheThinSampleGuardSitsBetweenTenAndTwentyFive brackets the threshold itself.
//
// `minCalibrationSample` is unexported, so this does not name it: it asserts that a
// position with **10** expected events is not fitted and one with **25** is. Those
// two facts together confine the constant to `(10, 25]`, which is what the archive
// census depends on — the floor releasing partway through the opening weeks is the
// whole reason most of the exposure is unreachable, and a threshold of 1e-6 or of
// 200 would move that completely while leaving a zero-expectation fixture green.
//
// Deliberately a bracket rather than an equality. Pinning 20.0 exactly would be a
// second copy of the constant, which is the failure this project is named for; a
// bracket wide enough to hold it cannot drift into being the definition.
func TestTheThinSampleGuardSitsBetweenTenAndTwentyFive(t *testing.T) {
	thin := fpl.Element{
		ID: 1, ElementType: 4, Team: 1, Minutes: 3000,
		GoalsScored: 20, ExpectedGoals: fpl.Num(10),
		Assists: 20, ExpectedAssists: fpl.Num(10),
	}
	if got := scaleEngine(t, thin).scaleFor(4); got.Goals != 1 || got.Assists != 1 {
		t.Errorf("a position with 10 expected events fits %+v, want the neutral 1/1. "+
			"The thin-sample guard has dropped below 10, and the archive census in "+
			"enginescale_diag_test.go — where the floor releasing partway through the "+
			"opening weeks is what makes most of the exposure unreachable — no longer "+
			"describes what runs", got)
	}

	fat := fpl.Element{
		ID: 1, ElementType: 4, Team: 1, Minutes: 3000,
		GoalsScored: 50, ExpectedGoals: fpl.Num(25),
		Assists: 50, ExpectedAssists: fpl.Num(25),
	}
	if got := scaleEngine(t, fat).scaleFor(4); got.Goals != 2 || got.Assists != 2 {
		t.Errorf("a position with 25 expected events and twice as many realised fits "+
			"%+v, want 2/2. The thin-sample guard has risen above 25, so positions "+
			"this file's census reports as FITTED would in fact be floored", got)
	}
}

// TestTheEngineScaleReachesBaseXP90 is the liveness half, and it is the one that can
// fail for a real reason: a confinement check on a path that cannot carry the effect
// confirms nothing, so the pin above is worth nothing unless the fitted scale
// actually moves a score.
//
// It asserts that a player's `BaseXP90` — the quantity `Score` is built from — moves
// when the fit moves, and that it moves in the direction the mechanism says. This is
// what separates `e.xScale` from the instrument-side `Player.Conversion`, which is
// read at one non-test site and cannot move a replayed point.
func TestTheEngineScaleReachesBaseXP90(t *testing.T) {
	// A midfielder with real expected assists per 90, so the assist term of his
	// baseXP90 is non-zero and the scale has something to multiply.
	subject := fpl.Element{
		ID: 1, ElementType: 3, Team: 1, Minutes: 3000, Starts: 33,
		GoalsScored: 25, ExpectedGoals: fpl.Num(25), ExpectedGoalsPer90: fpl.Num(0.75),
		Assists: 30, ExpectedAssists: fpl.Num(30), ExpectedAssistsPer90: fpl.Num(0.9),
		Status: "a",
	}
	exposed := fpl.Element{
		ID: 2, ElementType: 3, Team: 1, Minutes: 900,
		Assists: 6, ExpectedAssists: fpl.Num(0),
		Status: "a",
	}

	lean := scaleEngine(t, subject)
	rich := scaleEngine(t, subject, exposed)
	if lean.scaleFor(3).Assists >= rich.scaleFor(3).Assists {
		t.Fatalf("the exposed element did not raise the fitted assist scale "+
			"(%.6f against %.6f); the rest of this test would prove nothing",
			lean.scaleFor(3).Assists, rich.scaleFor(3).Assists)
	}

	before := lean.Metrics(&lean.Boot.Elements[0]).BaseXP90
	after := rich.Metrics(&rich.Boot.Elements[0]).BaseXP90
	if after <= before {
		t.Errorf("BaseXP90 reads %.6f with the exposed element in the fit and %.6f "+
			"without it. A higher assist scale must raise the assist term of every "+
			"player of the position — if it does not, scaleFor has stopped reaching "+
			"baseXP90 and the whole exposure question is confined to reporting",
			after, before)
	}
}

// scaleEngine builds an engine over exactly the given elements, wired the ordinary
// way, so `scaleFor` returns the fit over that population and nothing else.
//
// The events are populated because `buildFixtureIndex` reads `NextEvent`, and none
// are finished: a fixture is not what any of these tests are about, and leaving the
// season un-started keeps every other term out of the way.
//
// Teams are derived from whatever team ids the elements themselves reference,
// rather than hardcoded to one — squad-construction fixtures
// (squadclubtrap_test.go) need several clubs to exercise the 3-per-club cap,
// and every caller here that still passes single-team fixtures gets exactly
// the one team it always did, just built the same way as everyone else
// instead of as a special case.
func scaleEngine(t *testing.T, els ...fpl.Element) *Engine {
	t.Helper()
	teamIDs := map[int]bool{1: true} // the historical default, kept for els with no Team set
	for _, e := range els {
		if e.Team != 0 {
			teamIDs[e.Team] = true
		}
	}
	ids := make([]int, 0, len(teamIDs))
	for id := range teamIDs {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	b := &fpl.Bootstrap{
		ElementTypes: []fpl.ElementType{
			{ID: 1, SingularNameShort: "GKP"}, {ID: 2, SingularNameShort: "DEF"},
			{ID: 3, SingularNameShort: "MID"}, {ID: 4, SingularNameShort: "FWD"},
		},
		Elements: append([]fpl.Element(nil), els...),
	}
	for _, id := range ids {
		b.Teams = append(b.Teams, fpl.Team{ID: id, Name: fmt.Sprintf("Test%d", id), ShortName: fmt.Sprintf("T%02d", id)})
	}
	for i := 1; i <= 38; i++ {
		b.Events = append(b.Events, fpl.Event{ID: i})
	}
	return NewEngineFull(b, nil, DefaultWeights(), Congestion{}, RoleRisk{})
}
