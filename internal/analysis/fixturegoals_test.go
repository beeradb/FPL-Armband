package analysis

import (
	"math"
	"testing"

	"armband/internal/fpl"
)

// FixtureGoals composes two quantities this package already owns, and these
// tests pin the two properties that make it a composition rather than a new
// model. Without them the multiplication is just an assertion.

// twoClubLeague builds an engine whose clubs have the granular strength ratings
// priorFromStrength prefers, so the prior is the fitted line rather than the
// coarse fallback. No fixtures are played, so every rate is its prior.
func twoClubLeague(t *testing.T, ratings ...[3]int) *Engine {
	t.Helper()
	var boot fpl.Bootstrap
	for i, r := range ratings {
		boot.Teams = append(boot.Teams, fpl.Team{
			ID: i + 1, Name: "Club", ShortName: "C",
			StrengthAttackHome: r[0], StrengthAttackAway: r[0],
			StrengthDefenceHome: r[1], StrengthDefenceAway: r[1],
			Strength: r[2],
		})
	}
	return NewEngine(&boot, nil, Weights{})
}

// TestAFixtureAgainstAnAverageOpponentIsTheClubsOwnRate is the identity the doc
// comment rests on. A projection that did not reduce to the club's own rate
// against a league-average opponent would be scaling by something other than
// opponent leakiness, and the number would mean nothing on its own.
func TestAFixtureAgainstAnAverageOpponentIsTheClubsOwnRate(t *testing.T) {
	// Three identical clubs, so every club's conceded rate IS the league
	// average and every multiplier is exactly 1.
	e := twoClubLeague(t, [3]int{1150, 1150, 3}, [3]int{1150, 1150, 3},
		[3]int{1150, 1150, 3})

	conceded, _ := e.LeagueGoals()
	if got := e.TeamRatesFor(2).Conceded; math.Abs(got-conceded) > 1e-12 {
		t.Fatalf("setup is wrong: club 2 concedes %v, league average is %v — "+
			"this test cannot say anything unless they are equal", got, conceded)
	}

	home, away := e.FixtureGoals(1, 2)
	if want := e.TeamRatesFor(1).Scored; math.Abs(home-want) > 1e-12 {
		t.Errorf("home projection %v, want the club's own rate %v. Against an "+
			"opponent conceding exactly the league average the multiplier is 1, "+
			"so the projection must BE the rate — otherwise FixtureGoals is "+
			"scaling by something the doc comment does not describe", home, want)
	}
	if want := e.TeamRatesFor(2).Scored; math.Abs(away-want) > 1e-12 {
		t.Errorf("away projection %v, want %v", away, want)
	}
}

// TestFixtureGoalsUsesTheDampedMultiplierRatherThanTheRawRatio pins WHICH form
// ships, and deliberately does not claim it is the right one.
//
// The two forms move in the same direction and differ only in SIZE, so a swap
// between them is invisible in any output a person eyeballs — which is the only
// reason this test exists. ⚠️ It is not evidence for the damped form. The
// square root was fitted against attacking RETURNS and is used here on club
// GOALS, and whether those two respond to a leaky defence by the same amount is
// unmeasured; see FixtureGoals's doc comment. If someone measures it and the
// raw ratio wins, this test is what they should change, not what should stop
// them.
func TestFixtureGoalsUsesTheDampedMultiplierRatherThanTheRawRatio(t *testing.T) {
	// A leaky opponent (club 2, weak defence) against two average clubs, so the
	// league average is pulled by only one club and the ratio is well above 1.
	e := twoClubLeague(t, [3]int{1150, 1150, 3}, [3]int{1150, 1000, 2},
		[3]int{1150, 1150, 3})

	leagueConceded, _ := e.LeagueGoals()
	ratio := e.TeamRatesFor(2).Conceded / leagueConceded
	if ratio <= 1.02 {
		t.Fatalf("setup is wrong: opponent concedes %.3f against a league %.3f "+
			"(ratio %.3f). The damped and raw forms only separate when the "+
			"ratio is away from 1, so this test has no teeth as built",
			e.TeamRatesFor(2).Conceded, leagueConceded, ratio)
	}

	home, _ := e.FixtureGoals(1, 2)
	rate := e.TeamRatesFor(1).Scored
	damped, raw := rate*math.Pow(ratio, magnitudeAlpha), rate*ratio

	if math.Abs(home-damped) > 1e-9 {
		t.Errorf("projection %v, want the DAMPED %v", home, damped)
	}
	if math.Abs(home-raw) < 1e-9 {
		t.Errorf("projection %v equals the RAW ratio form %v, so the damping is "+
			"no longer being applied. That may be the right call — nothing has "+
			"measured the two on club goals — but it is a change of shipped "+
			"behaviour and must be made deliberately rather than by a refactor",
			home, raw)
	}
}

// TestFixtureGoalsIsBlindToWhoIsAtHome pins the limitation the doc comment
// states, because the (home, away) argument order invites the opposite
// assumption. priorFromStrength averages FPL's home and away ratings away, so
// there is no home advantage in this number to find. If one is ever added this
// test should FAIL and be deleted with the comment it guards.
func TestFixtureGoalsIsBlindToWhoIsAtHome(t *testing.T) {
	e := twoClubLeague(t, [3]int{1250, 1100, 4}, [3]int{1050, 1300, 2},
		[3]int{1150, 1150, 3})

	home, away := e.FixtureGoals(1, 2)
	swappedHome, swappedAway := e.FixtureGoals(2, 1)
	if home != swappedAway || away != swappedHome {
		t.Errorf("swapping the fixture round changed the projection: "+
			"(%v, %v) became (%v, %v). That would mean a home advantage exists "+
			"in TeamRates, which FixtureGoals's doc comment says it does not — "+
			"fix the comment, not this test", home, away, swappedHome, swappedAway)
	}
}
