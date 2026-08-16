package analysis

import "testing"

// thinSeasonMinutes was the FOURTH declaration of 1710 in the repository, beside
// priors.ThinSeason, recent.ThinSeason and backtest's unexported thinPrior. It is
// now an alias of the one that ships.
//
// The bar and the gate both live in this package (ThinSeason, ShouldBlendPrior),
// because the three packages that express the rule cannot import one another but
// all import this one. Each has a TestTheBlendGateIsThinAndNonZero checking its
// own path against ShouldBlendPrior, so the rule has one statement and three
// witnesses.
//
// Note what has NOT changed and is the point of the test below: the analysis
// layer still has no thinness gate in the SCORING path. blendRates decides
// between believing a prior and shrinking to the league on presence, not on
// thinness, and ShouldBlendPrior is nowhere near it — it decides what goes INTO
// the prior index, several layers up.
const thinSeasonMinutes = ThinSeason

// TestAThinPriorIsBelievedRatherThanShrunk pins the fact the prior-blend
// experiment turned on, and which two written specifications of that experiment
// got backwards.
//
// # The claim that was wrong
//
// The work queue and the brief for the experiment both said that a raw thin-season rate
// is a strawman baseline, "because shrinkToLeague already pulls a thin season
// toward league rates". It does not. The gate in blendRates is
//
//	p, ok := e.Priors.Get(el.Code)
//	if !ok || p.Minutes == 0 { return e.shrinkToLeague(el, b) }
//
// which is presence, not thinness. A player with ninety minutes to his name last
// season has a prior and is believed at face value, mixed with this season at
// BlendRateK; only a player with NO prior, or one recorded at exactly zero
// minutes, meets the league rate at all.
//
// That distinction decides the experiment. It is why blending an older season in
// helps the injury-shaped population — nothing else was protecting them — and why
// it hurts the players who recorded no minutes at all, for whom the shipped model
// already has a defensible answer and the blend replaces it with a stale one.
//
// # Why it is a test rather than a comment
//
// Because it has now been stated wrongly twice in writing, and because the check
// is cheap: an assertion that reads the shipped code cannot drift away from it.
func TestAThinPriorIsBelievedRatherThanShrunk(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	el := findEverPresent(t, e)
	mins, starts, bonus := el.Minutes, el.Starts, el.Bonus
	defer func() { el.Minutes, el.Starts, el.Bonus = mins, starts, bonus }()

	// One match played, and three bonus points in it. Raw, that reads as three
	// bonus every gameweek; the league rate for a midfielder is a fraction of
	// one, so the two branches are far apart and cannot be confused.
	playGameweeks(t, e, 1)
	el.Minutes, el.Starts, el.Bonus = 90, 1, 3

	// A prior nobody would call a season, carrying a rate no league average is
	// near: about 12.97 bonus per 90, held roughly constant as the minutes vary
	// so that only the thinness changes. Bonus is a whole number, so the thinnest
	// case is one match rather than one minute — below that the rate cannot be
	// expressed at all, which is a property of the fixture and not of the model.
	for _, c := range []struct{ minutes, bonus int }{
		{90, 13}, {300, 43}, {694, 100}, {thinSeasonMinutes - 1, 246},
	} {
		priorMinutes := c.minutes
		e.Priors = fakePriors{el.Code: {Minutes: c.minutes, Starts: 1, Bonus: c.bonus}}
		m := e.Metrics(el)
		if m.Bonus90 < 5 {
			t.Errorf("prior of %d minutes: bonus/90 came back %.2f. The prior carried "+
				"about 12.97 bonus per 90 and was believed at far less than that, which "+
				"means it went through shrinkToLeague. The shipped gate is presence, "+
				"not thinness.", priorMinutes, m.Bonus90)
		}
	}

	// The other side of the same gate. A prior recorded at zero minutes is the
	// same fact as no prior at all — he did not play — and both meet the league.
	for _, c := range []struct {
		name string
		p    fakePriors
	}{
		{"no prior at all", fakePriors{}},
		{"a prior of exactly 0 minutes", fakePriors{el.Code: {Minutes: 0, Bonus: 100}}},
	} {
		e.Priors = c.p
		m := e.Metrics(el)
		if m.Bonus90 > 1 {
			t.Errorf("%s: bonus/90 came back %.2f. With no usable prior a single "+
				"three-bonus appearance must be pulled to the league rate, or the "+
				"replay pays a hit for a debutant again.", c.name, m.Bonus90)
		}
	}
}
