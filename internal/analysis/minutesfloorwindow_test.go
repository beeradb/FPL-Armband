package analysis

import (
	"fmt"
	"testing"

	"armband/internal/fpl"
)

// floorWindowEngine builds a synthetic engine parked in the live GW1 gap —
// SeasonHasStarted true, GameweeksPlayed still 0 — with the three club states
// that coexist there, so each can be asked for its own minutes-floor window.
//
// It is deliberately not a live engine: the gap is a few days of a season, so a
// test that waited for the real calendar to enter it would assert nothing for
// the other fifty-one weeks and could never be run on demand.
//
//	team 1 — a completed match on the board
//	team 2 — a match kicked off and still being played
//	team 3 — has not kicked off at all
func floorWindowEngine(t *testing.T) *Engine {
	t.Helper()
	b := &fpl.Bootstrap{
		Season: "2026-27",
		Teams: []fpl.Team{
			{ID: 1, ShortName: "FIN", Strength: 3},
			{ID: 2, ShortName: "LIV", Strength: 3},
			{ID: 3, ShortName: "NIL", Strength: 3},
			{ID: 4, ShortName: "OPP", Strength: 3},
		},
		ElementTypes: []fpl.ElementType{
			{ID: 1, SingularNameShort: "GKP"}, {ID: 2, SingularNameShort: "DEF"},
			{ID: 3, SingularNameShort: "MID"}, {ID: 4, SingularNameShort: "FWD"},
		},
	}
	for i := 1; i <= GameweeksPerSeason; i++ {
		b.Events = append(b.Events, fpl.Event{ID: i, Name: "Gameweek"})
	}
	gw1 := 1
	fx := []fpl.Fixture{
		// Full time, numbers final.
		{ID: 1, Event: &gw1, TeamH: 1, TeamA: 4, Started: true, Finished: true, FinishedProvisional: true},
		// Kicked off, still in progress — its minutes are a partial count.
		{ID: 2, Event: &gw1, TeamH: 2, TeamA: 4, Started: true},
		// Team 3's own fixture has not begun. No Event.Finished is set anywhere,
		// which is what GameweeksPlayed reads, so the gameweek is still "unplayed"
		// while real football has already happened.
		{ID: 3, Event: &gw1, TeamH: 3, TeamA: 4},
	}
	e := NewEngineFull(b, fx, DefaultWeights(), Congestion{}, RoleRisk{})
	if !e.SeasonHasStarted() || e.GameweeksPlayed() != 0 {
		t.Fatalf("setup: SeasonHasStarted=%v GameweeksPlayed=%d, want true/0 — "+
			"this engine is not in the gap it exists to represent",
			e.SeasonHasStarted(), e.GameweeksPlayed())
	}
	return e
}

// TestTheMinutesFloorScalesOnTheClubsOwnMatches is the guard on the
// fplarmband.com production incident of 2026-08-22.
//
// The squad pool's minutes floor is written as a SEASON TOTAL (600) and has to
// be scaled to however much football FPL's aggregates currently cover. It read
// that window off DataWindow, which answers a full pre-season 38 for the whole
// multi-day span between GW1's first kickoff and its last final whistle —
// GameweeksPlayed only moves once a gameweek FINISHES. So the floor stayed at an
// unscaled 600 while FPL had already zeroed every player's season minutes, and
// every candidate in the game failed it.
//
// What survived the pool was only the floor's two exemptions: players with a
// standing minutes override, and players priced at or below BenchFodderPrice.
// The site's optimal squad then spent £71.5m of £100m, left £28.5m unspent, and
// started a £4.5m midfielder at 0.14 pts/gw — a solver working correctly on a
// pool that had already been gutted.
//
// matchesAvailable, the rate denominator, had been given a per-club override for
// exactly this window two commits earlier; this quantity had not. One quantity,
// two implementations, and only one of them fixed — this record's signature
// failure arriving by its usual route.
func TestTheMinutesFloorScalesOnTheClubsOwnMatches(t *testing.T) {
	e := floorWindowEngine(t)

	const seasonFloor = 600 // what cmd/armband's squad page asks for

	// One completed match: a thirty-eighth of the season, so a thirty-eighth of
	// the floor. A ninety-minute starter clears it; a player who did not get off
	// the bench does not, which is the floor still doing its job on real evidence.
	if got, want := e.ScaledMinMinutesFor(1, seasonFloor), seasonFloor/GameweeksPerSeason; got != want {
		t.Errorf("a club with one COMPLETED match scales the %d-minute season floor "+
			"to %d, want %d", seasonFloor, got, want)
	}

	// A match in progress is not a match's worth of evidence, it is a partial
	// one: el.Minutes for that club is whatever the live game has accumulated so
	// far. There is no complete sample to test, so the sample-size test cannot
	// run — the same distinction blend.go draws between TeamMatchesStarted and
	// TeamMatchesFinished, for the same reason.
	if got := e.ScaledMinMinutesFor(2, seasonFloor); got != 0 {
		t.Errorf("a club whose match is still being played scales the season floor "+
			"to %d, want 0 — a partial in-match minutes count is not a completed "+
			"match of evidence to screen against", got)
	}

	// And a club that has not kicked off has no current-season sample at all.
	// FPL zeroes the whole league's aggregates the moment the season's first ball
	// is kicked, not per club, so these players read zero minutes with nothing
	// behind them. Screening them on a season-total floor removes them for having
	// no record on a day nobody has one.
	if got := e.ScaledMinMinutesFor(3, seasonFloor); got != 0 {
		t.Errorf("a club that has not kicked off scales the season floor to %d, "+
			"want 0 — this is the half that leaves ten clubs unbuyable for days", got)
	}
}

// TestTheMinutesFloorIsUnchangedOutsideTheGap is the confinement half. The fix
// above must reach the live GW1 gap and nothing else: pre-season the aggregates
// really are last season's 38-match totals, and once a gameweek has finished
// DataWindow is honest again for everybody. A per-club window loosening the floor
// outside the gap would quietly stop screening mid-season, which is the failure
// the floor exists to prevent.
//
// It passes with the fix and without it, on purpose: a confinement check on a
// path the change cannot reach can only ever fail, so it confirms nothing on its
// own. Its pair is TestTheMinutesFloorScalesOnTheClubsOwnMatches above, which
// must move and does — all three of its assertions fail on the pre-fix code.
func TestTheMinutesFloorIsUnchangedOutsideTheGap(t *testing.T) {
	e := floorWindowEngine(t)
	const seasonFloor = 600

	// Pre-season: nothing has kicked off.
	for i := range e.Fixtures {
		e.Fixtures[i].Started = false
		e.Fixtures[i].Finished = false
		e.Fixtures[i].FinishedProvisional = false
	}
	if e.SeasonHasStarted() {
		t.Fatal("setup: the season still reads as started with no fixture kicked off")
	}
	for _, team := range []int{1, 2, 3} {
		if got := e.ScaledMinMinutesFor(team, seasonFloor); got != seasonFloor {
			t.Errorf("pre-season, club %d scales the %d-minute season floor to %d, "+
				"want %d unchanged — FPL still holds last season's totals",
				team, seasonFloor, got, seasonFloor)
		}
	}

	// And once a gameweek has actually finished, every club is back on the
	// season-wide window regardless of its own fixture list.
	for i := range e.Fixtures {
		e.Fixtures[i].Started = true
		e.Fixtures[i].Finished = true
		e.Fixtures[i].FinishedProvisional = true
	}
	e.Boot.Events[0].Finished = true
	if e.GameweeksPlayed() != 1 {
		t.Fatalf("setup: GameweeksPlayed = %d, want 1", e.GameweeksPlayed())
	}
	for _, team := range []int{1, 2, 3} {
		if got, want := e.ScaledMinMinutesFor(team, seasonFloor), seasonFloor/GameweeksPerSeason; got != want {
			t.Errorf("after GW1 finished, club %d scales the season floor to %d, "+
				"want %d — the window is DataWindow again", team, got, want)
		}
	}
}

// TestTheMinutesFloorIsResolvedForAnyClubID guards the repair's own failure
// mode, which runs in the same unsafe direction as the bug.
//
// Optimize memoises the floor per club. The first version of that pre-filled the
// map by walking `e.Boot.Teams`, and a map answers a key it has never seen with
// Go's zero value — a floor of ZERO, which `clearsMinutesFloor` reads as "any
// minutes at all clears this". The screen would have been silently off for that
// club's entire squad, which is what the incident looked like from the outside.
//
// So the floor must be a total function of the club id: computed on demand, not
// looked up in a table that can miss. `Boot.Teams` and `Boot.Elements` come from
// one payload, so the miss should be unreachable — which is precisely why
// nothing would have noticed it.
func TestTheMinutesFloorIsResolvedForAnyClubID(t *testing.T) {
	e := floorWindowEngine(t)
	const seasonFloor = 600
	const noSuchClub = 4242 // not in Boot.Teams

	// In the gap, an unknown club has no completed match, so zero is the honest
	// answer and matches every other club in that state.
	if got := e.ScaledMinMinutesFor(noSuchClub, seasonFloor); got != 0 {
		t.Errorf("in the gap, an unknown club scales the season floor to %d, want 0", got)
	}

	// Outside it, the answer must be the season-wide window and NOT a zero-value
	// miss — this is the assertion a pre-filled map fails.
	for i := range e.Fixtures {
		e.Fixtures[i].Started = false
		e.Fixtures[i].Finished = false
		e.Fixtures[i].FinishedProvisional = false
	}
	if got := e.ScaledMinMinutesFor(noSuchClub, seasonFloor); got != seasonFloor {
		t.Errorf("pre-season, an unknown club scales the %d-minute season floor to "+
			"%d, want %d — a zero here is not a conservative default, it switches "+
			"the screen off for everyone at that club", seasonFloor, got, seasonFloor)
	}
}

// squadFieldTier is one rung of the synthetic field below: a price and the
// quality that has to justify it, so the optimiser has a real trade to make
// rather than a field of clones it can fill from either end.
type squadFieldTier struct {
	price   int // tenths of a million
	xg, xa  float64
	xgc     float64
	perTeam int
}

// partialGameweekField builds a whole synthetic league — twenty clubs, fifteen
// players each — parked in the live GW1 gap, and returns it with the set of club
// ids whose fixture has been played out in full.
//
// # Why this is not the live bootstrap
//
// It was written against the live feed first, and that was wrong twice over. The
// gap is a few days of a season, so a live test asserts nothing for the rest of
// it. And the live feed INSIDE the gap has already been zeroed by FPL, so the
// "last season" totals a test would reach for to build a prior are gone — the
// first version of this helper read them off the bootstrap and silently got
// zeroes, which made the whole field worthless for reasons that had nothing to
// do with what is under test.
//
// So: ten clubs play GW1 out in full, ten have not kicked off, no Event is
// marked finished anywhere, and every player's season aggregate is zero except a
// ninety-minute appearance for the clubs that have played. Last season is handed
// back through the engine's Priors hook, which is what cmd/armband does with
// internal/priors — this package cannot import that without a cycle.
func partialGameweekField(t *testing.T) (*Engine, map[int]bool) {
	t.Helper()

	const teams = 20
	// Fifteen players a club, matching the squad quota, so a legal squad can be
	// built from whatever subset of clubs the pool admits. The tiers give the
	// optimiser something to spend money ON: without a premium rung it can fill
	// fifteen slots out of petty cash and leave the budget unspent for entirely
	// legitimate reasons, which would make the budget assertion below prove
	// nothing.
	tiers := map[int][]squadFieldTier{
		1: {{price: 55, xgc: 38, perTeam: 1}, {price: 40, xgc: 60, perTeam: 1}},
		2: {{price: 60, xg: 4, xa: 5, xgc: 38, perTeam: 1}, {price: 45, xg: 2, xa: 2, xgc: 48, perTeam: 2},
			{price: 40, xg: 1, xa: 1, xgc: 60, perTeam: 2}},
		3: {{price: 120, xg: 18, xa: 12, perTeam: 1}, {price: 65, xg: 7, xa: 6, perTeam: 2},
			{price: 45, xg: 2, xa: 2, perTeam: 2}},
		4: {{price: 90, xg: 20, xa: 5, perTeam: 1}, {price: 60, xg: 9, xa: 4, perTeam: 1},
			{price: 45, xg: 3, xa: 2, perTeam: 1}},
	}

	b := &fpl.Bootstrap{
		Season: "2026-27",
		ElementTypes: []fpl.ElementType{
			{ID: 1, SingularNameShort: "GKP"}, {ID: 2, SingularNameShort: "DEF"},
			{ID: 3, SingularNameShort: "MID"}, {ID: 4, SingularNameShort: "FWD"},
		},
	}
	for i := 1; i <= teams; i++ {
		b.Teams = append(b.Teams, fpl.Team{
			ID: i, Name: fmt.Sprintf("Club %02d", i), ShortName: fmt.Sprintf("C%02d", i),
			Strength: 3,
		})
	}
	for i := 1; i <= GameweeksPerSeason; i++ {
		b.Events = append(b.Events, fpl.Event{ID: i, Name: "Gameweek", IsNext: i == 1})
	}

	priors := fakePriors{}
	id := 0
	for team := 1; team <= teams; team++ {
		for pos := 1; pos <= 4; pos++ {
			for _, tier := range tiers[pos] {
				for n := 0; n < tier.perTeam; n++ {
					id++
					b.Elements = append(b.Elements, fpl.Element{
						ID: id, Code: id, Team: team, ElementType: pos,
						WebName: fmt.Sprintf("C%02dP%03d", team, id),
						NowCost: tier.price, Status: "a",
					})
					// An ever-present last season, at the quality his price claims.
					priors[id] = &PriorPlayer{
						Minutes: 3200, Starts: 36,
						XG: tier.xg, XA: tier.xa, XGC: tier.xgc,
					}
				}
			}
		}
	}

	// A fixture list long enough for the scoring horizon to have something to
	// price. Clubs are paired the same way every gameweek, which is not a real
	// calendar but is a uniform one — every club faces the same difficulty, so
	// nothing here separates players by fixture.
	var fx []fpl.Fixture
	fid := 0
	for gw := 1; gw <= 10; gw++ {
		ev := gw
		for h := 1; h <= teams; h += 2 {
			fid++
			fx = append(fx, fpl.Fixture{
				ID: fid, Event: &ev, TeamH: h, TeamA: h + 1,
				TeamHDifficulty: 3, TeamADifficulty: 3,
			})
		}
	}

	// The gap itself: the first half of GW1 played out in full, the rest not
	// kicked off, and no Event marked finished anywhere — which is what
	// GameweeksPlayed reads, and why it answers 0 while real football has already
	// happened.
	played := map[int]bool{}
	for i := range fx {
		if fx[i].Event == nil || *fx[i].Event != 1 || fx[i].TeamH > teams/2 {
			continue
		}
		fx[i].Started, fx[i].Finished, fx[i].FinishedProvisional = true, true, true
		played[fx[i].TeamH], played[fx[i].TeamA] = true, true
	}

	// FPL zeroes the whole league's aggregates at the season's first kickoff, not
	// per club — which is why the clubs that have NOT played read zero minutes
	// rather than last season's total.
	for i := range b.Elements {
		el := &b.Elements[i]
		if played[el.Team] {
			el.Minutes, el.Starts = 90, 1
		}
	}

	e := NewEngineFull(b, fx, DefaultWeights(), Congestion{}, RoleRisk{})
	e.Priors = priors
	if !e.SeasonHasStarted() || e.GameweeksPlayed() != 0 {
		t.Fatalf("setup: SeasonHasStarted=%v GameweeksPlayed=%d, want true/0",
			e.SeasonHasStarted(), e.GameweeksPlayed())
	}
	if len(played) != teams/2 {
		t.Fatalf("setup: %d clubs have played, want %d", len(played), teams/2)
	}
	return e, played
}

// TestTheSquadPoolSurvivesThePartialGameweek is the end-to-end half: the unit
// tests above pin the window, this one pins what the window is for. It runs the
// request cmd/armband/page.go runs, against a full-sized field parked in the
// gap, and asserts the two things the production incident showed.
//
// Before the fix both assertions fail on this field, for the reason the incident
// showed: the floor stays at an unscaled 600, every candidate's fresh-season
// minutes count fails it, and the only survivors are the floor's two exemptions
// — a standing minutes override (this field has none) and a price at or below
// BenchFodderPrice. The optimiser then spends what cheap bodies cost and holds
// the rest of the budget as cash, because there is nothing left in the pool to
// convert it into.
//
// Neither assertion names a player or a score.
func TestTheSquadPoolSurvivesThePartialGameweek(t *testing.T) {
	e, played := partialGameweekField(t)

	// The request cmd/armband/page.go builds for the squad page.
	req := OptimizeRequest{Budget: DefaultBudget, MinMinutes: 600, MinExpectedMinutes: 55}

	// Candidates who are not exempt from the floor: above the bench-fodder price,
	// and with no standing minutes override (this field has none).
	var fromPlayed, fromUnplayed int
	for _, m := range e.AllMetrics() {
		if m.Price <= BenchFodderPrice || !e.ReachesExpectedMinutesCut(m, req) {
			continue
		}
		if played[m.TeamID] {
			fromPlayed++
		} else {
			fromUnplayed++
		}
	}
	if fromPlayed == 0 {
		t.Error("no candidate above the bench-fodder price from a club that has " +
			"already played its GW1 fixture reaches the pool; a season-total minutes " +
			"floor is being compared against a fresh-season minutes count")
	}
	if fromUnplayed == 0 {
		t.Error("no candidate above the bench-fodder price from a club that has NOT " +
			"kicked off reaches the pool; FPL zeroes the season aggregates " +
			"league-wide at the first kickoff, so screening these players on a " +
			"minutes floor removes half the league for having no record on a day " +
			"nobody has one")
	}

	sq, err := e.Optimize(req)
	if err != nil {
		t.Fatalf("Optimize during the partial gameweek: %v", err)
	}
	spent := 0
	for _, p := range sq.Players {
		spent += int(p.Price*10 + 0.5)
	}
	// The incident's own symptom, and the reason it was noticed at all: with the
	// pool gutted there was nothing left worth buying, so £28.5m of £100m went
	// unspent. This field has a premium rung in every outfield position, so a
	// populated pool always has somewhere better to put the money than cash.
	if spent < DefaultBudget*95/100 {
		t.Errorf("the optimal squad spent £%.1fm of £%.1fm and left £%.1fm unspent; "+
			"this field has premiums to buy, so unspent budget means the pool was "+
			"empty rather than the solver declining",
			float64(spent)/10, float64(DefaultBudget)/10, float64(DefaultBudget-spent)/10)
	}
}
