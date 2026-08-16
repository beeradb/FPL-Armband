package analysis

import (
	"math"
	"testing"
)

// neutralScale prices xG and xA one-for-one, which is what these tests want.
//
// Every case in this file is about the clean sheet, the concede deduction or the
// doubles arithmetic — channels the conversion scale does not touch — so a neutral
// scale isolates them from it and keeps each assertion testing what it was written
// to test. The scale's own behaviour is covered by
// TestTheConversionScaleIsAppliedPerPosition and its siblings.
var neutralScale = ConversionScale{Goals: 1, Assists: 1}

// modernRules is the current points table, named through the first season it was
// in force in rather than through a literal, so the one place a season boundary
// is written down stays `scoringrules.go`.
//
// Every test in this package except the season-pin ones is about a channel's
// arithmetic rather than about which table it is priced from, and every value
// they assert — a defender's 6, a forward's 4, an assist's 3, the clean sheet and
// the concede block — is identical in both tables. `ScoringRulesFor` amends the
// goalkeeper's goal and nothing else.
var modernRules = ScoringRulesFor(keeperGoalRuleChangeSeason)

// TestXPointsPricesTheCleanSheetPerFixture.
//
// The recorded doubles class, arriving through a non-linear function. A double
// gameweek is ONE archive row with Fixtures = 2 and XGC summed over both matches,
// so `exp(-(x1+x2))` stands in for `exp(-x1) + exp(-x2)`. The guard that catches the
// linear version of this bug keys on (element, fixture) and cannot see it, because
// nothing is double-counted — the arithmetic is simply the wrong arithmetic.
//
// It has already cost a published figure once, moving a clean-sheet over-prediction
// from 28.3% to 33.1% and turning a bug into what looked like an independent
// corroboration.
//
// This is the regression test. It is constructed rather than sampled: the summed and
// per-fixture forms differ by a factor of two here, far outside anything the archive
// could produce by chance, so a failure means the form changed and nothing else.
func TestXPointsPricesTheCleanSheetPerFixture(t *testing.T) {
	const xgcPerMatch = 1.0

	single := ExpectedCleanSheets(xgcPerMatch, 1)
	if want := math.Exp(-1.0); math.Abs(single-want) > 1e-12 {
		t.Fatalf("single fixture: got %v, want exp(-1) = %v", single, want)
	}

	// Two matches at the same rate must be worth twice one match.
	double := ExpectedCleanSheets(2*xgcPerMatch, 2)
	if want := 2 * math.Exp(-1.0); math.Abs(double-want) > 1e-12 {
		t.Errorf("double gameweek: got %v, want 2*exp(-1) = %v.\n\n"+
			"The per-fixture form is f*exp(-xgc/f). If this returns exp(-2) = %v "+
			"the summed form is back, and a double gameweek is being priced as one "+
			"very hard match instead of two ordinary ones.",
			double, want, math.Exp(-2.0))
	}

	// And state the failure directly: the summed form is not merely different, it
	// is far too small. Pinning the direction stops a "fix" that swaps one wrong
	// form for another.
	if summed := math.Exp(-2 * xgcPerMatch); double <= summed {
		t.Errorf("the per-fixture expectation (%v) must exceed the summed one (%v)",
			double, summed)
	}
}

// TestXPointsLeavesTheUnreplacedChannelsAlone.
//
// The instrument replaces four channels and leaves appearance, bonus, saves, cards
// and defensive contribution realised. That is what lets it run on seasons whose
// bonus regime and defcon availability differ without knowing anything about either.
//
// The check: a player whose underlying exactly matches his returns must score
// xPoints equal to his realised points, whatever else is in that number. If a future
// change starts re-scoring a channel from scratch, this fails — the unmodelled
// points would no longer survive the round trip.
func TestXPointsLeavesTheUnreplacedChannelsAlone(t *testing.T) {
	// A midfielder with two goals and an assist, whose xG and xA say exactly that,
	// plus 3 bonus and a yellow somewhere inside Points. Nothing about bonus or
	// cards is handed to the instrument.
	g := XPointsGW{
		Position: 3, Fixtures: 1, Minutes: 90,
		Points: 16,
		Goals:  2, Assists: 1,
		XG: 2.0, XA: 1.0,
	}
	if got := XPointsResidual(g, neutralScale, modernRules); math.Abs(got) > 1e-12 {
		t.Errorf("residual on a player who converted exactly his underlying is %v, "+
			"want 0", got)
	}
	if got := XPoints(g, neutralScale, modernRules); math.Abs(got-16) > 1e-12 {
		t.Errorf("xPoints = %v, want the realised 16. Bonus and cards are not "+
			"handed to this function, so anything other than the realised total "+
			"means a channel is being re-scored rather than left alone.", got)
	}
}

// TestXPointsRemovesTheConversionAndNotTheChance.
//
// The direction the whole instrument rests on: an over-performer is marked down to
// what his chances were worth, an under-performer is marked up. If this inverts, the
// metric rewards exactly the finishing noise it exists to remove.
func TestXPointsRemovesTheConversionAndNotTheChance(t *testing.T) {
	base := XPointsGW{Position: 3, Fixtures: 1, Minutes: 90, XG: 0.5}

	lucky := base
	lucky.Goals, lucky.Points = 2, 12
	unlucky := base
	unlucky.Goals, unlucky.Points = 0, 2

	if x := XPoints(lucky, neutralScale, modernRules); x >= 12 {
		t.Errorf("a player who scored twice off 0.5 xG must be marked DOWN from 12; "+
			"got %v", x)
	}
	if x := XPoints(unlucky, neutralScale, modernRules); x <= 2 {
		t.Errorf("a player who scored none off 0.5 xG must be marked UP from 2; "+
			"got %v", x)
	}

	// Same chances, so the same xPoints, regardless of what went in. This is the
	// property that makes it a lower-variance instrument at all.
	if a, b := XPoints(lucky, neutralScale, modernRules), XPoints(unlucky, neutralScale, modernRules); math.Abs(a-b) > 1e-9 {
		t.Errorf("two players with identical underlying scored %v and %v; the whole "+
			"point is that these agree", a, b)
	}
}

// TestXPointsIgnoresABlankGameweek.
//
// ⚠️ The first version of this test was VACUOUS, proved by mutation: deleting the
// `Minutes <= 0` guard it exists to pin left every test passing, because at zero
// minutes both 60-minute gates already exclude the clean sheet and the goals and
// assists terms are all zero anyway.
//
// Fixed by giving the row live goals and assists — a player credited with returns he
// cannot have had, which is nonsense data on purpose. Only the guard stops it
// producing a residual, so only the guard can make this pass.
func TestXPointsIgnoresABlankGameweek(t *testing.T) {
	g := XPointsGW{
		Position: 2, Fixtures: 1, Minutes: 0, XGC: 1.4,
		Goals: 1, Assists: 1, XG: 0.2, XA: 0.3,
	}
	if got := XPointsResidual(g, neutralScale, modernRules); got != 0 {
		t.Errorf("residual on a blank gameweek is %v, want 0.\n\nThe minutes guard "+
			"is what makes this zero — every other term here is live.", got)
	}
}

// TestXPointsWiresTheFixtureCountIntoTheResidual.
//
// ⚠️ **This is the test that was missing, and its absence was proved by mutation.**
// TestXPointsPricesTheCleanSheetPerFixture checks ExpectedCleanSheets in isolation;
// no test ever built an XPointsGW with Fixtures != 1, so replacing
// `ExpectedCleanSheets(g.XGC, g.Fixtures)` with `ExpectedCleanSheets(g.XGC, 1)` —
// which for a double IS the summed form this file was written to prevent — left the
// whole suite green.
//
// A test that pins a helper while leaving its call site unpinned is the shape of
// guard this record keeps finding: it passes, it looks like coverage, and the thing
// it names can still break.
// ⚠️ It must vary the fixture count and NOTHING ELSE. A first attempt compared a
// 180-minute double against a 90-minute single, which moves the *eligible* count too
// — and it passed under the very mutation it was written to catch, because the two
// rows still differed through eligibility. Two rows differing in two ways cannot
// isolate either.
//
// So: same minutes, same clean sheet, same xGC, one club fixture against two. That
// holds `eligible` at 1 on both sides and leaves the per-match rate as the only live
// difference.
func TestXPointsWiresTheFixtureCountIntoTheResidual(t *testing.T) {
	// ⚠️ A MIDFIELDER, not a defender. The concede channel is now restricted to
	// single-fixture rows, so a defender's residual would differ between these two
	// rows through that channel even with the clean-sheet wiring broken — which is
	// exactly how the second attempt at this test also passed under the mutation.
	// Midfielders are in cleanSheetPoints and absent from concedeBlock, so the
	// clean sheet is the only live channel here.
	oneFixture := XPointsGW{
		Position: 3, Fixtures: 1, Minutes: 90,
		Points: 6, CleanSheets: 1, XGC: 1.0,
	}
	twoFixtures := oneFixture
	twoFixtures.Fixtures = 2 // 90 minutes still, so still one clean sheet on offer

	a, b := XPointsResidual(oneFixture, neutralScale, modernRules), XPointsResidual(twoFixtures, neutralScale, modernRules)
	if a == b {
		t.Fatalf("one club fixture and two produce the identical residual (%v) at "+
			"equal minutes and equal xGC.\n\n"+
			"Then Fixtures is not reaching the clean-sheet expectation and the "+
			"accumulated xGC is being exponentiated whole — the summed form this "+
			"file exists to prevent.", a)
	}

	// Direction: with the xGC spread over two matches, each match is easier to keep
	// a clean sheet in, so the per-match expectation RISES and a single realised
	// clean sheet is less of an over-performance.
	if b >= a {
		t.Errorf("two fixtures give residual %v and one gives %v; spreading the same "+
			"xGC over two matches must raise the per-match clean-sheet expectation "+
			"and so lower the residual", b, a)
	}
}

// TestXPointsCapsTheCleanSheetAtWhatWasOnOffer.
//
// The club's fixture count is right for splitting the xGC and wrong for counting the
// chances: a clean sheet needs sixty minutes in a PARTICULAR match, so a player who
// played one leg of a double can realise at most one however many his club played.
//
// Uncapped, 320 archive rows were inflated by 505 points between them. Kiwior
// 2023-24 GW34 — two fixtures, 90 minutes, one clean sheet — was charged an expected
// 1.865 against a realisable maximum of 1.
func TestXPointsCapsTheCleanSheetAtWhatWasOnOffer(t *testing.T) {
	oneLeg := XPointsGW{
		Position: 2, Fixtures: 2, Minutes: 90,
		Points: 6, CleanSheets: 1, XGC: 0.14,
	}
	bothLegs := oneLeg
	bothLegs.Minutes = 180

	// He kept the only clean sheet available to him, off a tiny xGC, so he slightly
	// UNDER-performed at worst — never by three points.
	if r := XPointsResidual(oneLeg, neutralScale, modernRules); r < -1.0 {
		t.Errorf("a 90-minute player in a double who kept his one clean sheet has "+
			"residual %v; the uncapped form gives about -3.46 by charging him for a "+
			"second clean sheet he was never eligible for", r)
	}
	// Playing both legs really does put two on offer, so his one clean sheet is a
	// genuine under-performance and the residual must be more negative.
	if XPointsResidual(bothLegs, neutralScale, modernRules) >= XPointsResidual(oneLeg, neutralScale, modernRules) {
		t.Errorf("playing both legs (%v) must carry a more negative residual than "+
			"playing one (%v) — two chances taken once against one taken once",
			XPointsResidual(bothLegs, neutralScale, modernRules), XPointsResidual(oneLeg, neutralScale, modernRules))
	}
}

// TestXPointsPricesTheConcedeDeductionAsAFloor.
//
// The deduction is -1 per two conceded, so its expectation is E[floor(X/2)] and not
// xGC/2. The two differ substantially at the rates real matches produce, and the
// linear version is the intuitive one — which is why baseXP90 carries the same
// Poisson treatment and why this is worth pinning.
func TestXPointsPricesTheConcedeDeductionAsAFloor(t *testing.T) {
	// Two defenders identical except for the scoreline. Everything else — the goals
	// channel and the clean-sheet channel, which is live here because xGC is non-zero
	// — is common to both and cancels in the difference, so what is left is exactly
	// the realised deduction. Differencing rather than asserting an absolute keeps
	// this test from having to recompute the expectation and thereby carry its own
	// copy of the thing it is checking.
	base := XPointsGW{Position: 2, Fixtures: 1, Minutes: 90, XGC: 1.0}

	conceded1 := base
	conceded1.GoalsConceded = 1 // floor(1/2) = 0 deducted
	conceded2 := base
	conceded2.GoalsConceded = 2 // floor(2/2) = 1 deducted

	got := XPointsResidual(conceded2, neutralScale, modernRules) - XPointsResidual(conceded1, neutralScale, modernRules)
	if want := -1.0; math.Abs(got-want) > 1e-9 {
		t.Errorf("conceding a second goal moves the residual by %v, want %v — the "+
			"deduction is a FLOOR at one point per two conceded, so the first goal "+
			"costs nothing and the second costs a point", got, want)
	}

	// A third goal must cost nothing again; a fourth must cost another point. The
	// linear stand-in xgc/2 would charge half a point per goal and pass neither.
	conceded3, conceded4 := base, base
	conceded3.GoalsConceded, conceded4.GoalsConceded = 3, 4
	if d := XPointsResidual(conceded3, neutralScale, modernRules) - XPointsResidual(conceded2, neutralScale, modernRules); math.Abs(d) > 1e-9 {
		t.Errorf("the third goal conceded moved the residual by %v, want 0", d)
	}
	if d := XPointsResidual(conceded4, neutralScale, modernRules) - XPointsResidual(conceded3, neutralScale, modernRules); math.Abs(d+1) > 1e-9 {
		t.Errorf("the fourth goal conceded moved the residual by %v, want -1", d)
	}

	// And the expectation side is a Poisson floor, not lambda/d. At lambda 1 those
	// are 0.26 and 0.5, so the two are separable.
	if e := poissonFloorDiv(2, 1.0); e <= 0 || e >= 0.5 {
		t.Errorf("E[floor(X/2)] at lambda 1 is %v; it must lie strictly between 0 "+
			"and the linear stand-in of 0.5, or this channel has been linearised", e)
	}
}
