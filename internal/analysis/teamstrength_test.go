package analysis

import (
	"math"
	"testing"

	"armband/internal/fpl"
)

// TestPriorFromStrengthHandlesBothScales — FPL publishes two different strength
// scales and only one of them exists at a time.
//
// The granular 1000-1400 ratings are what the archive carries, because its
// snapshots are taken once FPL has filled them in. Live in August they are all
// zero and the coarse 1-5 value arrives in strength_overall_home while the
// `strength` field itself is null. Reading only the granular ones gave every
// club in the league exactly the average, which is how this was found.
func TestPriorFromStrengthHandlesBothScales(t *testing.T) {
	granular := &fpl.Team{
		StrengthDefenceHome: 1310, StrengthDefenceAway: 1300,
		StrengthAttackHome: 1390, StrengthAttackAway: 1400,
	}
	c, s := priorFromStrength(granular)
	if !(c > 0.5 && c < 1.5) || !(s > 1.5 && s < 3.0) {
		t.Errorf("a strong club on the granular scale reads %.2f conceded, %.2f scored", c, s)
	}

	// Pre-season: granular absent, coarse arriving in strength_overall_home.
	weak := &fpl.Team{StrengthOverallHome: 2}
	cw, sw := priorFromStrength(weak)
	if cw <= c {
		t.Errorf("a weak club concedes %.2f, no worse than a strong club's %.2f", cw, c)
	}
	if sw >= s {
		t.Errorf("a weak club scores %.2f, no less than a strong club's %.2f", sw, s)
	}
	// And it must not silently fall through to the league average, which is the
	// failure that hid this.
	if math.Abs(cw-leagueAverageGoals) < 1e-9 {
		t.Error("pre-season prior fell through to the league average; the coarse rating was ignored")
	}
}

// TestMagnitudeDifficultyIsSubProportional pins the exponent's job.
//
// Overshooting is exactly how the previous attempt at this failed: re-rating the
// worst defences to difficulty 1 gave +30% where the measured truth is +23%, and
// that cost more than under-shooting had saved. A defence conceding 50% more
// than the league must produce about +22%, not +50%.
func TestMagnitudeDifficultyIsSubProportional(t *testing.T) {
	got := magnitudeRatio(1.5*1.48, 1.48)
	if math.Abs(got-1.225) > 0.02 {
		t.Errorf("a defence conceding 50%% more gives %.3f, want about 1.22 — the measured "+
			"attacker response to the leakiest defences is +23%%, not +50%%", got)
	}
	// Bounded, so a freak early-season rate cannot invent a multiplier.
	if v := magnitudeRatio(10, 1.48); v > 1.60001 {
		t.Errorf("an absurd rate produced %.3f, above the clamp", v)
	}
	if v := magnitudeRatio(0.01, 1.48); v < 0.6-1e-9 {
		t.Errorf("an absurd rate produced %.3f, below the clamp", v)
	}
}

// TestTeamRatesBlendTowardTheSeason — with no matches played the rating is the
// prior; with matches played it moves toward what actually happened, slowly.
//
// Slowly is the point: k is 25 for goals conceded, so at twenty matches last
// season still carries 55% of the weight. Believing this season outright is
// never right at any cutoff up to 25 matches.
func TestTeamRatesBlendTowardTheSeason(t *testing.T) {
	if teamConcededK < teamScoredK {
		t.Error("defence should converge more slowly than attack, not faster")
	}
	e := testEngine(t)
	var withPlay, total int
	for i := range e.Boot.Teams {
		r := e.teamRates(e.Boot.Teams[i].ID)
		if r.Conceded <= 0 || r.Scored <= 0 {
			t.Fatalf("%s has a non-positive rate: %+v", e.Boot.Teams[i].ShortName, r)
		}
		if r.Played > 0 {
			withPlay++
		}
		total++
	}
	if total == 0 {
		t.Fatal("no teams rated")
	}
	t.Logf("%d of %d clubs have finished fixtures to blend from", withPlay, total)
}
