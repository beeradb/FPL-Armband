package backtest

import (
	"encoding/json"
	"math"
	"testing"

	"armband/internal/analysis"
)

// aCalibratableSeason builds a season with enough expected goals and assists to
// clear minCalibrationSample, so the fitted scale is the season's own ratio rather
// than the thin-sample fallback of 1.0.
//
// The floor is 20.0 expected events per position, which is why this needs volume
// rather than a couple of hand-picked rows: a fixture under the floor tests the
// fallback and nothing else, and it would pass every assertion below for the wrong
// reason. Defenders convert at 0.5 here and forwards at 2.0, deliberately far apart
// and on opposite sides of neutral.
// ⚠️ **Every position carries TWO profiles, and that is load-bearing rather than
// decoration.** A first version gave all ten players of a position byte-identical
// rows, and two of the tests below could not then fail:
//
//   - "every player of a position carries the same scale" is true under a PER-PLAYER
//     fit as well, when every player is identical — so it could not detect the
//     arm-dependent fit it is named for. Here the two profiles have different
//     individual ratios (a defender converts 0.33 or 1.00) and only the position
//     aggregate is the target, so a per-player fit gives ten different scales.
//   - each row's attacking residual was individually zero, so summing exercised no
//     CANCELLATION — the property the in-sample identity actually rests on. Here
//     profile A sits at -1.0 and profile B at +1.0 per row, and only the sum is zero.
//
// The floor is 20.0 expected events per position and every channel clears it by
// more than an order of magnitude, so the fixture tests the fit rather than the
// thin-sample fallback. Keepers are included at a deliberately TINY volume, because
// they are the one position where the identity does not hold — see the assertion in
// TestTheConversionScaleIsFittedFromTheSeasonsOwnRows.
func aCalibratableSeason(name string) *Season {
	s := &Season{Name: name, Players: map[int]*Player{}}
	add := func(id, pos, goals, assists int, xg, xa float64) {
		gws := map[int]GW{}
		for gw := 1; gw <= 30; gw++ {
			gws[gw] = GW{
				Minutes: 90, Fixtures: 1, Points: 2,
				Goals: goals, Assists: assists, XG: xg, XA: xa,
			}
		}
		s.Players[id] = &Player{ID: id, Code: 100000 + id, Type: pos, GWs: gws}
	}
	// Five of each profile, so the aggregate hits the stated target while no
	// individual player does.
	for i := 0; i < 5; i++ {
		// DEF: goals 2/4.0 = 0.5, assists 2/2.0 = 1.0
		add(100+i, 2, 1, 1, 3.0, 1.5)
		add(110+i, 2, 1, 1, 1.0, 0.5)
		// MID: goals 2/2.0 = 1.0, assists 2/2.0 = 1.0
		add(200+i, 3, 1, 1, 1.5, 1.5)
		add(210+i, 3, 1, 1, 0.5, 0.5)
		// FWD: goals 4/2.0 = 2.0, assists 2/1.0 = 2.0
		add(300+i, 4, 2, 1, 1.5, 0.75)
		add(310+i, 4, 2, 1, 0.5, 0.25)
	}
	// One keeper, with the volume a keeper really has: a couple of assists off
	// almost no expected anything. He is here so the thin-sample fallback is
	// exercised rather than assumed.
	add(1, 1, 0, 1, 0.0, 0.05)
	return s
}

// TestTheConversionScaleIsFittedFromTheSeasonsOwnRows pins that the scale is the
// season's realised ratio and that the thin-sample floor is not silently swallowing
// it — a test that passed only through the 1.0 fallback would assert nothing.
func TestTheConversionScaleIsFittedFromTheSeasonsOwnRows(t *testing.T) {
	s := aCalibratableSeason("2024-25")
	s.calibrateConversion()

	want := map[int]analysis.ConversionScale{
		2: {Goals: 0.5, Assists: 1.0},
		3: {Goals: 1.0, Assists: 1.0},
		4: {Goals: 2.0, Assists: 2.0},
	}
	got := s.ConversionScales()
	for pos, w := range want {
		g, ok := got[pos]
		if !ok {
			t.Fatalf("position %d carries no conversion scale", pos)
		}
		if math.Abs(g.Goals-w.Goals) > 1e-9 || math.Abs(g.Assists-w.Assists) > 1e-9 {
			t.Errorf("position %d fitted %+v, want %+v", pos, g, w)
		}
	}
	// The fixture must be capable of distinguishing a fit from the fallback. If
	// every position came back neutral the assertions above would hold for a
	// season that never cleared minCalibrationSample.
	if got[2].Goals == 1.0 && got[4].Goals == 1.0 {
		t.Error("every position fitted neutral 1.0; this fixture is under the " +
			"thin-sample floor and tests the fallback rather than the fit")
	}

	// ⚠️ And the keeper is the documented exception, asserted rather than assumed.
	//
	// minCalibrationSample binds on the EXPECTED total, not the realised one, and a
	// keeper's expected total is nowhere near 20 in any season — so GKP falls back
	// to neutral 1.0 and keeps whatever attacking residual he has. That is the
	// right behaviour, but it means "the position-mean attacking residual is zero
	// by construction" is a claim about DEF, MID and FWD and not about all four.
	//
	// It is not academic: keeper assists/xA across the archive runs 1.5 to 7.8,
	// because roughly half a keeper's assists are long punts that xA cannot model.
	// A fixture with no keeper in it cannot notice any of this.
	if k := got[1]; k.Goals != 1.0 || k.Assists != 1.0 {
		t.Errorf("the keeper fitted %+v, want neutral 1.0/1.0 from the thin-sample "+
			"floor. Fitting a keeper's ~1.5 expected assists would give a ratio of "+
			"20 and price every goalkeeper as a striker", k)
	}
}

// TestTheShippedFitCountsTheExposedReturns pins WHICH arm of conversionFit the
// shipped path resolves.
//
// # Why this is owed, established by mutation rather than argued
//
// `conversionFit` takes an `exposedReturns` parameter so a diagnostic can size the
// alternative without carrying a second copy of the fit. That parameter is a
// choice about the instrument, and before this test nothing pinned it. Flipping
// `calibrateConversion` to `dropExposedReturns` and running the package fails
// exactly ONE test — `TestTheConversionScaleFollowsTheXGRepair` — whose message
// says "the scale was fitted against a different data state than the one the
// instrument scores", which points a reader at the xG repair rather than at the
// arm. Every other property survives the flip, because dropping rows from the
// numerator leaves the fit season-global, still fitted from the season's own
// rows, and still keyed by position.
//
// ⚠️ **In particular the sibling identity test below does NOT catch it**, and it
// looks as though it should. `aCalibratableSeason` gives every outfield profile a
// non-zero xA and the keeper xA 0.05, so the fixture contains no exposed row at
// all and the two arms are byte-identical on it. That is this record's
// "byte-identical result is not a tie" arriving inside a test fixture: the
// comparison could not run.
//
// # The exposed row is added here rather than to the shared fixture
//
// Deliberately local. `aCalibratableSeason` is read by four tests that assert
// exact fitted ratios, and adding an assist with no xA to it would move the
// forward scale and rewrite their expectations for a reason unrelated to what
// they check.
func TestTheShippedFitCountsTheExposedReturns(t *testing.T) {
	s := aCalibratableSeason("2024-25")

	// An EXPOSED return: an assist FPL paid for, on a row this season records xA
	// for and whose own xA is zero. A won penalty, a deflected pass, a rebound
	// off the player's own shot — none of which an expected-assists model counts.
	gws := map[int]GW{}
	for gw := 1; gw <= 30; gw++ {
		gws[gw] = GW{Minutes: 90, Fixtures: 1, Points: 2, Assists: 1, XA: 0}
	}
	s.Players[400] = &Player{ID: 400, Code: 100400, Type: 4, GWs: gws}

	s.calibrateConversion()

	counted := s.conversionFit(countExposedReturns)
	dropped := s.conversionFit(dropExposedReturns)

	// The mutation guard comes FIRST, because everything below is vacuous without
	// it: if the two arms agree, the equality check cannot distinguish them and
	// would pass under exactly the change it exists to catch.
	if counted[4] == dropped[4] {
		t.Fatalf("the two arms fitted the same forward scale %+v, so this fixture "+
			"has no exposed return in it and the check below asserts nothing",
			counted[4])
	}

	for pos, want := range counted {
		if got := s.ConversionScales()[pos]; got != want {
			t.Errorf("position %d resolved %+v but the counted arm fits %+v (the "+
				"dropped arm fits %+v). The shipped instrument must price a realised "+
				"return the season records no underlying for, because that is what "+
				"makes the position-mean attacking residual zero in sample — see "+
				"underlyingCoverage for why gating those rows is the wrong repair",
				pos, got, want, dropped[pos])
		}
	}
}

// TestTheFittedScaleZeroesThePositionMeanAttackingResidual pins the arithmetic
// property the in-sample fit has, and names it as arithmetic.
//
// ⚠️ **This is not evidence for the fix's SIZE and must not be read as any.**
// Fitting s = ΣG/ΣXG over the same rows the residual is then evaluated on makes the
// position-mean attacking residual exactly zero by construction. The test is here
// because that identity is also the sharpest available wiring check — it fails if
// the scale is applied to the wrong channel, keyed by the wrong position, or fitted
// over a different row population than it is evaluated over — and because a reader
// who does not know the fit is in sample will otherwise mistake the zero for a
// measurement.
//
// What the identity costs is recorded on XPointsResidual: the instrument reports
// within-position conversion deviation only, and can no longer see a position-level
// or season-level conversion effect.
func TestTheFittedScaleZeroesThePositionMeanAttackingResidual(t *testing.T) {
	s := aCalibratableSeason("2024-25")
	s.calibrateConversion()

	// Attacking residual only: the clean sheet and the concede deduction are
	// untouched by the scale, and summing them in would hide the property.
	// ⚠️ The per-position point value is deliberately NOT reproduced here.
	//
	// The identity this test asserts — Sum(G - s*xG) = 0 with s fitted in sample —
	// is invariant to the multiplier, so a local copy of the scoring table would
	// guard nothing while re-creating the divergence
	// TestTheXPointsScriptsShareTheScoringTable exists to have caught once already:
	// a first version of this file carried `goalValue(1) = 6` against
	// `goalPoints[1] = 10`, which is exactly the GOAL[1] divergence that test
	// records, and it survived for exactly the same reason — no keeper in the
	// fixture, and an identity that holds for any multiplier.
	// ⚠️ Amended 2026-08-16: "the GOAL[1] bug" was the wrong word. 6 is a real FPL
	// value — what a keeper's goal paid in 2020-21, decoded from the archive's only
	// goalkeeper goal; see internal/analysis/scoringrules.go. The defect was a
	// table with no season attached, which is a different thing and is the one this
	// comment's argument actually needs. The reason it survived is unchanged.
	// Weight the two channels 1 and 1.
	attacking := func(p *Player, g GW, sc analysis.ConversionScale) float64 {
		return (float64(g.Goals) - g.XG*sc.Goals) +
			(float64(g.Assists) - g.XA*sc.Assists)
	}

	sums := map[int]float64{}
	for id, p := range s.Players {
		_ = id
		for gw := 1; gw <= 38; gw++ {
			g, ok := p.GWs[gw]
			if !ok || g.Minutes <= 0 {
				continue
			}
			sums[p.Type] += attacking(p, g, p.Conversion)
		}
	}
	// ⚠️ DEF, MID and FWD only. The identity holds exactly where the fit is the raw
	// ratio, and NOT where minCalibrationSample or the [0.5, 3.0] clamp binds.
	for _, pos := range []int{2, 3, 4} {
		if total := sums[pos]; math.Abs(total) > 1e-6 {
			t.Errorf("position %d has a summed attacking residual of %v, want ~0; "+
				"the scale is not fitted over the rows it is evaluated over", pos, total)
		}
	}

	// And the keeper is the exception, asserted so the scope of "exactly zero by
	// construction" is pinned rather than left to a comment. He is under the
	// thin-sample floor, so his scale is neutral 1.0 and his attacking residual
	// survives — which is the right behaviour and the reason the claim must always
	// be stated as DEF/MID/FWD. A fixture with no keeper in it says nothing here,
	// and this file had none until a code review pointed it out.
	if math.Abs(sums[1]) < 1e-6 {
		t.Error("the keeper's summed attacking residual is ~0, so this fixture " +
			"cannot show that the in-sample identity does NOT reach a position on " +
			"the thin-sample floor — which is the one caveat the claim needs")
	}

	// The mutation this exists to catch: cross the defender and forward scales. If
	// the position key were dropped anywhere on the path the totals would still be
	// zero, and the test above would pass under exactly the bug it is named for.
	crossed := map[int]analysis.ConversionScale{
		2: s.ConversionScales()[4],
		4: s.ConversionScales()[2],
	}
	for _, pos := range []int{2, 4} {
		var total float64
		for _, p := range s.Players {
			if p.Type != pos {
				continue
			}
			for gw := 1; gw <= 38; gw++ {
				g, ok := p.GWs[gw]
				if !ok || g.Minutes <= 0 {
					continue
				}
				total += attacking(p, g, crossed[pos])
			}
		}
		if math.Abs(total) < 1e-6 {
			t.Errorf("position %d still sums to ~0 under the OTHER position's "+
				"scale; the assertion above cannot detect a mis-keyed scale and "+
				"is therefore vacuous", pos)
		}
	}
}

// TestTheConversionScaleIsSeasonGlobal pins the property that keeps every paired
// comparison a comparison of ONE metric.
//
// A scale fitted on league-wide rows for a (season, position) is identical in every
// arm, so hold_xpoints and policy_xpoints differences stay differences of one
// quantity. A scale fitted on the SQUAD's rows, or on a CELL's window, would move
// with the arm and quietly turn a paired difference into two metrics — which is the
// failure that would be hardest of all to see, because both arms would still look
// like xPoints.
//
// The check has two halves: every player of a position carries the identical scale
// (so nothing per-player has crept in), and dropping a squad's worth of players
// from the SEASON does change it (so the population really is the season and the
// first half is not passing on a constant).
func TestTheConversionScaleIsSeasonGlobal(t *testing.T) {
	s := aCalibratableSeason("2024-25")
	s.calibrateConversion()

	byPos := map[int]analysis.ConversionScale{}
	for _, p := range s.Players {
		if seen, ok := byPos[p.Type]; ok {
			if seen != p.Conversion {
				t.Errorf("two players of position %d carry different scales, %+v "+
					"and %+v; the scale must be a property of the season and the "+
					"position, never of the player", p.Type, seen, p.Conversion)
			}
			continue
		}
		byPos[p.Type] = p.Conversion
	}

	// And it is genuinely fitted from the population rather than constant. Change
	// the season's forwards and the forward scale must follow; if it does not, the
	// first half above is passing on something that never varies.
	before := s.ConversionScales()[4]
	for _, p := range s.Players {
		if p.Type != 4 {
			continue
		}
		for gw := 1; gw <= 30; gw++ {
			g := p.GWs[gw]
			g.Goals = 1
			p.GWs[gw] = g
		}
	}
	s.calibrateConversion()
	if after := s.ConversionScales()[4]; math.Abs(after.Goals-before.Goals) < 1e-9 {
		t.Errorf("halving every forward's goals left the forward goal scale at %v; "+
			"it is not fitted from the season's rows", after.Goals)
	}
}

// TestTheConversionScaleNeverReachesRealisedPoints is the confinement invariant,
// and it is the strongest cheap evidence this change has.
//
// The whole safety argument for fitting a scale on a season's own rows is that
// xPoints is INSTRUMENTATION: `weekScoreWithChip` carries two totals through one
// set of decisions, the decisions read `Points`, and nothing on the scoring path
// reads `XPoints`. If that were false the scale would be a season-global quantity
// reaching the decision path, which is a point-in-time leak of the kind
// `TestPointInTimeHidesFutureResults` exists to stop — and it would be invisible,
// because both arms would still look like a replay.
//
// A call-graph reading establishes this once, for the tree as it is today. This
// asserts it every run: perturb the scale as violently as the clamp allows and the
// realised points must be BYTE-identical while xPoints moves. It is the same shape
// as the recorded confinement check on the gate arms, where the points arm coming
// back byte-identical is what proved the fixes were confined to the instrument.
func TestTheConversionScaleNeverReachesRealisedPoints(t *testing.T) {
	xi := func(s *Season) []*Player { return idsToPlayers(s, xpXI) }

	base := mkXPointsSeason()
	before := weekScore(xi(base), nil, xpWeek, 13, 14)

	// The same fixture, scored through a scale at the far end of the [0.5, 3.0]
	// clamp. Every player, every position.
	moved := mkXPointsSeason()
	for _, p := range moved.Players {
		p.Conversion = analysis.ConversionScale{Goals: 3, Assists: 3}
	}
	after := weekScore(xi(moved), nil, xpWeek, 13, 14)

	if before.Points != after.Points {
		t.Errorf("tripling the conversion scale moved realised points from %d to "+
			"%d. The scale is a season-global quantity; if it reaches the points "+
			"the replay scores and decides on, it is a point-in-time leak and the "+
			"whole arm-invariance argument for fitting it in sample is void",
			before.Points, after.Points)
	}

	// And the fixture has to be able to detect the failure. If xPoints did not move
	// either, the assertion above would pass on a season where the scale reaches
	// nothing at all, which is a test that cannot fail.
	if closeEnough(before.XPoints, after.XPoints) {
		t.Errorf("tripling the conversion scale left xPoints at %v; the scale is "+
			"reaching neither metric here, so the points assertion above is "+
			"vacuous", after.XPoints)
	}
}

// TestTheConversionScaleFollowsTheXGRepair is the ordering pin, and it guards a
// sharper version of the trap `repaired()` already documents.
//
// The scale is fitted ON xG. Computing it inside `fetch`, or serialising it, would
// bake a scale fitted on the UNREPAIRED archive into the cache — and then
// FPL_NO_XG_REPAIR=1 would compare a repaired arm against an unrepaired one through
// the SAME scale. The escape hatch would be a partial no-op that reads as a null,
// which is precisely the shape of the cache bug this package has now paid for
// three times: kickoff times, the starts harvest, and the xG repair itself.
func TestTheConversionScaleFollowsTheXGRepair(t *testing.T) {
	orig := aCalibratableSeason(aRepairedSeason)
	// A hole for the repair to fill, in a season the repair covers.
	g := orig.Players[300].GWs[5]
	g.XG = 0
	orig.Players[300].GWs[5] = g

	b, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// What is CACHED must carry no scale at all. If Conversion were serialised this
	// is where it would show, and every later cache hit would score through a scale
	// fitted before the repair ran.
	var cached Season
	if err := json.Unmarshal(b, &cached); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, p := range cached.Players {
		if p.Conversion != (analysis.ConversionScale{}) {
			t.Fatalf("the cached season carries a conversion scale (%+v); "+
				"Player.Conversion must be json:\"-\" — it is fitted after the "+
				"repair, so a cached copy is a scale from the wrong data state",
				p.Conversion)
		}
	}

	load := func(off bool) *Season {
		var s Season
		if err := json.Unmarshal(b, &s); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if off {
			t.Setenv("FPL_NO_XG_REPAIR", "1")
		} else {
			t.Setenv("FPL_NO_XG_REPAIR", "")
		}
		out, err := repaired(&s)
		if err != nil {
			t.Fatalf("repaired: %v", err)
		}
		return out
	}

	on, off := load(false), load(true)

	// Every player must come out of `repaired` with a scale, on both arms. A zero
	// here is the panic in XPointsResidual waiting to happen on a real replay.
	for _, s := range []*Season{on, off} {
		for _, p := range s.Players {
			if p.Conversion.Goals <= 0 || p.Conversion.Assists <= 0 {
				t.Fatalf("%s: player %d left `repaired` with scale %+v; every "+
					"loaded season must be calibrated", s.Name, p.ID, p.Conversion)
			}
		}
	}

	// # The ordering itself, asserted against the rows rather than against a value
	//
	// The scale a season leaves `repaired` with must be the scale its rows imply
	// AFTER the repair has run. If calibrateConversion moved above applyXGRepair,
	// the stored scale would be the pre-repair fit and this recomputation would
	// disagree with it.
	for _, s := range []*Season{on, off} {
		var goals, xg float64
		for _, p := range s.Players {
			if p.Type != 4 {
				continue
			}
			for gw := 1; gw <= 38; gw++ {
				g, ok := p.GWs[gw]
				if !ok || g.Minutes <= 0 {
					continue
				}
				goals += float64(g.Goals)
				xg += g.XG
			}
		}
		want := analysis.CalibrationRatio(goals, xg)
		if got := s.ConversionScales()[4].Goals; math.Abs(got-want) > 1e-12 {
			t.Errorf("%s: the forward scale is %v but this season's post-repair "+
				"rows imply %v; the scale was fitted against a different data "+
				"state than the one the instrument scores", s.Name, got, want)
		}
	}

	// # And whether this run could have detected the violation at all
	//
	// The xG repair fills from the Understat harvest, which is keyed on real player
	// codes — a synthetic season's players are not in it, so on this fixture the
	// repair cannot move any minutes-bearing xG row and the two arms are
	// legitimately identical. Say which case this run is rather than passing
	// silently, because a test that cannot fail is not a test.
	counted := func(s *Season) float64 {
		var xg float64
		for _, p := range s.Players {
			for gw := 1; gw <= 38; gw++ {
				if g, ok := p.GWs[gw]; ok && g.Minutes > 0 {
					xg += g.XG
				}
			}
		}
		return xg
	}
	if math.Abs(counted(on)-counted(off)) < 1e-12 {
		t.Skip("the repair moved no minutes-bearing xG on this fixture (it fills " +
			"from a harvest keyed on real player codes), so the two arms cannot " +
			"differ and only the recomputation above is load-bearing here")
	}
	if on.ConversionScales()[4] == off.ConversionScales()[4] {
		t.Errorf("the repair moved counted xG yet the forward scale is identical "+
			"with the switch on (%+v) and off (%+v); the scale is being fitted "+
			"before the repair runs",
			on.ConversionScales()[4], off.ConversionScales()[4])
	}
}
