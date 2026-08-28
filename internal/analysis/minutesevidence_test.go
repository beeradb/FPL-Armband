package analysis

import (
	"testing"
	"time"

	"armband/internal/fpl"
)

// unplayedEngine builds a club that has played `matches` finished fixtures and a
// player with no prior season who did not feature in any of them.
//
// joined, when non-empty, is his TeamJoinDate. Fixtures are dated one week apart
// starting 2026-08-01, so a join date can be placed before or after any of them.
func unplayedEngine(matches int, joined string) (*Engine, *fpl.Element) {
	player := fpl.Element{
		ID: 2, Code: 2, WebName: "Unplayed", ElementType: 3, Team: 1,
		NowCost: 45, Status: "a", TeamJoinDate: joined,
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
		// A second player who plays every match, so the league rates this
		// shrinks toward are not built from the unplayed man alone. His minutes
		// scale with the fixtures played, because a mid-season bootstrap carries
		// season-to-date totals and a fixed 3000 would make his per-match rate
		// absurd at low match counts.
		Elements: []fpl.Element{
			{ID: 1, Code: 1, WebName: "Regular", ElementType: 3, Team: 2,
				NowCost: 60, Status: "a",
				Minutes: 90 * matches, Starts: matches},
			player,
		},
	}
	for i := 1; i <= 38; i++ {
		b.Events = append(b.Events, fpl.Event{ID: i, Name: "Gameweek"})
	}
	start := time.Date(2026, 8, 1, 15, 0, 0, 0, time.UTC)
	var fx []fpl.Fixture
	for i := 0; i < matches; i++ {
		gw := i + 1
		ko := start.AddDate(0, 0, 7*i)
		fx = append(fx, fpl.Fixture{
			ID: i + 1, Event: &gw, TeamH: 1, TeamA: 2,
			Started: true, Finished: true, FinishedProvisional: true,
			KickoffTime: &ko,
		})
	}
	e := NewEngineFull(b, fx, DefaultWeights(), Congestion{}, RoleRisk{})
	// ⚠️ A non-nil prior index that simply LACKS the unplayed player. With
	// e.Priors nil, blendRatesCode returns before shrinkToLeague is reached and
	// the whole path under test never runs — a fixture that would have made
	// every assertion here vacuous.
	e.Priors = fakePriors{1: {Minutes: 3000, Starts: 33}}
	return e, &e.Boot.Elements[1]
}

// ⚠️ **A club match a player sat out is EVIDENCE, and weighting by his own
// minutes makes it invisible.**
//
// `shrinkToLeague` weighted the league fallback by `n90 = el.Minutes/90` until
// 2026-08-28. For a rate that is right — a rate can only be estimated from
// football actually played. For VOLUME it is exactly wrong: a player whose club
// has played twenty matches without him has not produced "no evidence" about
// whether he plays, he has produced twenty zeroes, and the only quantity that
// could have displaced the league average was the one that stays at zero.
//
// Measured across six replayed seasons before the fix: a no-prior player still
// unplayed at GW20 read 41.9 expected minutes a match against 0.6 actual, and
// the gap WIDENED as the season ran.
//
// The established-prior branch of `blendRatesCode` already counted
// `TeamMatchesFinished`, and `inLiveGameweekGap`'s doc comment already classed
// "blend.go's minutes-evidence mix" as an evidence COUNT that must. One quantity,
// two implementations, and only the copy on the no-prior path was blind.
//
// This pins the property rather than a number: more matches missed must mean
// fewer expected minutes, strictly, every step.
func TestSittingOutIsEvidenceThatHeDoesNotPlay(t *testing.T) {
	// ⚠️ Starts at ONE finished match, not zero. With no fixture played the
	// season has not started and `blendRatesCode` takes its pre-season branch,
	// which is a different code path reading 0 for its own reasons — including
	// it made the first comparison meaningless rather than strict.
	var first, last float64
	for _, matches := range []int{1, 5, 10, 20} {
		e, el := unplayedEngine(matches, "")
		got := e.Metrics(el).ExpectedMinutes
		t.Logf("%2d matches missed -> %.1f expected minutes", matches, got)
		if matches > 1 && got >= last {
			t.Errorf("after %d missed matches the model expects %.1f minutes, "+
				"no less than the %.1f it expected with fewer — sitting out is "+
				"not being counted as evidence", matches, got, last)
		}
		if matches == 1 {
			first = got
		}
		last = got
	}
	// And the level has to actually go somewhere, not merely tick down. A
	// monotone check alone passes on a decline of a tenth of a minute across a
	// whole season, which would be the defect wearing the fix's clothes.
	if last > first/2 {
		t.Errorf("twenty missed matches took expected minutes only from %.1f to "+
			"%.1f; the fallback is still doing most of the talking", first, last)
	}
}

// ⚠️ **A mid-season arrival must not be charged for matches he could not play
// in**, which is the opposite error and lands on exactly the group the league
// fallback exists to serve. His club's finished-match count includes fixtures
// from before he was registered; counting those would read "he was not at the
// club" as "he does not get picked".
func TestAMidSeasonArrivalIsNotChargedForTheAutumn(t *testing.T) {
	// Ten fixtures from 2026-08-01, one a week, so the tenth is 2026-10-03.
	// A player who joined on 2026-09-26 could have played in the last two.
	const matches = 10
	late, _ := unplayedEngine(matches, "2026-09-26")
	early, _ := unplayedEngine(matches, "")

	lateEl := &late.Boot.Elements[1]
	earlyEl := &early.Boot.Elements[1]
	if got, want := late.minutesEvidence(lateEl), 2.0; got != want {
		t.Errorf("a player who joined before the last %v of %d fixtures was "+
			"charged %v matches of evidence", want, matches, got)
	}
	if got := early.minutesEvidence(earlyEl); got != float64(matches) {
		t.Errorf("a player with no join date was charged %v of %d matches; the "+
			"uncapped count is the right default when FPL supplies no date",
			got, matches)
	}

	lateMin := late.Metrics(lateEl).ExpectedMinutes
	earlyMin := early.Metrics(earlyEl).ExpectedMinutes
	if lateMin <= earlyMin {
		t.Errorf("the January signing (%.1f) is rated no higher than the player "+
			"who has been available all season and never picked (%.1f) — the "+
			"join-date cap is not being applied", lateMin, earlyMin)
	}
}

// A join date FPL did not supply, or supplied unparseably, must fall back to the
// uncapped club count rather than to zero evidence. Zero would hand every such
// player the full league average, which is the defect this all started from.
func TestAnUnusableJoinDateFallsBackToTheClubsOwnCount(t *testing.T) {
	for _, joined := range []string{"", "not-a-date", "2026/09/26"} {
		e, el := unplayedEngine(10, joined)
		if got := e.minutesEvidence(el); got != 10 {
			t.Errorf("TeamJoinDate %q gave %v matches of evidence, want the "+
				"uncapped 10 — an unreadable date must not erase the club's "+
				"own record", joined, got)
		}
	}
}

// The two branches must count evidence the SAME way, which is the property the
// whole change is about.
//
// `blendRatesCode`'s established-prior branch counts `TeamMatchesFinished`; this
// pins that `minutesEvidence` — the no-prior branch's counter — agrees with it
// wherever the join-date cap does not bind. One quantity, one implementation is
// the rule this violated for as long as the two differed, and a later edit to
// either side that reintroduces a divergence fails here.
//
// This replaces a confinement check that asserted something FALSE: that a player
// with a prior is untouched by his club's match count. He is not, and correctly
// so — his branch has always used it. Confinement was the wrong frame; agreement
// is the right one.
func TestBothBlendBranchesCountEvidenceTheSameWay(t *testing.T) {
	for _, matches := range []int{0, 1, 5, 10, 20} {
		e, el := unplayedEngine(matches, "")
		got := e.minutesEvidence(el)
		want := float64(e.TeamMatchesFinished(el.Team))
		if got != want {
			t.Errorf("with %d finished matches the no-prior branch counts %v of "+
				"evidence and the established-prior branch counts %v; they are "+
				"the same quantity and must not diverge", matches, got, want)
		}
	}
}
