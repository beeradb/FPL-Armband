package analysis

import (
	"math"
	"testing"

	"armband/internal/fpl"
)

// newRestFixture builds a synthetic two-club, two-player bootstrap for
// exercising the post-tournament rest factor deterministically.
//
// It cannot be built on roleEngine/playGameweeks (that helper always drives a
// LIVE engine, and this needs the rest window pinned to a specific
// gameweek on demand) or on the live API (this package's tests cannot load a
// prior season the way cmd/armband's live server does — see
// skipDuringLiveGW1Gap's own comment). partialGameweekEngine is the closest
// existing helper; this is its sibling for a different axis, whether GW1 has
// finished, rather than whether a fixture has kicked off mid-gameweek.
//
// gw1Finished selects which side of GW1 the engine sits on:
//   - false leaves the season pre-season — SeasonHasStarted() false,
//     NextEvent().ID == 1 — which is the state blendRatesCode's preSeasonRates
//     branch reads, and the one the rest factor's cited 0.93 (GW1) comes from.
//   - true marks GW1's own event Finished, so GameweeksPlayed() == 1 and
//     NextEvent().ID == 2 — still inside RestGameweeks (2), so the rest
//     factor is still live, but a no-prior player now takes the in-season
//     shrinkToLeague path instead of the pre-season one.
//
// Both players carry real minutes so a no-prior shrink has a nonzero raw
// figure to shrink FROM — a genuine zero-minute debutant would make
// GateMinutesPerMatch zero, and 0/f is 0 regardless of f, which would pass
// even on the bug this fixture exists to catch.
func newRestFixture(gw1Finished bool) (*fpl.Bootstrap, []fpl.Fixture) {
	established := fpl.Element{
		ID: 1, Code: 1, WebName: "Established", ElementType: 3, Team: 2,
		NowCost: 60, Status: "a", Minutes: 3000, Starts: 33,
	}
	debutant := fpl.Element{
		ID: 2, Code: 2, WebName: "Debutant", ElementType: 3, Team: 1,
		NowCost: 45, Status: "a", Minutes: 900, Starts: 10,
	}
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
		Elements: []fpl.Element{established, debutant},
	}
	for i := 1; i <= 38; i++ {
		b.Events = append(b.Events, fpl.Event{ID: i, Name: "Gameweek", Finished: gw1Finished && i == 1})
	}
	gw1 := 1
	fx := []fpl.Fixture{
		{
			ID: 1, Event: &gw1, TeamH: 1, TeamA: 2,
			Started: gw1Finished, Finished: gw1Finished, FinishedProvisional: gw1Finished,
		},
	}
	return b, fx
}

// restFixtureEngine builds one engine from newRestFixture, with restPlayers
// wired straight into Weights.RestPlayers in place of the real season's
// list — matched by WebName exactly as the real config is.
func restFixtureEngine(t *testing.T, gw1Finished bool, restPlayers []string) *Engine {
	t.Helper()
	b, fx := newRestFixture(gw1Finished)
	w := DefaultWeights()
	w.RestPlayers = restPlayers
	return NewEngineFull(b, fx, w, Congestion{}, RoleRisk{})
}

// assertRestFactorLive fails the test if the named player does not actually
// carry a live rest discount on e — the setup check every "on" arm below
// needs, since a name or window mismatch would otherwise silently degrade the
// test into comparing two identical arms.
func assertRestFactorLive(t *testing.T, e *Engine, el *fpl.Element) {
	t.Helper()
	if _, f := e.restFactor(el); f >= 1 {
		t.Fatalf("setup: restFactor is a no-op (%.3f) for %s — the rest window or "+
			"the name did not take", f, el.WebName)
	}
}

// TestAnOverriddenRestPlayerKeepsHisOverriddenSettledMinutes is the SettledMinutes
// half of a bug this package's own AGENTS.md names: the decision "was the rest
// factor applied?" used to be made twice, in two files, with two different
// conditions. blendForCode never applies the rest factor to an overridden
// player at all — a minutes correction from the analysis layer is a statement
// of fact, and blendForCode returns before it ever reaches the rest-factor
// block. But metrics.go used to re-derive "was it applied?" by calling
// restFactor a SECOND time, independently of what blendForCode actually did,
// and divided the override by that freshly-recomputed factor regardless —
// inflating a rest-listed, overridden player's eligibility floor even though
// no rest discount was ever applied to it in the first place.
func TestAnOverriddenRestPlayerKeepsHisOverriddenSettledMinutes(t *testing.T) {
	e := restFixtureEngine(t, false, []string{"Established"})
	established := &e.Boot.Elements[0]
	assertRestFactorLive(t, e, established)

	e.SetMinutesOverride(established.Code, 70, 0, true)

	m := e.Metrics(established)
	if m.SettledMinutes != 70 {
		t.Errorf("SettledMinutes = %.4f, want exactly 70 (the override) — a rest-listed, "+
			"overridden player must not have a rest factor blendForCode never applied to "+
			"him divided back out (measured ~75.1 on the pre-fix code)", m.SettledMinutes)
	}
	if m.ExpectedMinutes != 70 {
		t.Errorf("ExpectedMinutes = %.4f, want exactly 70 — the override should not move "+
			"either", m.ExpectedMinutes)
	}
}

// TestANoPriorRestPlayersFloorIsNotInflatedByTheRestUndo is the general,
// no-override population the bug above generalises to: a no-prior player
// shrinkToLeague routes to GateMinutesPerMatch, captured BEFORE the volume
// shrink and BEFORE blendForCode's rest-factor block. Before this fix,
// blendForCode multiplied the rest factor into MinutesPerMatch but never
// into GateMinutesPerMatch, so GateMinutesPerMatch carried NO rest discount
// at all — and metrics.go divided it by the factor anyway, inflating a
// rest-listed debutant's eligibility floor above what a non-rest-listed
// debutant with the identical underlying evidence gets.
//
// Paired liveness, required alongside the tie: a byte-identical SettledMinutes
// is not proof the rest condition did anything at all unless something else
// in the SAME arms is shown to move. ExpectedMinutes must differ, because the
// rest factor legitimately does discount the Score-side figure.
func TestANoPriorRestPlayersFloorIsNotInflatedByTheRestUndo(t *testing.T) {
	on := restFixtureEngine(t, false, []string{"Debutant"})
	off := restFixtureEngine(t, false, nil)
	on.Priors, off.Priors = fakePriors{}, fakePriors{} // nobody has a prior on either engine

	debutantOn := &on.Boot.Elements[1]
	debutantOff := &off.Boot.Elements[1]
	assertRestFactorLive(t, on, debutantOn)

	mOn := on.Metrics(debutantOn)
	mOff := off.Metrics(debutantOff)

	if mOff.SettledMinutes <= 0 {
		t.Fatalf("test is not exercising the bug: SettledMinutes is %.4f off the rest "+
			"list, want a nonzero raw figure to shrink from", mOff.SettledMinutes)
	}
	if diff := math.Abs(mOn.SettledMinutes - mOff.SettledMinutes); diff > 1e-9 {
		t.Errorf("SettledMinutes = %.4f rest-listed against %.4f off the rest list "+
			"(diff %.4f) — a no-prior player's eligibility floor must be byte-identical "+
			"whether or not he is rest-listed; the rest factor never reaches Score's "+
			"eligibility question, only how much he plays", mOn.SettledMinutes, mOff.SettledMinutes, diff)
	}
	// Liveness: the two arms must actually differ somewhere, or the tie above
	// proves nothing about the rest condition at all.
	if diff := math.Abs(mOn.ExpectedMinutes - mOff.ExpectedMinutes); diff < 0.5 {
		t.Errorf("ExpectedMinutes barely moved between the two arms (%.4f rest-listed vs "+
			"%.4f off, diff %.4f) — this test needs the rest condition to have a real "+
			"effect on SOMETHING to prove the SettledMinutes tie above is not vacuous",
			mOn.ExpectedMinutes, mOff.ExpectedMinutes, diff)
	}
}

// TestTheRestUndoStillRestoresAPlainRestedPlayer is the must-move guard on the
// two tests above: it stops "fix by deleting the undo" from passing them by
// accident. A player with a genuine prior season (GateMinutesSet stays
// false, so SettledMinutes reads straight off MinutesPerMatch) must have his
// rest discount undone exactly as before — SettledMinutes matches the
// rest-list-off figure — while ExpectedMinutes, the Score-side figure, still
// carries the discount and differs between the two arms. A change that simply
// deleted the divide-back-out in metrics.go would leave SettledMinutes
// rest-discounted on the "on" arm and pass the two tests above (which only
// exercise the gate-mismatch case), but fails here.
func TestTheRestUndoStillRestoresAPlainRestedPlayer(t *testing.T) {
	on := restFixtureEngine(t, false, []string{"Established"})
	off := restFixtureEngine(t, false, nil)

	// A real prior season, distinct from el.Minutes, so blendRatesCode takes
	// priorSeasonRates rather than the pre-season no-op branch — either way
	// GateMinutesSet stays false, but this keeps the figure clearly nonzero
	// and clearly not a pass-through of el.Minutes itself.
	prior := &PriorPlayer{Minutes: 2500, Starts: 30, XG: 5, XA: 3, XGC: 35, DefCon: 20, Bonus: 8}
	establishedOn := &on.Boot.Elements[0]
	establishedOff := &off.Boot.Elements[0]
	on.Priors = fakePriors{establishedOn.Code: prior}
	off.Priors = fakePriors{establishedOff.Code: prior}
	assertRestFactorLive(t, on, establishedOn)

	mOn := on.Metrics(establishedOn)
	mOff := off.Metrics(establishedOff)

	if mOff.SettledMinutes <= 0 {
		t.Fatalf("test is not exercising anything: SettledMinutes is %.4f off the rest list",
			mOff.SettledMinutes)
	}
	if diff := math.Abs(mOn.SettledMinutes - mOff.SettledMinutes); diff > 1e-9 {
		t.Errorf("SettledMinutes = %.4f rest-listed against %.4f off the rest list "+
			"(diff %.4f) — the rest undo must still restore a PLAIN rested player (no "+
			"gate divergence) to the same eligibility floor he would read off the rest "+
			"list", mOn.SettledMinutes, mOff.SettledMinutes, diff)
	}
	if diff := math.Abs(mOn.ExpectedMinutes - mOff.ExpectedMinutes); diff < 0.5 {
		t.Errorf("ExpectedMinutes barely moved between the two arms (%.4f rest-listed vs "+
			"%.4f off, diff %.4f) — a rest-listed player's SCORE-side minutes must still "+
			"carry the discount, or the SettledMinutes tie above just means the undo was "+
			"deleted rather than that it is working",
			mOn.ExpectedMinutes, mOff.ExpectedMinutes, diff)
	}
}
