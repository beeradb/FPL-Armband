package analysis

import (
	"math"
	"testing"

	"armband/internal/fpl"
)

// fakePriors stands in for a loaded season so the blend can be tested without
// the network.
type fakePriors map[int]*PriorPlayer

func (f fakePriors) Get(code int) (*PriorPlayer, bool) { p, ok := f[code]; return p, ok }

// TestBlendIsANoOpPreSeason — before GW1, FPL's aggregates *are* last season, so
// there is nothing to shrink toward and blending would double-count it.
func TestBlendIsANoOpPreSeason(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())

	// ⚠️ A MATCHING prior, not an empty map, and the difference is the whole test.
	//
	// Pre-season FPL has not reset its aggregates, so what the bootstrap carries
	// IS last season -- which means any player with real minutes necessarily HAS
	// a prior, and it necessarily agrees with the bootstrap. That is the state
	// this test is named for, and preSeasonRates' third branch ("a prior
	// identical to what FPL carries -> change nothing") is the no-op it asserts.
	//
	// ⚠️ It used to pass `fakePriors{}`, an EMPTY map, which does not mean
	// "priors are loaded" -- it means NOBODY has a prior, which routes every
	// player down the unknown-prior path instead. That path legitimately shrinks
	// toward the position's league average (UnknownPriorShare, shipped 558806af
	// on 2026-08-28), so the test read 0.822 and looked like an engine defect.
	// Measured on the committed capture: empty prior gives PriorWeight 0.822 and
	// 50.91 expected minutes; a matching prior gives 1.000 and 87.63; a genuinely
	// unknown player -- no prior AND no aggregates -- gives 0.000 and 50.91,
	// which is the fallback working as intended.
	//
	// The engine was right. The setup described a state that cannot occur.
	//
	// It passed on its original 2026-08-16 shape only because the unknown-prior
	// path did not exist yet, so an empty map also fell through to "change
	// nothing". When that path shipped the empty map started meaning something
	// else, and this should have failed -- but its selection needs
	// `el.Minutes >= 900`, true only pre-season, and by then GW1 had reset the
	// aggregates and it had already gone dark. It has asserted nothing since.
	priors := fakePriors{}
	for i := range e.Boot.Elements {
		el := &e.Boot.Elements[i]
		if el.Minutes > 0 {
			priors[el.Code] = &PriorPlayer{Minutes: el.Minutes, Starts: el.Starts}
		}
	}
	e.Priors = priors

	var checked int
	for i := range e.Boot.Elements {
		el := &e.Boot.Elements[i]
		if el.Minutes < 900 {
			continue
		}
		checked++
		if m := e.Metrics(el); m.PriorWeight != 1 {
			t.Fatalf("%s has prior weight %.3f pre-season, want 1", el.WebName, m.PriorWeight)
		}
	}
	if checked == 0 {
		t.Fatal("no players with 900+ minutes in the committed pre-season capture; " +
			"the capture pin is wrong, and skipping here is how this test went " +
			"dark for a fortnight")
	}
}

// TestBlendHoldsTheLineEarlyInTheSeason is the point of the exercise.
//
// After one gameweek FPL knows one match. An established starter who is rested
// for it must not collapse to fringe, and a squad player who happens to start
// must not be promoted to nailed. Last season is the only thing standing between
// the model and that noise.
func TestBlendHoldsTheLineEarlyInTheSeason(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	el := findEverPresent(t, e)
	mins, starts := el.Minutes, el.Starts
	defer func() { el.Minutes, el.Starts = mins, starts }()

	// Last season he was an ever-present.
	e.Priors = fakePriors{el.Code: {Minutes: 3300, Starts: 37, XG: 8, XA: 6, XGC: 40, DefCon: 300}}

	playGameweeks(t, e, 2)
	el.Minutes, el.Starts = 30, 0 // two gameweeks, one brief cameo

	withPrior := e.Metrics(el)
	e.Priors = nil
	without := e.Metrics(el)

	t.Logf("after 2 GWs with 30 minutes: expected minutes %.1f with a prior, %.1f without",
		withPrior.ExpectedMinutes, without.ExpectedMinutes)

	if withPrior.ExpectedMinutes <= without.ExpectedMinutes {
		t.Error("the prior did not lift an established starter after a quiet start")
	}
	if withPrior.ExpectedMinutes < 40 {
		t.Errorf("an ever-present is down to %.1f expected minutes after two gameweeks; "+
			"the prior is not holding", withPrior.ExpectedMinutes)
	}
	if withPrior.PriorWeight >= 0.5 {
		t.Errorf("current-season weight is %.2f after two gameweeks, expected well under half",
			withPrior.PriorWeight)
	}
}

// TestBlendYieldsToTheSeason — the prior is a starting point, not an anchor. By
// the end of a season it must be almost entirely displaced, or a player who has
// genuinely lost his place would never be marked down.
func TestBlendYieldsToTheSeason(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	el := findEverPresent(t, e)
	mins, starts := el.Minutes, el.Starts
	defer func() { el.Minutes, el.Starts = mins, starts }()
	e.Priors = fakePriors{el.Code: {Minutes: 3300, Starts: 37, XG: 8, XA: 6, XGC: 40, DefCon: 300}}

	var last float64
	for _, gw := range []int{2, 5, 10, 20, 30} {
		playGameweeks(t, e, gw)
		el.Minutes, el.Starts = gw*10, 0 // dropped: ten minutes a week all season
		m := e.Metrics(el)
		t.Logf("GW%-3d weight on this season %.2f, expected minutes %.1f", gw, m.PriorWeight, m.ExpectedMinutes)
		if m.PriorWeight < last {
			t.Errorf("current-season weight fell from %.2f to %.2f at GW%d", last, m.PriorWeight, gw)
		}
		last = m.PriorWeight
	}
	playGameweeks(t, e, 30)
	el.Minutes, el.Starts = 300, 0
	if m := e.Metrics(el); m.ExpectedMinutes > 30 {
		t.Errorf("after 30 gameweeks at 10 minutes a week he still shows %.1f expected minutes; "+
			"the prior is anchoring rather than informing", m.ExpectedMinutes)
	}
}

// TestPriorlessPlayersShrinkToTheLeague — a promoted club's player or an arrival
// from abroad has no prior of his own, and scoring him on his own first few
// matches is how the replay ended up paying a four-point hit for a debutant.
//
// His rates AND his minutes shrink toward his position's league-wide figures.
// ⚠️ Minutes used to be left untouched here on the reasoning that ninety
// minutes in one appearance really does mean ninety minutes when he plays,
// and the minutes-reliability term already prices whether he plays again —
// but MinutesRating is a bare function of these same fields with no term of
// its own for sample size, so it inherited the identical false certainty
// rather than pricing it. Reproduced live 2026-08-23: HUL's McBurnie and
// Belloumi, one 90-minute debut each, projected at StartShare 1.000 and
// dominated the wildcard/free-hit builder. See shrinkToLeague's own comment.
func TestPriorlessPlayersShrinkToTheLeague(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	el := findEverPresent(t, e)
	mins, starts, bonus := el.Minutes, el.Starts, el.Bonus
	defer func() { el.Minutes, el.Starts, el.Bonus = mins, starts, bonus }()

	e.Priors = fakePriors{} // empty: nobody has a prior
	playGameweeks(t, e, 1)
	el.Minutes, el.Starts, el.Bonus = 90, 1, 3 // a debut, and three bonus points

	m := e.Metrics(el)
	if m.PriorWeight >= 0.5 {
		t.Errorf("one match carries weight %.2f; a debut is not half a season of evidence",
			m.PriorWeight)
	}
	// Raw, three bonus in ninety minutes reads as three bonus every gameweek.
	if m.Bonus90 > 1.0 {
		t.Errorf("bonus/90 is %.2f after a single three-bonus debut; the league rate "+
			"should have pulled it well under 1", m.Bonus90)
	}
	// The debut start is real evidence and is not discarded outright (a
	// mid-match sub would report far less than this), only kept from being
	// read as a guarantee of the same every week for the rest of the horizon.
	if m.ExpectedMinutes > 85 {
		t.Errorf("expected minutes %.1f after a single 90-minute debut with no prior — "+
			"want it pulled below the raw 90 toward the league average, not left reading "+
			"as a guaranteed nailed starter off one match", m.ExpectedMinutes)
	}
	if m.ExpectedMinutes < 20 {
		t.Errorf("expected minutes %.1f over-corrected a real 90-minute start almost to "+
			"nothing", m.ExpectedMinutes)
	}
}

// partialGameweekEngine builds a synthetic engine caught in the exact window
// GameweeksPlayed() cannot see: one club's fixture has kicked off, but no
// Event is Finished, because a Premier League gameweek spans days and this one
// has not finished yet. It cannot be built on roleEngine/playGameweeks — that
// helper always finishes a fixture and its event together — and it cannot be
// built on the live API either: skipDuringLiveGW1Gap in datawindow_test.go
// documents that this package's tests cannot load a prior season the way
// cmd/armband's live server does, so live data in this exact window proves
// nothing about the model.
func partialGameweekEngine(t *testing.T) (*Engine, *fpl.Element) {
	t.Helper()
	established := fpl.Element{
		ID: 1, Code: 1, WebName: "Established", ElementType: 3, Team: 2,
		NowCost: 60, Status: "a", Minutes: 3000, Starts: 33,
		ExpectedGoals: 8, ExpectedAssists: 6, ExpectedGoalsConceded: 40, Bonus: 10,
	}
	debutant := fpl.Element{
		ID: 2, Code: 2, WebName: "Debutant", ElementType: 3, Team: 1,
		NowCost: 45, Status: "a",
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
		b.Events = append(b.Events, fpl.Event{ID: i, Name: "Gameweek"})
	}
	gw1 := 1
	fx := []fpl.Fixture{
		// Kicked off and even finished — real football has happened — but no
		// Event.Finished is set anywhere, which is what GameweeksPlayed() reads.
		// That is the live GW1 gap: this fixture is done while the gameweek it
		// belongs to is not.
		{ID: 1, Event: &gw1, TeamH: 1, TeamA: 2, Started: true, Finished: true, FinishedProvisional: true},
	}
	e := NewEngineFull(b, fx, DefaultWeights(), Congestion{}, RoleRisk{})
	return e, &e.Boot.Elements[1]
}

// TestPlayersWhoHaveAlreadyPlayedShrinkDuringThePartialGameweek reproduces the
// live GW1 failure directly: GameweeksPlayed() reads 0 for days after real
// matches have happened, because it waits for the whole gameweek to finish
// rather than for a club's own fixture to start. A player with no prior of his
// own and a real cameo already on the board then hit blendForCode's
// "pre-season, nothing to blend against" branch and came back raw — his three
// bonus points in thirty minutes read as nine bonus a gameweek — rather than
// through shrinkToLeague, the same defect 1a6f0a3 fixed one call site over.
func TestPlayersWhoHaveAlreadyPlayedShrinkDuringThePartialGameweek(t *testing.T) {
	e, debutant := partialGameweekEngine(t)
	e.Priors = fakePriors{} // empty: nobody has a prior

	if e.GameweeksPlayed() != 0 {
		t.Fatalf("GameweeksPlayed() = %d, want 0 — this test is about the gap "+
			"where it is still 0 despite real football", e.GameweeksPlayed())
	}
	if !e.SeasonHasStarted() {
		t.Fatal("SeasonHasStarted() is false; the fixture setup is not exercising the gap")
	}

	debutant.Minutes, debutant.Starts, debutant.Bonus = 30, 1, 3 // a cameo, three bonus points
	m := e.Metrics(debutant)

	if m.PriorWeight >= 0.5 {
		t.Errorf("one cameo carries weight %.2f; a debut whose club just kicked off "+
			"is not half a season of evidence", m.PriorWeight)
	}
	// Raw, three bonus in thirty minutes reads as nine bonus every gameweek.
	// Shrunk toward the league rate it should read nowhere close.
	if m.Bonus90 > 2.0 {
		t.Errorf("bonus/90 is %.2f off a single thirty-minute cameo; the league rate "+
			"should have pulled it well below the raw 9.0 — SeasonHasStarted's gate "+
			"is not taking effect", m.Bonus90)
	}
}

// TestMinutesOverrideSurvivesThePriorsBlend is the fplarmband.com production
// incident of 2026-08-22: a player with a genuine prior season on record, given
// a standing minutes correction, must not have that correction diluted back
// toward his prior by the current/prior-season blend.
//
// blendRatesCode applies the override once, early, to seed b.MinutesPerMatch —
// then, for a player who HAS a prior (p.Minutes > 0, the branch shrinkToLeague
// does NOT cover), blended that already-corrected figure straight back against
// his prior season with weight n/(n+k), n = GameweeksPlayed(), and never
// reasserted the override afterward — unlike the pre-season branch a few lines
// above it, which already did. At n == 0 the weight is exactly 0 and the
// override vanished entirely.
//
// Live incident: Kinsky was corrected to a nailed 88 expected minutes ("De
// Zerbi has named him first choice"), but his own prior season was 630 backup
// minutes. During the exact window this test constructs — SeasonHasStarted
// true, GameweeksPlayed still 0, the multi-day gap between a gameweek's first
// kickoff and its last final whistle — the site reported 16.6 expected minutes
// (630/38) and "fringe" instead. A sibling player with NO prior of his own
// (van Ewijk, no Premier League minutes on record) was unaffected, because
// shrinkToLeague never touches minutes — only a player who has a real prior
// season took this path at all, which is why the bug was invisible on most
// overridden players and exact on this one.
func TestMinutesOverrideSurvivesThePriorsBlend(t *testing.T) {
	e, _ := partialGameweekEngine(t)
	established := &e.Boot.Elements[0]
	if e.GameweeksPlayed() != 0 || !e.SeasonHasStarted() {
		t.Fatalf("setup: GameweeksPlayed=%d SeasonHasStarted=%v, want 0/true",
			e.GameweeksPlayed(), e.SeasonHasStarted())
	}

	// A thin-minutes backup last season -- a real prior, but a low one, same
	// shape as Kinsky's 630.
	e.Priors = fakePriors{established.Code: {Minutes: 630, Starts: 7, XG: 1, XA: 1, XGC: 10, DefCon: 5}}
	established.Minutes, established.Starts = 0, 0 // nothing from this season yet

	// The analysis layer has corrected him: nailed now, 88 expected minutes.
	// Confirmed is deliberately false here — this test's "nailed" comes from
	// the real prior season alone, and must not silently start depending on
	// the analyst's confidence flag too.
	e.SetMinutesOverride(established.Code, 88, 6, false)

	m := e.Metrics(established)
	if m.ExpectedMinutes < 80 {
		t.Errorf("override says 88 expected minutes but Metrics reports %.1f — the "+
			"priors blend (weight n/(n+k), n = GameweeksPlayed() = %d) reverted "+
			"almost entirely to his 630-minute prior season (630/38 = %.1f)",
			m.ExpectedMinutes, e.GameweeksPlayed(), 630.0/38)
	}
	if m.RotationRisk != "nailed" {
		t.Errorf("rotation_risk = %q with an 88-minute override in force, want nailed", m.RotationRisk)
	}
}

// TestSingleCameoDoesNotReadNailed is the fplarmband.com defect this is the
// regression test for: rotationLabel used to be a flat threshold on the point
// estimate alone, so a promoted-club debutant's one match — ninety minutes
// divided by the one match his club has played so far — read exactly like a
// season of established starts. No override and no prior season are involved
// here at all: this is the organic, no-override population the fix's evidence
// gate exists for.
func TestSingleCameoDoesNotReadNailed(t *testing.T) {
	e, debutant := partialGameweekEngine(t)
	e.Priors = fakePriors{} // no prior season for anybody

	debutant.Minutes, debutant.Starts = 90, 1 // his club's one match, played in full

	m := e.Metrics(debutant)
	if m.ExpectedMinutes < 75 {
		t.Fatalf("test is not exercising the failure: expected minutes %.1f, want >= 75 "+
			"(one full match against one match available)", m.ExpectedMinutes)
	}
	if m.RotationRisk == "nailed" {
		t.Errorf("rotation_risk = nailed off a single 90-minute cameo with no prior season "+
			"and no override — expected minutes (%.1f) cleared the threshold on one match's "+
			"worth of raw evidence, exactly the failure this label is supposed to catch",
			m.ExpectedMinutes)
	}
}

// TestOverrideConfirmedGatesNailed is the Tzolis production incident this
// field exists to fix, plus the Kinsky/van Ewijk contrast the fix must not
// regress.
//
// The PREVIOUS mechanism (nailedOverrideFloor, now deleted) inferred
// confidence from ExpectedMinutes' own magnitude: >= 80 read as "confident",
// on the theory that a hedge and a confident assertion cluster at different
// values. That held for the six overrides on file when it was written — until
// Tzolis shipped in the real 2026-27 config at 82, with a reason that reads
// "Set to 82 rather than a nailed 85 as this is still only his second
// competitive appearance for the club": an explicit hedge, at a value the
// floor still waved through, because RosterOverride carried no field that let
// the model tell "82, asserted as settled" from "82, hedged". A magnitude can
// never carry that distinction — only an explicit Confirmed field can, which
// is what this test pins: the SAME value (82, and separately 88, matching
// Kinsky) must read nailed or not purely off Confirmed, never off the number.
func TestOverrideConfirmedGatesNailed(t *testing.T) {
	tests := []struct {
		name       string
		overrideAt float64
		confirmed  bool
		wantNailed bool
	}{
		{"Tzolis: hedged reason, 82, unconfirmed — must not read nailed", 82, false, false},
		{"Tzolis' own value WOULD clear the old magnitude floor, but Confirmed wins", 82, false, false},
		{"Kinsky: confirmed at 88 reads nailed", 88, true, true},
		{"van Ewijk: confirmed at 85 reads nailed", 85, true, true},
		{"a confirmed override below the label's own 75 minutes band still does not read nailed", 70, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, debutant := partialGameweekEngine(t)
			e.Priors = fakePriors{} // no Premier League history on record at all
			debutant.Minutes, debutant.Starts = 0, 0

			e.SetMinutesOverride(debutant.Code, tt.overrideAt, 0, tt.confirmed)

			m := e.Metrics(debutant)
			if m.ExpectedMinutes != tt.overrideAt {
				t.Fatalf("expected minutes %.1f, want the override value %.1f unchanged — "+
					"this test is about the LABEL, not the estimate", m.ExpectedMinutes, tt.overrideAt)
			}
			if got := m.RotationRisk == "nailed"; got != tt.wantNailed {
				t.Errorf("override %.0f confirmed=%v: rotation_risk = %q (nailed=%v), want nailed=%v",
					tt.overrideAt, tt.confirmed, m.RotationRisk, got, tt.wantNailed)
			}
		})
	}
}

// TestMinutesEvidenceIsTheClubsOwnMatches guards the general-population sibling
// of TestMinutesOverrideSurvivesThePriorsBlend: a player with NO override at
// all, who simply has a real prior season AND a real appearance already on the
// board for his own club this season.
//
// blendRatesCode's current/prior mix used n = GameweeksPlayed() as the evidence
// count for MinutesPerMatch and StartShare. During the same multi-day gap the
// other two fixes (#39, #40, both today) address — SeasonHasStarted true,
// GameweeksPlayed still 0 because no gameweek has fully finished — that meant
// EVERY player was blended at n == 0, regardless of how many matches his own
// club had actually played: w = 0/(0+k) = 0, so MinutesPerMatch and StartShare
// came back as exactly last season's rate, discarding a real ninety-minute
// appearance already on record. Left unfixed alongside the other two because it
// degrades toward the prior rather than exploding to a raw value — bounded, not
// silent.
func TestMinutesEvidenceIsTheClubsOwnMatches(t *testing.T) {
	e, _ := partialGameweekEngine(t)
	established := &e.Boot.Elements[0]
	if e.GameweeksPlayed() != 0 || !e.SeasonHasStarted() {
		t.Fatalf("setup: GameweeksPlayed=%d SeasonHasStarted=%v, want 0/true",
			e.GameweeksPlayed(), e.SeasonHasStarted())
	}
	if got := e.TeamMatchesStarted(established.Team); got != 1 {
		t.Fatalf("setup: TeamMatchesStarted(established's club) = %d, want 1 — "+
			"this test needs a player whose OWN club has already played", got)
	}

	// A thin backup season last year — 630 minutes across 38, the same shape as
	// the Kinsky incident's prior.
	e.Priors = fakePriors{established.Code: {Minutes: 630, Starts: 7, XG: 1, XA: 1, XGC: 10, DefCon: 5}}
	// But his club's one match so far this season, he played the full ninety —
	// real, current-season evidence that his role is not what it was, and no
	// standing override asserts it for him.
	established.Minutes, established.Starts = 90, 1

	m := e.Metrics(established)

	priorOnly := 630.0 / GameweeksPerSeason // what n == 0 (the bug) reports
	if m.ExpectedMinutes <= priorOnly+5 {
		t.Errorf("expected minutes %.1f is barely above the pure-prior figure %.1f; "+
			"a 90-minute appearance already on record for this club's one match is "+
			"not moving the estimate, so the blend is still keyed on GameweeksPlayed "+
			"(0) rather than this club's own TeamMatchesStarted (1)",
			m.ExpectedMinutes, priorOnly)
	}
	// And it must not be anywhere near the full 90 either — one match is thin
	// evidence against a real 38-game prior season, and BlendMinutesK exists to
	// shrink exactly this.
	if m.ExpectedMinutes > 50 {
		t.Errorf("expected minutes %.1f overweights a single match against a real "+
			"prior season", m.ExpectedMinutes)
	}
}

// TestAFinishedGameweekDoesNotMakeADebutantLookNailed is the second occurrence
// of the exact defect e41d5bd2 (2026-08-23) fixed once already, one call site
// over.
//
// shrinkToLeague shrinks a no-prior player's RATE terms unconditionally, on
// n90 = el.Minutes/90 — real, current-season evidence that never resets. It
// shrinks the VOLUME terms (MinutesPerMatch, StartShare) only inside
// `if e.GameweeksPlayed() == 0`, gated on a different quantity entirely: how
// many gameweeks FPL has marked Finished, not how much of the player's own
// evidence has accumulated. GameweeksPlayed() flips a gameweek to Finished
// the instant every fixture in it has blown its final whistle — a
// bookkeeping event that carries no new football for a player whose own
// club has already played and stopped. A debutant whose one ninety-minute
// appearance was his club's only match so far reads shrunk while that
// gameweek is still open, then, at the exact instant the LAST OTHER club's
// fixture in the gameweek finishes, reads raw: MinutesPerMatch jumps to
// 90.0 and StartShare to 1.00, though not one additional minute of his own
// was played anywhere.
//
// e41d5bd2's own comment defended the boundary this way: "a no-prior
// player's OWN current-season n90 grows every week he plays regardless of
// whether he ever gets a prior SEASON on file, so the shrink weight is
// already correctly near 1 by then [past GameweeksPlayed()==0]." That
// reasoning holds many weeks into a season, and is false at the exact
// boundary this test sits on: at n90 = 1 (one 90-minute debut), wMin =
// n90/(n90+BlendMinutesK) = 1/6 — nowhere near 1 — so the gate does not
// merely stop shrinking a population that has outgrown the need, it stops
// shrinking THIS population mid-correction, at its single most extreme
// weight.
//
// This test does not assert a shrunk value, a cutoff, or a gate condition —
// the fix shape is an open prototype landing separately. It asserts the
// discontinuity itself is absent: two engines, identical in every respect
// except whether the gameweek already played is flagged Finished, must
// report the same MinutesPerMatch (via ExpectedMinutes) and StartShare for
// the debutant. No new football happened between the two states, so on
// this model's own terms no answer may change.
//
// The league rate the debutant shrinks toward is rebuilt LOCALLY below,
// rather than left as partialGameweekEngine hands it back, because that
// helper's own established player carries a full-SEASON minutes total
// (3000/33) against a fixture list holding a single match — fine for the
// three other tests in this file that use the helper and never look at the
// league rate, but it makes calibrateLeagueRates divide a season total by
// one match and report an impossible ~3000-minute, ~33-start "average" for
// a single game, which is what a shrink toward it looks like a jump UP
// rather than the down-toward-a-plausible-average shrink the real defect
// produces.
func TestAFinishedGameweekDoesNotMakeADebutantLookNailed(t *testing.T) {
	live, debutantLive := partialGameweekEngine(t)
	finished, debutantFinished := partialGameweekEngine(t)
	finished.Boot.Events[0].Finished = true // the gameweek is now marked over

	// Give the established player a one-match line instead of a season
	// total — a substitute appearance, on the field for 60 minutes without
	// starting — so the recalibrated league rate is a plausible per-match
	// figure (60 minutes, 0 start share) rather than a season total divided
	// by one match. Identical on both engines, and recalibrated after the
	// Finished flip so each engine's own matchesAvailable is used, though it
	// resolves to the same window (1) on both — see DataWindow/
	// inLiveGameweekGap's own comments for why.
	establishedLive := &live.Boot.Elements[0]
	establishedFinished := &finished.Boot.Elements[0]
	establishedLive.Minutes, establishedLive.Starts = 60, 0
	establishedFinished.Minutes, establishedFinished.Starts = 60, 0
	live.calibrateLeagueRates()
	finished.calibrateLeagueRates()

	if live.GameweeksPlayed() != 0 {
		t.Fatalf("setup: GameweeksPlayed() = %d on the live engine, want 0", live.GameweeksPlayed())
	}
	if finished.GameweeksPlayed() != 1 {
		t.Fatalf("setup: GameweeksPlayed() = %d on the finished engine, want 1", finished.GameweeksPlayed())
	}
	for _, e := range []*Engine{live, finished} {
		if !e.SeasonHasStarted() {
			t.Fatal("setup: SeasonHasStarted() is false; the fixture setup is not exercising the gap")
		}
	}

	live.Priors = fakePriors{}     // nobody has a prior
	finished.Priors = fakePriors{} // identical: nobody has a prior
	debutantLive.Minutes, debutantLive.Starts = 90, 1
	debutantFinished.Minutes, debutantFinished.Starts = 90, 1 // identical debut, on the identical fixture

	mLive := live.Metrics(debutantLive)
	mFinished := finished.Metrics(debutantFinished)

	t.Logf("GameweeksPlayed=0: expected minutes %.3f, start share %.3f", mLive.ExpectedMinutes, mLive.StartShare)
	t.Logf("GameweeksPlayed=1: expected minutes %.3f, start share %.3f", mFinished.ExpectedMinutes, mFinished.StartShare)

	const tolerance = 1e-9
	if diff := math.Abs(mFinished.ExpectedMinutes - mLive.ExpectedMinutes); diff > tolerance {
		t.Errorf("expected minutes moved from %.3f to %.3f purely because the gameweek "+
			"flipped to Finished — no new football was played, so shrinkToLeague's volume "+
			"gate must not change the answer here", mLive.ExpectedMinutes, mFinished.ExpectedMinutes)
	}
	if diff := math.Abs(mFinished.StartShare - mLive.StartShare); diff > tolerance {
		t.Errorf("start share moved from %.3f to %.3f purely because the gameweek flipped "+
			"to Finished — no new football was played, so shrinkToLeague's volume gate "+
			"must not change the answer here", mLive.StartShare, mFinished.StartShare)
	}
}

// TestMinutesEvidenceIgnoresAMatchStillInProgress guards the sibling mistake to the one
// above, at a finer grain: a fixture that has KICKED OFF but has not yet locked in its
// final numbers is not a match's worth of evidence, it is a partial one. el.Minutes for
// a club mid-fixture is whatever the live match has accumulated so far — a nailed
// starter's 47 minutes into his 90, not his eventual total — and TeamMatchesStarted
// alone would count that as "1 match played", blending the partial figure in as if it
// were complete. TeamMatchesFinished must answer 0 here, leaving the blend on the prior
// until the match's own stats are final.
//
// This gates on FinishedProvisional, not Finished — see the field's own comment on
// fpl.Fixture and TeamMatchesFinished's. Finished lags full time by many hours live,
// so a test built on it would prove the wrong thing.
func TestMinutesEvidenceIgnoresAMatchStillInProgress(t *testing.T) {
	e, _ := partialGameweekEngine(t)
	established := &e.Boot.Elements[0]
	// Overwrite the shared fixture: kicked off, still being played, so its numbers
	// are not final yet — Finished stays whatever the fixture literal set it to,
	// deliberately, since TeamMatchesFinished must not be reading that field.
	e.Fixtures[0].FinishedProvisional = false
	if got := e.TeamMatchesStarted(established.Team); got != 1 {
		t.Fatalf("setup: TeamMatchesStarted = %d, want 1 (the match has kicked off)", got)
	}
	if got := e.TeamMatchesFinished(established.Team); got != 0 {
		t.Fatalf("TeamMatchesFinished = %d, want 0 — the match's numbers are not final yet", got)
	}

	e.Priors = fakePriors{established.Code: {Minutes: 630, Starts: 7, XG: 1, XA: 1, XGC: 10, DefCon: 5}}
	// A live snapshot mid-match: 47 minutes so far, not a completed 90.
	established.Minutes, established.Starts = 47, 1

	m := e.Metrics(established)

	priorOnly := 630.0 / GameweeksPerSeason
	if m.ExpectedMinutes > priorOnly+5 {
		t.Errorf("expected minutes %.1f moved away from the pure-prior figure %.1f while "+
			"the match is still live; a partial in-match snapshot must not count as a "+
			"completed match of evidence until TeamMatchesFinished says it is one",
			m.ExpectedMinutes, priorOnly)
	}
}

// TestCountingStatsGoThroughTheBlend guards a bug that survived the first blend.
//
// Bonus, saves and cards were read straight off the element as count*90/minutes
// while xG, xA and defensive contributions were blended. A player with 22
// minutes and two bonus points therefore scored at 8.18 bonus a gameweek, and
// the replay's early transfers were driven almost entirely by that: modelled
// gains of +5.59 and +8.50 a gameweek, into players with one substitute
// appearance.
//
// Any new counting term must be blended the same way, or it reproduces this.
func TestCountingStatsGoThroughTheBlend(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	el := findEverPresent(t, e)
	mins, starts := el.Minutes, el.Starts
	bonus, saves, yel, red := el.Bonus, el.Saves, el.YellowCards, el.RedCards
	defer func() {
		el.Minutes, el.Starts = mins, starts
		el.Bonus, el.Saves, el.YellowCards, el.RedCards = bonus, saves, yel, red
	}()

	// Last season: an ever-present with entirely ordinary counting stats.
	e.Priors = fakePriors{el.Code: {
		Minutes: 3300, Starts: 37, XG: 8, XA: 6, XGC: 40, DefCon: 300,
		Bonus: 11, Saves: 0, Yellow: 4, Red: 0,
	}}
	playGameweeks(t, e, 2)
	el.Minutes, el.Starts = 22, 0 // one brief cameo across two gameweeks
	el.Bonus, el.Saves, el.YellowCards, el.RedCards = 2, 3, 1, 1

	m := e.Metrics(el)
	// 22 minutes is a quarter of a match, so unblended every one of these is
	// multiplied by roughly four.
	for _, c := range []struct {
		name string
		got  float64
		raw  float64
	}{
		{"bonus", m.Bonus90, 2 * 90.0 / 22},
		{"saves", m.Saves90, 3 * 90.0 / 22},
		{"yellow", m.Yellow90, 1 * 90.0 / 22},
		{"red", m.Red90, 1 * 90.0 / 22},
	} {
		if c.got > c.raw/2 {
			t.Errorf("%s/90 is %.2f against a raw %.2f; it is not going through the blend",
				c.name, c.got, c.raw)
		}
	}
	// And the effect that matters: the score must not explode off one cameo.
	if m.Score > 8 {
		t.Errorf("score %.2f off 22 minutes; the counting stats are still raw", m.Score)
	}
}

// TestCalibratedBlendConstants records where the defaults came from, so a change
// has to be a deliberate recalibration rather than a nudge.
func TestCalibratedBlendConstants(t *testing.T) {
	w := DefaultWeights()
	if w.BlendMinutesK != 5 {
		t.Errorf("BlendMinutesK is %v; 5 was measured against 2025-26 (MAE 18.74 mins/match, "+
			"15%% better than either extreme)", w.BlendMinutesK)
	}
	if w.BlendRateK != 8 {
		t.Errorf("BlendRateK is %v; 8 was measured with 2024-25 as prior and 2025-26 as outcome "+
			"across 218 players (MAE 0.0511 xG/90, 16%% better than ignoring last season)", w.BlendRateK)
	}
	if w.LeagueShrinkK != 8 {
		t.Errorf("LeagueShrinkK is %v; the out-of-sample MAE optimum (K=2) costs POLICY "+
			"-0.843/gw (t=-1.94) on the replay — a predictive win that loses on the argmax, "+
			"the same failure mode as rate recency. Stays at the shared 8.", w.LeagueShrinkK)
	}
}

// fakeRecent stands in for per-gameweek history.
type fakeRecent map[int]RecentPlayer

func (f fakeRecent) Get(code int) (RecentPlayer, bool) { p, ok := f[code]; return p, ok }

// TestRecentMinutesDisplaceTheSeasonAverage is the point of the Recent hook.
//
// Minutes are a statement about the present. A player who lost his place six
// weeks ago still reads as a starter on a season average, and a season average
// is all FPL's bootstrap publishes. Predicting the next five gameweeks' minutes
// across three replayed seasons, a half-life of 2 is 8.9% better than the flat
// total; wiring it into the replay won all three seasons, +31 on the mean.
func TestRecentMinutesDisplaceTheSeasonAverage(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	el := findEverPresent(t, e)
	mins, starts := el.Minutes, el.Starts
	defer func() { el.Minutes, el.Starts = mins, starts }()

	e.Priors = fakePriors{el.Code: {Minutes: 3300, Starts: 37, XG: 8, XA: 6, XGC: 40, DefCon: 300}}
	playGameweeks(t, e, 20)
	// Season to date says an ever-present: 20 full matches.
	el.Minutes, el.Starts = 1800, 20

	seasonAvg := e.Metrics(el)

	// Recently, though, he has stopped playing.
	e.Recent = fakeRecent{el.Code: {MinutesPerMatch: 5, StartShare: 0, Matches: 20}}
	recent := e.Metrics(el)

	t.Logf("expected minutes: %.1f on the season average, %.1f on recent form",
		seasonAvg.ExpectedMinutes, recent.ExpectedMinutes)
	if recent.ExpectedMinutes >= seasonAvg.ExpectedMinutes {
		t.Errorf("a player who has stopped playing reads %.1f minutes against %.1f on the "+
			"season average; the recency hook is not applied",
			recent.ExpectedMinutes, seasonAvg.ExpectedMinutes)
	}
	if recent.Score >= seasonAvg.Score {
		t.Error("the score did not follow the minutes down")
	}
}

// TestRecentIsIgnoredPreSeason — before a ball is kicked there is no current
// season to be recent about, and FPL's totals are still last season's.
func TestRecentIsIgnoredPreSeason(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	el := findEverPresent(t, e)
	before := e.Metrics(el)
	e.Recent = fakeRecent{el.Code: {MinutesPerMatch: 0, StartShare: 0, Matches: 20}}
	if after := e.Metrics(el); after.ExpectedMinutes != before.ExpectedMinutes {
		t.Errorf("pre-season minutes moved from %.1f to %.1f; recency must not apply",
			before.ExpectedMinutes, after.ExpectedMinutes)
	}
}

// TestRecencyAppliesToMinutesOnly records the measurement that split them.
//
// The same out-of-sample test run on points and on xG+xA says sharp recency is
// actively worse — "last 3 games" is 19% worse than the season average on both,
// because underlying quality is stable and a short window chases finishing
// variance. Only minutes are weighted, and a future term must be measured
// before it joins them.
func TestRecencyAppliesToMinutesOnly(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	el := findEverPresent(t, e)
	mins, starts := el.Minutes, el.Starts
	defer func() { el.Minutes, el.Starts = mins, starts }()

	e.Priors = fakePriors{el.Code: {Minutes: 3300, Starts: 37, XG: 8, XA: 6, XGC: 40, DefCon: 300}}
	playGameweeks(t, e, 20)
	el.Minutes, el.Starts = 1800, 20

	base := e.Metrics(el)
	// Recent form that leaves minutes untouched must leave the rates untouched
	// too — the hook carries no rate information at all.
	e.Recent = fakeRecent{el.Code: {
		MinutesPerMatch: base.ExpectedMinutes, StartShare: 1, Matches: 20,
	}}
	got := e.Metrics(el)
	for _, c := range []struct {
		name     string
		was, now float64
	}{
		{"xG/90", base.XG90, got.XG90},
		{"xA/90", base.XA90, got.XA90},
		{"bonus/90", base.Bonus90, got.Bonus90},
		{"defcon/90", base.DefCon90, got.DefCon90},
	} {
		if math.Abs(c.was-c.now) > 1e-9 {
			t.Errorf("%s moved from %.4f to %.4f; recency must touch minutes only",
				c.name, c.was, c.now)
		}
	}
}

// TestMinutesOverrideBeatsTheData is the mechanism that should be reached for
// before any lock.
//
// Isak scored 0.49 pts/gw because a leg break held him to 694 minutes — the
// number is an artefact, not a judgement about his role. Correcting it lets the
// model recompute and answer the question it is good at, which is whether that
// is worth his price. A lock forces him in and never asks.
func TestMinutesOverrideBeatsTheData(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	el := findEverPresent(t, e)
	mins, starts := el.Minutes, el.Starts
	defer func() { el.Minutes, el.Starts = mins, starts }()

	playGameweeks(t, e, 10)
	el.Minutes, el.Starts = 90, 1 // one appearance in ten gameweeks

	fringe := e.Metrics(el)
	if fringe.ExpectedMinutes > 30 {
		t.Skipf("fixture gives %.1f expected minutes; too high to test", fringe.ExpectedMinutes)
	}

	e.MinutesOverride = map[int]float64{el.Code: 90}
	fixed := e.Metrics(el)

	t.Logf("%s: %.2f pts/gw on %.1f mins -> %.2f on %.1f after the correction",
		el.WebName, fringe.Score, fringe.ExpectedMinutes, fixed.Score, fixed.ExpectedMinutes)

	if fixed.ExpectedMinutes < 85 {
		t.Errorf("expected minutes %.1f after an override of 90", fixed.ExpectedMinutes)
	}
	if fixed.Score <= fringe.Score {
		t.Errorf("score did not rise with the minutes: %.2f -> %.2f", fringe.Score, fixed.Score)
	}
	// The correction must not touch his rates — it says how much he plays, not
	// how good he is.
	if math.Abs(fixed.XG90-fringe.XG90) > 1e-9 || math.Abs(fixed.XA90-fringe.XA90) > 1e-9 {
		t.Error("a minutes correction changed the per-90 rates")
	}
}

// TestNaturalMetricsUndoesExactlyOneOverride guards the mechanism behind the
// news surface's before/after line ("pts a week 4.20 -> 3.15"): NaturalMetrics
// must reproduce the pre-override score for the overridden player, through the
// same scoring path Metrics uses, and must not move any OTHER player's score —
// a player check-in for one footballer must not silently reprice the pool.
func TestNaturalMetricsUndoesExactlyOneOverride(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	el := findEverPresent(t, e)
	mins, starts := el.Minutes, el.Starts
	defer func() { el.Minutes, el.Starts = mins, starts }()

	playGameweeks(t, e, 10)
	el.Minutes, el.Starts = 90, 1 // one appearance in ten gameweeks

	// A bystander, scored before the override exists, so the same-figure check
	// below is not comparing against a value the override could have moved.
	var bystander *fpl.Element
	for i := range e.Boot.Elements {
		if c := e.Boot.Elements[i].Code; c != 0 && c != el.Code {
			bystander = &e.Boot.Elements[i]
			break
		}
	}
	if bystander == nil {
		t.Skip("no second player in the pool")
	}
	bystanderBefore := e.Metrics(bystander).Score

	if e.HasMinutesOverride(el.Code) {
		t.Fatal("override present before one was installed")
	}
	natural := e.Metrics(el)

	e.MinutesOverride = map[int]float64{el.Code: 90}
	if !e.HasMinutesOverride(el.Code) {
		t.Fatal("HasMinutesOverride false immediately after installing one")
	}
	overridden := e.Metrics(el)
	undone := e.NaturalMetrics(el)

	t.Logf("%s: natural %.2f, overridden %.2f, NaturalMetrics %.2f",
		el.WebName, natural.Score, overridden.Score, undone.Score)

	if overridden.Score == natural.Score {
		t.Fatal("test is not exercising the override: overridden score did not move")
	}
	if math.Abs(undone.Score-natural.Score) > 1e-9 {
		t.Errorf("NaturalMetrics gave %.4f, want the pre-override %.4f", undone.Score, natural.Score)
	}
	if math.Abs(undone.ExpectedMinutes-natural.ExpectedMinutes) > 1e-9 {
		t.Errorf("NaturalMetrics minutes %.2f, want the pre-override %.2f",
			undone.ExpectedMinutes, natural.ExpectedMinutes)
	}

	// The bystander must be untouched by both the override and by asking to
	// ignore someone else's.
	if got := e.Metrics(bystander).Score; math.Abs(got-bystanderBefore) > 1e-9 {
		t.Errorf("bystander's score moved from %.4f to %.4f", bystanderBefore, got)
	}
	if got := e.NaturalMetrics(bystander).Score; math.Abs(got-bystanderBefore) > 1e-9 {
		t.Errorf("NaturalMetrics moved a player with no override of his own: %.4f -> %.4f",
			bystanderBefore, got)
	}

	// A player with no override at all: NaturalMetrics must be a no-op, not an
	// error and not a different number, so a caller who forgets to gate on
	// HasMinutesOverride gets the honest answer rather than a silently wrong one.
	e.MinutesOverride = nil
	if e.HasMinutesOverride(el.Code) {
		t.Fatal("override still reported present after clearing")
	}
	plain := e.Metrics(el)
	plainNatural := e.NaturalMetrics(el)
	if math.Abs(plain.Score-plainNatural.Score) > 1e-9 {
		t.Errorf("NaturalMetrics diverged from Metrics with no override in force: %.4f vs %.4f",
			plainNatural.Score, plain.Score)
	}
}

// TestMinutesOverrideToZeroSuppresses — setting a player to zero is how the
// analysis layer says "he is out", and it should make him unpickable without
// needing the certainty a hard exclusion implies.
func TestMinutesOverrideToZeroSuppresses(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	el := findEverPresent(t, e)
	playGameweeks(t, e, 10)

	before := e.Metrics(el)
	e.MinutesOverride = map[int]float64{el.Code: 0}
	after := e.Metrics(el)

	t.Logf("%s: %.2f -> %.2f pts/gw when set to zero minutes", el.WebName, before.Score, after.Score)
	if after.Score >= before.Score*0.25 {
		t.Errorf("a player set to zero minutes still scores %.2f against %.2f; he would "+
			"still be picked", after.Score, before.Score)
	}
}

// TestMinutesOverrideIsProratedByReturnDate — "out until GW12" is a claim about
// particular gameweeks, not about every week the horizon averages over.
//
// Applied flat, a short override on a five-gameweek horizon says the absence
// lasts as long as the model happens to be looking. Prorated, a player back
// inside the window carries his own minutes for the weeks he is available,
// which is what makes an expected return date usable through the mechanism that
// already works instead of through an exclusion.
func TestMinutesOverrideIsProratedByReturnDate(t *testing.T) {
	e := testEngine(t)
	next := e.Boot.NextEvent()
	if next == nil {
		t.Skip("no next gameweek")
	}
	// An established player, so the natural estimate is well clear of zero and
	// the proration has something to show.
	var el *fpl.Element
	for i := range e.Boot.Elements {
		if e.Boot.Elements[i].Minutes > 2500 {
			el = &e.Boot.Elements[i]
			break
		}
	}
	if el == nil {
		t.Skip("no established player in the pool")
	}
	natural := e.Metrics(el).ExpectedMinutes
	if natural <= 10 {
		t.Skipf("natural minutes %.1f too low to test", natural)
	}

	e.Weights.Horizon = 5
	e.MinutesOverride = map[int]float64{el.Code: 0}

	// Indefinite: applies flat, so the player is worth nothing across the board.
	e.MinutesOverrideUntil = nil
	if got := e.prorateOverride(el.Code, 0, natural); got != 0 {
		t.Errorf("indefinite override prorated to %.2f, want 0", got)
	}

	// Out for the whole horizon: still flat.
	e.MinutesOverrideUntil = map[int]int{el.Code: next.ID + 4}
	if got := e.prorateOverride(el.Code, 0, natural); got != 0 {
		t.Errorf("override covering the horizon gave %.2f, want 0", got)
	}

	// Back after two of the five: three weeks at his own minutes.
	e.MinutesOverrideUntil = map[int]int{el.Code: next.ID + 1}
	got := e.prorateOverride(el.Code, 0, natural)
	want := 3 * natural / 5
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("prorated to %.3f, want %.3f (natural %.1f)", got, want, natural)
	}
	if !(got > 0 && got < natural) {
		t.Errorf("prorated %.3f should sit strictly between 0 and %.3f", got, natural)
	}

	// The imminent-week view has a horizon of one, so a player out this week is
	// out, with no averaging to soften it.
	e.Weights.Horizon = 1
	if got := e.prorateOverride(el.Code, 0, natural); got != 0 {
		t.Errorf("horizon-1 gave %.2f for a player out this week, want 0", got)
	}
}
