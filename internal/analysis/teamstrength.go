package analysis

import (
	"math"

	"armband/internal/fpl"
)

// How good is a club, in goals rather than in rank?
//
// FPL's fixture difficulty is an integer 1 to 5, set pre-season and barely
// moved, so every difficulty-2 opponent gets the same treatment. The tail is not
// evenly spaced: the three leakiest defences concede 31-70% more than the median
// and attackers return about +23% against them, against 0.97 versus everyone
// else. A rank cannot say the 20th defence is twice as bad as the 19th.
//
// # The prior has to exist live
//
// A blend needs last season's goals and the FPL API publishes no team history —
// which is why this uses FPL's own pre-season strength rating instead. That is
// available for every club including a promoted one, in the archive and live,
// and TestDiagStrengthToGoals fits it to goals across 80 club-seasons:
//
//	conceded = -0.00272 x strength_defence + 4.671   R2 0.479, rms 0.264
//	scored   = +0.00356 x strength_attack - 2.581    R2 0.529, rms 0.288
//
// Against 0.365 and 0.420 for guessing the league average, so the rating is
// worth roughly 28% of the error. It also solves the promoted-club case for
// free: they have no Premier League record but they do have a rating.
//
// # How fast to believe this season
//
// Measured out of sample in TestDiagTeamBlend, predicting the remainder of the
// season from the record so far. Believing this season outright is never right
// at any cutoff up to 25 matches; after three games it is more than twice the
// error of ignoring the season entirely. A team wants k around 25 for goals
// conceded and 13 for goals scored — far heavier than BlendRateK's 8 for player
// rates. At twenty matches played, last season still carries 55% of a club's
// defensive rating.
//
// Attack converges faster than defence, which is why the two take different
// constants.
//
// # Fitted against the prior that actually runs
//
// Those first constants — 25 and 13 — were measured against a prior built from
// *last season's goals*, which is not the prior this file uses. Re-fitted in
// TestDiagTeamBlendPriorSource against priorFromStrength itself, the answer is
// roughly three times heavier, because FPL's rating is a **better** prior than
// last season's goals and a better prior earns more weight:
//
//	rms predicting the rest of the season, prior alone
//	                   last season's goals   FPL rating
//	3 matches played   0.3201                0.2769
//	8 matches played   0.3232                0.2860
//	scored, 3 played   0.3602                0.2670
//
// That is 13% better on goals conceded and 26% on goals scored, which is what
// you would expect: the rating prices transfers, a new manager and promotion,
// none of which last season's goals can see.
//
// Best k by cutoff runs 100/100/70/100/70/70/45 for conceded and
// 15/30/45/30/45/45/30 for scored. A single value reproduces each to within
// 0.001 rms at every cutoff, so one constant per side is still enough. The
// qualitative finding survives — defence wants a heavier prior than attack —
// but both roughly triple.
//
// Note this changes nothing that ships: teamRates is consumed only by
// magnitudeAttack and magnitudeDefence, which sit behind FPL_MAGNITUDE and are
// off by default. It is a correctness fix to a dormant feature.
const (
	teamConcededK = 70.0
	teamScoredK   = 35.0

	strengthToConceded, strengthToConcededBase = -0.00272, 4.671
	strengthToScored, strengthToScoredBase     = 0.00356, -2.581
)

// TeamRates is a club's goals for and against per match, blended between FPL's
// pre-season rating and what it has actually done this season.
type TeamRates struct {
	Scored   float64
	Conceded float64
	// Played is how many finished matches the current-season half rests on.
	Played int
}

// teamRates returns the blended rates for one club, building the whole table
// once. Built under a sync.Once because the tool runner scores the pool from
// several goroutines at once and an unguarded map build is a fatal "concurrent
// map writes" rather than a recoverable panic.
func (e *Engine) teamRates(teamID int) TeamRates {
	e.teamOnce.Do(e.buildTeamRates)
	return e.teamStrength[teamID]
}

// LeagueGoals is the average goals conceded and scored per match per club, the
// denominators a magnitude-scaled difficulty is expressed against.
func (e *Engine) LeagueGoals() (conceded, scored float64) {
	e.teamOnce.Do(e.buildTeamRates)
	return e.leagueConceded, e.leagueScored
}

func (e *Engine) buildTeamRates() {
	e.teamStrength = map[int]TeamRates{}

	// This season so far, from finished fixtures only. Gate on
	// FinishedProvisional rather than Finished: goals for/against are the
	// match's own numbers, locked in at the whistle, and Finished can lag full
	// time by 16+ hours on live data (see the doc comment on fpl.Fixture). In a
	// replay, playedFixtures mirrors Finished into FinishedProvisional for
	// revealed weeks and nils both plus the scores past the cutoff — but the
	// pre-season path (PreSeasonWith) returns the archive's fixtures
	// unfiltered, all 380 of them scored and both flags false, so without this
	// gate a GW1 engine would accumulate the whole season it has not seen yet.
	gf, ga, n := map[int]float64{}, map[int]float64{}, map[int]float64{}
	for _, f := range e.Fixtures {
		if !f.FinishedProvisional || f.TeamHScore == nil || f.TeamAScore == nil {
			continue
		}
		gf[f.TeamH] += float64(*f.TeamHScore)
		ga[f.TeamH] += float64(*f.TeamAScore)
		gf[f.TeamA] += float64(*f.TeamAScore)
		ga[f.TeamA] += float64(*f.TeamHScore)
		n[f.TeamH]++
		n[f.TeamA]++
	}

	var totalConceded, totalScored, clubs float64
	for i := range e.Boot.Teams {
		t := &e.Boot.Teams[i]
		priorConceded, priorScored := priorFromStrength(t)

		played := n[t.ID]
		conceded, scored := priorConceded, priorScored
		if played > 0 {
			wc := played / (played + teamConcededK)
			ws := played / (played + teamScoredK)
			conceded = wc*(ga[t.ID]/played) + (1-wc)*priorConceded
			scored = ws*(gf[t.ID]/played) + (1-ws)*priorScored
		}
		e.teamStrength[t.ID] = TeamRates{
			Scored: scored, Conceded: conceded, Played: int(played),
		}
		totalConceded += conceded
		totalScored += scored
		clubs++
	}
	// Two denominators, not one. Every goal conceded is a goal scored, so in
	// reality these are the same number — but the priors are fitted separately
	// and need not balance, and dividing the defensive side by the mean
	// *conceded* deflated every defensive multiplier. Caught by a probe showing
	// 0.754 where 0.86 was right.
	e.leagueConceded, e.leagueScored = leagueAverageGoals, leagueAverageGoals
	if clubs > 0 && totalConceded > 0 {
		e.leagueConceded = totalConceded / clubs
	}
	if clubs > 0 && totalScored > 0 {
		e.leagueScored = totalScored / clubs
	}
}

// leagueAverageGoals is the long-run goals per match per club, used only where
// a club has no rating at all. Measured at 1.48 across 80 club-seasons.
const leagueAverageGoals = 1.48

// coarseConceded and coarseScored map FPL's 1-5 rating to goals per match,
// measured across four seasons.
//
// This exists because the granular ratings are **not populated pre-season**,
// which is exactly when the prior does all the work. Live in August every club
// reads zero for strength_attack_home and strength_defence_home, and the coarse
// value arrives in strength_overall_home while the `strength` field itself is
// null — the archive hides this, because its snapshots are taken once FPL has
// filled the granular numbers in. Discovered by printing the live payload after
// every club came back at exactly the league average.
//
// Note the span: 0.87 to 2.11 goals conceded is a factor of 2.4, which is the
// magnitude an integer 1-5 difficulty cannot express and the whole reason for
// this file. Strength 1 never appears in four seasons; it takes an
// extrapolation rather than a measurement.
var (
	coarseConceded = map[int]float64{1: 2.45, 2: 2.11, 3: 1.49, 4: 1.23, 5: 0.87}
	coarseScored   = map[int]float64{1: 0.75, 2: 0.95, 3: 1.39, 4: 1.87, 5: 2.22}
)

// PriorFromStrength exposes the rating-to-goals map so a calibration can fit
// the blend constant against the prior that actually ships, rather than against
// the last-season-goals prior the constants were first measured on. See
// TestDiagTeamBlendPriorSource.
func PriorFromStrength(t *fpl.Team) (conceded, scored float64) {
	return priorFromStrength(t)
}

// TeamBlendK reports the shipped blend constants, so a calibration measuring
// them does not restate them and drift.
func TeamBlendK() (conceded, scored float64) {
	return teamConcededK, teamScoredK
}

// priorFromStrength turns FPL's rating into goals per match, preferring the
// granular 1000-1400 ratings and falling back to the coarse 1-5 one.
func priorFromStrength(t *fpl.Team) (conceded, scored float64) {
	sDef := float64(t.StrengthDefenceHome+t.StrengthDefenceAway) / 2
	sAtk := float64(t.StrengthAttackHome+t.StrengthAttackAway) / 2
	conceded, scored = leagueAverageGoals, leagueAverageGoals

	// The granular scale runs in the hundreds; anything smaller is the coarse
	// rating leaking into the same field, which is what happens pre-season.
	if sDef > 100 {
		if v := strengthToConceded*sDef + strengthToConcededBase; v > 0 {
			conceded = v
		}
	}
	if sAtk > 100 {
		if v := strengthToScored*sAtk + strengthToScoredBase; v > 0 {
			scored = v
		}
	}
	if sDef > 100 && sAtk > 100 {
		return conceded, scored
	}

	coarse := t.Strength
	if coarse <= 0 || coarse > 5 {
		coarse = t.StrengthOverallHome // where FPL puts it pre-season
	}
	if v, ok := coarseConceded[coarse]; ok && sDef <= 100 {
		conceded = v
	}
	if v, ok := coarseScored[coarse]; ok && sAtk <= 100 {
		scored = v
	}
	return conceded, scored
}

// magnitudeAttack is how much an attacker's returns scale against a given
// opponent, from how leaky that opponent actually is rather than from its rank.
//
// The exponent is what stops it overshooting, and overshooting is precisely how
// the previous attempt failed: re-rating the worst defences to difficulty 1 gave
// them +30% where the measured truth is +23%, and that cost more than
// under-shooting had saved. The three leakiest defences concede roughly 50% more
// than the median and attackers return +23% against them, so the response is
// sub-proportional — 1.5^0.5 = 1.22, which is the measured figure almost
// exactly. Hence a square root rather than a straight ratio.
func (e *Engine) magnitudeAttack(opponentID int) float64 {
	conceded, _ := e.LeagueGoals()
	return magnitudeRatio(e.teamRates(opponentID).Conceded, conceded)
}

// magnitudeDefence is the same for a defender or keeper: how much more his side
// concedes against this opponent, from how potent that opponent actually is.
func (e *Engine) magnitudeDefence(opponentID int) float64 {
	_, scored := e.LeagueGoals()
	return magnitudeRatio(e.teamRates(opponentID).Scored, scored)
}

func magnitudeRatio(rate, league float64) float64 {
	if league <= 0 || rate <= 0 {
		return 1
	}
	// Clamped to the span the FDR ladders already cover, so a freak early-season
	// rate cannot produce a multiplier no evidence supports.
	return clamp(math.Pow(rate/league, magnitudeAlpha), 0.60, 1.60)
}

// DefconTermFor is the points a player's defensive-contribution rate is worth
// per 90, exposed so a calibration can ask how much of the model's estimate the
// term is responsible for.
func DefconTermFor(elementType int, defCon90 float64) float64 {
	return defconPer90(elementType, defCon90)
}

// DefconCleanFactorFor exposes the defcon/clean-sheet coupling, for the same
// reason as the two either side of it: a calibration that omits it is fitting a
// different quantity from the one the engine scores.
//
// It exists because the clean-sheet regressor diagnostic did omit it, and the
// omission was disclosed but unsized for a day. It is 1 for a keeper and for any
// player with no defensive-contribution rate, so a calibration that never meets a
// defender cannot tell whether it matters.
func (e *Engine) DefconCleanFactorFor(elementType int, defCon90 float64) float64 {
	return e.defconCleanFactor(elementType, defCon90)
}

// CleanSheetTermFor is the points a player's expected goals conceded is worth
// per 90 through the clean sheet, exposed for the same reason DefconTermFor is:
// a calibration needs to know how much of the estimate each term owns.
func CleanSheetTermFor(elementType int, xgc90 float64) float64 {
	csPts := cleanSheetPoints[elementType]
	if csPts <= 0 || xgc90 <= 0 {
		return 0
	}
	return cleanSheetProb(xgc90, 1, 1) * csPts
}

// defconClean couples the clean sheet to a defender's own defensive workload.
//
// The model prices the clean sheet from his team's expected goals conceded and
// the defensive contribution from his own action rate, with nothing linking
// them. Measured on 2025-26, that is wrong in a specific and sizeable way: the
// model predicts the *same* clean-sheet value for every defender group —
// 1.016, 0.987, 1.064 per 90 from the lowest defcon third to the highest —
// while what they actually collect is 1.046, 1.059 and 0.825.
//
// A defender clearing ten times a match is on a side under pressure, and xGC
// does not fully know it. So his workload is evidence about the clean sheet
// over and above the team rate, and the coupling folds it in where it belongs:
// it raises the expected goals conceded that the clean-sheet term is computed
// from, rather than docking the answer afterwards.
//
//	xGC_effective = xGC x (dc90 / reference) ^ DefConCleanCoupling
//
// Ships at 0.3 on the mechanism rather than on the points — see the sweep in
// AGENTS.md, where both metrics are positive there but non-monotone. The
// reference is the median defender's rate; the ratio is clamped because the
// correction is fitted on 87 defenders in the only season that has the category
// and must not run away on an outlier.
const defconReference = 7.6

func (e *Engine) defconCleanFactor(elementType int, dc90 float64) float64 {
	g := e.Weights.DefConCleanCoupling
	if g <= 0 || dc90 <= 0 || elementType != 2 {
		return 1
	}
	return clamp(math.Pow(dc90/defconReference, g), 0.75, 1.35)
}

// SavesTermFor is the points a keeper's save rate is worth per 90, exposed for
// calibration alongside DefconTermFor and CleanSheetTermFor.
func SavesTermFor(saves90 float64) float64 {
	if saves90 <= 0 {
		return 0
	}
	return poissonFloorDiv(savesBlock, saves90)
}

// TeamRatesFor exposes a club's blended goals for and against per match.
//
// The rates themselves are the whole of this file's fitted content, and until
// now nothing outside it could read them: the two consumers, magnitudeAttack
// and magnitudeDefence, sit behind FPL_MAGNITUDE and ship off. Reading a rate
// is not the same as scoring a player with it — this accessor does not turn
// the magnitude feature on.
func (e *Engine) TeamRatesFor(teamID int) TeamRates {
	return e.teamRates(teamID)
}

// FixtureGoals projects the goals each side scores in one fixture: a club's own
// rate, scaled by how leaky the opponent actually is.
//
// # What is fitted here, and what is assumed
//
// `teamRates` is fitted: a club's goals per match, blended between FPL's
// pre-season rating and this season's record, with the blend constants
// measured out of sample in TestDiagTeamBlendPriorSource.
//
// The multiplier is the damped `magnitudeAttack` rather than a raw ratio,
// because that is this package's only fitted response to opponent strength and
// a second one would be a second implementation of the same idea.
//
// ⚠️ **But it was fitted against a different quantity than this one.** The
// square root and the [0.60, 1.60] clamp come from ATTACKING RETURNS — FPL
// points from goals, assists and bonus — where re-rating the worst defences
// gave +30% against a measured +23%. Here it scales a club's raw GOALS. Those
// two elasticities need not be equal, and nothing has measured whether they
// are, so the choice is conservative rather than correct: the damped form
// under-reacts to a leaky defence where a raw ratio would over-react, and
// under-reacting is the cheaper error for a number that gets published.
// TestFixtureGoalsUsesTheDampedMultiplierRatherThanTheRawRatio pins which form
// ships; it does not establish that this one is right.
//
// ⚠️ **This is a reporting surface, not a fixture-difficulty rating for
// picking players.** The closed line against a custom FDR was measured on
// replayed points totals, and this changes no scoring path: `magnitudeAttack`
// still reaches Score only under FPL_MAGNITUDE, which still ships off.
//
// The neutral-opponent identity is what keeps the number readable on its own,
// and it is pinned: against an opponent conceding exactly the league average
// the projection IS the club's own scored rate, because the multiplier is 1.
// TestAFixtureAgainstAnAverageOpponentIsTheClubsOwnRate.
//
// ⚠️ **There is no home advantage in this number.** priorFromStrength averages
// FPL's home and away ratings, and the season half is a plain per-match rate
// over both, so swapping which club is at home changes nothing. Stating it
// because the argument order invites the opposite assumption.
//
// ⚠️ **These are projected GOALS, not xG.** Nothing here reads
// `expected_goals`; the season half is goals actually scored and conceded and
// the prior is FPL's strength rating. A caller labelling the output "xG" would
// be naming a different quantity — FPL publishes a real per-player xG and this
// is not built from it.
//
// ⚠️ **It is a projection, so it works before there is a season to read.** At
// GW1 both clubs' rates are their priors, which is the case a backward-looking
// xG table cannot cover at all.
func (e *Engine) FixtureGoals(homeID, awayID int) (home, away float64) {
	return e.teamRates(homeID).Scored * e.magnitudeAttack(awayID),
		e.teamRates(awayID).Scored * e.magnitudeAttack(homeID)
}
