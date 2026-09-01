package analysis

import (
	"math"
	"testing"

	"armband/internal/fpl"
)

// This file guards the fix that moved saves, defensive contribution, bonus
// and cards from a MINUTES gate to a RATE gate in baseXP90 and in the
// defensive-contribution threshold block that follows it.
//
// blend.go supplies real evidence for those four terms at el.Minutes == 0 —
// a prior season's rate, or the position's league average — precisely for
// the population (summer signings, promoted-club regulars, a rotated-out
// established player) that has no CURRENT-season minutes yet. Gating on
// el.Minutes discarded that evidence outright rather than reading it, and
// produced a score that snapped from missing the whole block to carrying it
// in full at a player's very first recorded minute.

// basexp90TestEngine builds a small, self-contained, network-free engine one
// gameweek into a season — SeasonHasStarted true, GameweeksPlayed 1 — so
// blendRatesCode takes the established-prior mix branch on el.Minutes alone,
// never the pre-season or no-prior fallback. Modelled on
// partialGameweekEngine and floorWindowEngine (blend_test.go,
// minutesfloorwindow_test.go), which are the package's standing pattern for
// a controlled in-season state that does not depend on the live API or on
// whichever gameweek happens to be current when the suite runs.
func basexp90TestEngine(t *testing.T) *Engine {
	t.Helper()
	b := &fpl.Bootstrap{
		Season: "2026-27",
		Teams: []fpl.Team{
			{ID: 1, ShortName: "AAA", Strength: 3},
			{ID: 2, ShortName: "BBB", Strength: 3},
		},
		ElementTypes: []fpl.ElementType{
			{ID: 1, SingularNameShort: "GKP"}, {ID: 2, SingularNameShort: "DEF"},
			{ID: 3, SingularNameShort: "MID"}, {ID: 4, SingularNameShort: "FWD"},
		},
	}
	for i := 1; i <= GameweeksPerSeason; i++ {
		b.Events = append(b.Events, fpl.Event{ID: i, Name: "Gameweek", Finished: i == 1})
	}
	gw1 := 1
	fx := []fpl.Fixture{
		{ID: 1, Event: &gw1, TeamH: 1, TeamA: 2, Started: true, Finished: true, FinishedProvisional: true},
	}
	e := NewEngineFull(b, fx, DefaultWeights(), Congestion{}, RoleRisk{})
	if !e.SeasonHasStarted() || e.GameweeksPlayed() != 1 {
		t.Fatalf("setup: SeasonHasStarted=%v GameweeksPlayed=%d, want true/1 — "+
			"this engine is not in the in-season state it exists to represent",
			e.SeasonHasStarted(), e.GameweeksPlayed())
	}
	return e
}

func addElement(e *Engine, pos, team int, minutes, starts int) *fpl.Element {
	code := len(e.Boot.Elements) + 1
	e.Boot.Elements = append(e.Boot.Elements, fpl.Element{
		ID: code, Code: code, ElementType: pos, Team: team,
		WebName: "p", NowCost: 50, Status: "a",
		Minutes: minutes, Starts: starts,
	})
	e.calibrateLeagueRates()
	return &e.Boot.Elements[len(e.Boot.Elements)-1]
}

// TestBaseXP90HasNoDiscontinuityAtOneMinute is the first-minute snap the fix
// closes: a player scored at Minutes:0 and at Minutes:1, with an identical,
// substantial prior behind him both times. The gated terms are supplied by
// the blend either way — n90 is 0 or 1/90, a negligible difference in the
// mix weight against BlendRateK — so BaseXP90 should barely move. Pre-fix it
// moved by the whole defcon+bonus+cards block, because that block was
// wholly absent at Minutes:0 and wholly present at Minutes:1.
func TestBaseXP90HasNoDiscontinuityAtOneMinute(t *testing.T) {
	e := basexp90TestEngine(t)
	def := addElement(e, 2, 1, 0, 0)
	e.Priors = fakePriors{def.Code: {
		Minutes: 2500, Starts: 28, XG: 1, XA: 2, XGC: 35,
		DefCon: 250, Bonus: 25, Yellow: 8,
	}}

	def.Minutes, def.Starts = 0, 0
	m0 := e.Metrics(def)

	def.Minutes, def.Starts = 1, 0
	m1 := e.Metrics(def)

	// What the OLD, minutes-gated behaviour jumped by: the whole gated block,
	// wholly absent at zero minutes and wholly present at one.
	block := defconPer90(2, m0.DefCon90) + m0.Bonus90*e.bonusWeightFor(def) -
		m0.Yellow90*yellowCardPoints - m0.Red90*redCardPoints
	if block < 0.1 {
		t.Fatalf("test setup: the gated block is only %.4f, too small to tell a fix "+
			"from the bug it replaces", block)
	}

	delta := math.Abs(m1.BaseXP90 - m0.BaseXP90)
	t.Logf("BaseXP90 at 0 minutes: %.4f, at 1 minute: %.4f, gated block: %.4f",
		m0.BaseXP90, m1.BaseXP90, block)
	if delta > 0.1*block {
		t.Errorf("BaseXP90 moved %.4f between zero and one minute, %.0f%% of the "+
			"%.4f defcon+bonus+cards block — a single extra minute must not flip "+
			"that block from wholly absent to wholly present", delta, 100*delta/block, block)
	}
}

// TestZeroMinuteKeeperSavesMirrorFixtureSensitivity is the mirror-invariant
// half of the fix: fixtureSensitiveAt has never gated saves on minutes (it
// cannot — it is not given an *fpl.Element at all), so before this fix a
// zero-minute keeper's saves term was present in fixtureSensitiveAt's
// neutral evaluation but absent from BaseXP90, and fixtureAdjustedXP90's
// "fixture-insensitive remainder" (base minus that neutral evaluation) came
// out systematically short by exactly the missing saves term.
//
// ⚠️ This is NOT tested at literally neutral (atk=1, def=1) fixtures. At
// neutral difficulty fixtureAdjustedXP90's own algebra —
// adjusted + (base - fixtureSensitiveAt(neutral)) — collapses to exactly
// `base` regardless of what base is, since the "adjusted" side is then
// identical to the very term being subtracted back out. That identity holds
// whether or not the saves term is missing from base, so it cannot tell the
// bug from the fix: it was verified to pass unchanged on the pre-fix code.
// A mixed run of fixture difficulty — the same pattern
// TestPerFixtureRaisesTheCleanSheetOnAMixedRun and
// TestThresholdAndAdjustedUseTheSameFixtureRule use — is where the two
// actually disagree, by exactly the missing saves term at neutral
// difficulty.
func TestZeroMinuteKeeperSavesMirrorFixtureSensitivity(t *testing.T) {
	e := basexp90TestEngine(t)
	gk := addElement(e, 1, 1, 0, 0)
	// DefCon, Bonus, Yellow and Red all left at zero so this keeper's
	// fixture-INSENSITIVE remainder is exactly zero, isolating the
	// fixture-SENSITIVE saves term the mirror invariant is actually about.
	e.Priors = fakePriors{gk.Code: {Minutes: 3400, Starts: 38, XGC: 40, Saves: 140}}

	m := e.Metrics(gk)
	if m.Saves90 <= 0 {
		t.Fatal("test setup: no blended save rate at zero minutes")
	}
	if m.DefCon90 != 0 || m.Bonus90 != 0 || m.Yellow90 != 0 || m.Red90 != 0 {
		t.Fatalf("test setup: fixture-insensitive remainder is not zero "+
			"(defcon %.4f bonus %.4f yellow %.4f red %.4f)",
			m.DefCon90, m.Bonus90, m.Yellow90, m.Red90)
	}

	fx := scoredFixtures(1, 5, 1, 5, 3)
	got := e.fixtureAdjustedXP90(gk, m, fx)
	// The independently-computed ground truth. fixtureAdjustedXP90 blends
	// `base` with `adjusted` by FixtureWeight; `adjusted` is the per-fixture
	// average PLUS the fixture-insensitive remainder, and the remainder is
	// zero here by construction, so `adjusted` is exactly
	// perFixtureSensitive — the same helper
	// TestPerFixtureRaisesTheCleanSheetOnAMixedRun uses, built from
	// fixtureSensitiveAt directly and therefore never gated on minutes.
	base := m.BaseXP90 + m.SetPieceXP90
	w := clamp(e.Weights.FixtureWeight, 0, 1)
	want := base*(1-w) + perFixtureSensitive(e, m, 1, fx)*w

	if math.Abs(got-want) > 1e-9 {
		t.Errorf("fixtureAdjustedXP90 = %.4f, want %.4f — off by %.4f, which is "+
			"FixtureWeight (%.2f) times poissonFloorDiv(savesBlock, Saves90) = %.4f: "+
			"the saves term the remainder subtracted out but BaseXP90 never had",
			got, want, want-got, w, poissonFloorDiv(savesBlock, m.Saves90))
	}
}

// TestZeroMinuteDefenderBaseXPIncludesBlendedDefconAndBonus is the
// "unreachable branch" the fix reopens: an in-season defender with real
// current-season minutes of zero but a real prior season, whose blended
// DefCon90 and Bonus90 are both positive — evidence baseXP90 used to be
// unable to reach at all.
func TestZeroMinuteDefenderBaseXPIncludesBlendedDefconAndBonus(t *testing.T) {
	e := basexp90TestEngine(t)
	def := addElement(e, 2, 1, 0, 0)
	e.Priors = fakePriors{def.Code: {Minutes: 2500, Starts: 28, DefCon: 250, Bonus: 25}}

	m := e.Metrics(def)
	if m.DefCon90 <= 0 || m.Bonus90 <= 0 {
		t.Fatalf("test setup: zero-minute defender's blended DefCon90 (%.4f) or "+
			"Bonus90 (%.4f) is not positive", m.DefCon90, m.Bonus90)
	}

	wantDefcon := defconPer90(2, m.DefCon90)
	wantBonus := m.Bonus90 * e.bonusWeightFor(def)

	bare := m
	bare.DefCon90, bare.Bonus90 = 0, 0
	bareBase := e.baseXP90(def, bare)

	delta := m.BaseXP90 - bareBase
	if math.Abs(delta-(wantDefcon+wantBonus)) > 1e-9 {
		t.Errorf("zero-minute defender's BaseXP90 carries %.4f of defcon+bonus, want "+
			"%.4f (defcon %.4f + bonus %.4f) — the blended evidence the model has for "+
			"this player is not reaching BaseXP90", delta, wantDefcon+wantBonus, wantDefcon, wantBonus)
	}
}

// TestPreSeasonNoPriorZeroMinutesGetsLeagueFallbackInBaseXP90 is the
// pre-season sibling of TestAShareOfZeroReproducesTheOldZeroMinutes
// (unknownprior_test.go): a player the archive has never seen — no prior
// season, no current-season minutes — takes the position's league-average
// defcon, bonus and saves fallback via shrinkToLeague, and that fallback
// must reach BaseXP90, not stop at the blend.
func TestPreSeasonNoPriorZeroMinutesGetsLeagueFallbackInBaseXP90(t *testing.T) {
	b := &fpl.Bootstrap{
		Season: "2026-27",
		ElementTypes: []fpl.ElementType{
			{ID: 2, SingularNameShort: "DEF"},
		},
		Events: []fpl.Event{{ID: 1, Name: "Gameweek", IsNext: true}},
		Teams:  []fpl.Team{{ID: 1, Name: "Club", ShortName: "CLB", Strength: 3}},
	}
	for i := 0; i < 8; i++ {
		mins, bonus, defcon := 2500, 20, 200
		if i == 0 {
			mins, bonus, defcon = 0, 0, 0 // the unknown: no history at all
		}
		b.Elements = append(b.Elements, fpl.Element{
			ID: i + 1, Code: i + 1, ElementType: 2, Team: 1,
			WebName: "p", NowCost: 45 + 5*i, Status: "a",
			Minutes: mins, Starts: mins / 90,
			Bonus: bonus, DefensiveContribution: defcon,
		})
	}

	e := NewEngine(b, nil, DefaultWeights())
	e.Priors = stubPriors{}
	if e.SeasonHasStarted() {
		t.Fatal("test setup: SeasonHasStarted() is true; this test needs pre-season")
	}

	unknown := e.Boot.ElementByID(1)
	m := e.Metrics(unknown)
	if m.DefCon90 <= 0 {
		t.Errorf("pre-season no-prior player's blended DefCon90 is %v, want the "+
			"position's nonzero league-average fallback", m.DefCon90)
	}
	if m.Bonus90 <= 0 {
		t.Errorf("pre-season no-prior player's blended Bonus90 is %v, want the "+
			"position's nonzero league-average fallback", m.Bonus90)
	}

	bare := m
	bare.DefCon90, bare.Bonus90 = 0, 0
	bareBase := e.baseXP90(unknown, bare)
	if m.BaseXP90 <= bareBase {
		t.Errorf("BaseXP90 (%.4f) is no higher than with the defcon/bonus fallback "+
			"zeroed (%.4f) — the league-average fallback is not reaching BaseXP90",
			m.BaseXP90, bareBase)
	}
}

// TestDefConIsNotDoubleCountedAtZeroMinutes guards the seam between the two
// gate sites this fix removes: baseXP90 adds defconPer90(pos, DefCon90) into
// the per-90 rate, and the threshold-split block subtracts that same term
// back out and replaces it with defconChance's exposure-corrected
// per-gameweek figure. That subtraction and re-addition only cancel
// correctly if BOTH gates move together — fixing one site and not the other
// either double-counts the defcon term (subtraction skipped, so the raw
// per-90 figure survives into `rate` alongside the new perGW term) or
// subtracts a term that was never added (baseXP90's gate still in place
// while the threshold block's is not).
//
// DefConCleanCoupling is forced to 0 so the clean-sheet term — which also
// reads DefCon90 when the coupling is nonzero — cannot leak into the
// comparison; the two Score readings this test diffs must differ by the
// defensive-contribution channel alone.
func TestDefConIsNotDoubleCountedAtZeroMinutes(t *testing.T) {
	e := basexp90TestEngine(t)
	e.Weights.DefConCleanCoupling = 0
	def := addElement(e, 2, 1, 0, 0)

	e.Priors = fakePriors{def.Code: {Minutes: 2500, Starts: 28, DefCon: 250}}
	withDefcon := e.Metrics(def)
	if withDefcon.DefCon90 <= 0 {
		t.Fatal("test setup: zero-minute defender's blended DefCon90 is not positive")
	}
	if withDefcon.DefConChance == nil {
		t.Fatal("test setup: DefConChance is nil — the threshold-split gate is still " +
			"in place, so this test cannot isolate the channel it is checking")
	}
	chance := *withDefcon.DefConChance

	e.Priors = fakePriors{def.Code: {Minutes: 2500, Starts: 28, DefCon: 0}}
	withoutDefcon := e.Metrics(def)

	// If both gates moved together, the defcon channel's ENTIRE effect on
	// Score runs through perGW (the per-90 contribution cancels by
	// construction in `rate`), scaled by the same congestion/role/
	// availability multipliers Score applies to everything else.
	want := chance * defConPoints *
		withDefcon.Congestion * withDefcon.RoleFactor * withDefcon.AvailabilityFactor

	got := withDefcon.Score - withoutDefcon.Score
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("zeroing DefCon90 moved Score by %.6f, want exactly %.6f (the single, "+
			"exposure-corrected defconChance contribution) — a difference here means the "+
			"defcon term is counted through both the per-90 rate AND the per-gameweek "+
			"threshold figure, or through neither", got, want)
	}
}
