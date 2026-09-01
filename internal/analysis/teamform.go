package analysis

import (
	"math"
	"os"
	"strconv"
	"sync"
)

// Track a club's attack faster than its players' own rates do.
//
// # What this is for
//
// The expected-points review's Gap 3 asks whether a club's players' expected
// goals should be anchored to the club's total. The static form of that — learn a
// fixed per-club offset from history and apply it forward — is closed: a club's
// error correlates with its own next season at −0.232, so an offset fitted on
// history would carry last year's noise into this year rather than correct
// anything. See `TestDiagTeamGoalShare`.
//
// What survived is a different claim with a different clock. Predicting a club's
// expected goals over the closing stretch of a season, a half-and-half blend of the
// model with the club's own preceding nine gameweeks beats the model by 13.1%
// pooled, and the optimum is *interior* — 25% and 75% are both worse than 50% —
// which is a shape rather than an argmax over a sweep. Recency alone beats the
// model outright.
//
// The mechanism is **lag, not bias**. A player's rates blend this season against a
// prior at a fixed weight, so a club that changes character mid-season — a manager,
// a signing, a shift in how it plays — is tracked slowly by construction, and every
// player at that club is tracked slowly in the same direction at the same time. The
// repair for a delay is a shorter memory, not a constant.
//
// # Why this is off by default, and what would turn it on
//
// The prediction result **does not resolve when clustered by season**: +22.3% /
// −13.7% / +29.4% / −4.2%, mean 8.5% at t = 0.82 on three degrees of freedom. And
// it is club-level expected-goals prediction rather than points, so the standing
// trap applies in full — this project has a recorded case of a 2% better predictor
// costing about 49 points a season, because the transfer search is an argmax and
// reaches for whatever the model most over-rates.
//
// That trap is the reason this is a knob rather than a change. A between-club level
// shift is an ordering change of the **dangerous** kind: nothing forces you to own
// players from any particular club, so it reorders the whole pool — unlike a
// position-wide bias, which FPL's five-defenders rule makes harmless. Only the
// replay can arbitrate it.
//
// # The approximation, stated rather than hidden
//
// The measured blend is `model^(1-w) x recent^w`, which as a multiplier on the
// model is `(recent/model)^w`. Computing the model's own club total inside scoring
// would mean scoring every player before scoring any player, so this uses the
// club's **season-to-date** realised expected goals in place of the model's total.
//
// That substitution is close and it was measured rather than assumed: across 80
// club-seasons the model's club total against realised runs at a mean ratio of
// 1.041. The model's per-player rates are largely season-to-date realised rates
// scaled by expected minutes, and a club's expected minutes sum to 980 against the
// 990 eleven players for ninety minutes requires, so the two agree at club level by
// construction more than by luck. What the ratio therefore measures is **recent
// against season-long**, which is what "track the club faster" means.
//
// # The shape of the correction
//
// It multiplies the *input* — XG90 and XA90 — rather than the answer, exactly as
// TeamXGCFactor does on the defensive side. Everything downstream then recomputes
// from one corrected number instead of having a correction pasted onto a total, and
// the factor is reported on PlayerMetrics because every scoring term in this
// project is a reported multiplier.
//
// Assists move with goals rather than being left alone. A club-level attacking
// shift is a statement about how much the side creates, and the alternative —
// correcting goals while leaving assists on the old level — would silently reweight
// creators against finishers within the club, which is a within-team ordering
// change nothing here measured and the blend was never about.

// TeamFormSource supplies a club's realised expected goals per match over a
// trailing window and over the season to date, point-in-time.
//
// Nil by default, like Engine.Recent and for the same reason: a source that cannot
// be built must degrade to the shipped model rather than break it. The replay feeds
// it from the archive. Live it is unwired, deliberately — there is no point paying
// for a live feed of a correction that has not been shown to earn points.
type TeamFormSource interface {
	// TeamForm returns expected goals per match over the trailing window and over
	// the season so far, along with the match counts in each window. ok is false
	// when the club has too little history for the ratio to mean anything, and the
	// caller must then apply no correction at all rather than a neutral one — those
	// are the same number here, but they are not the same statement, and a later
	// reader should not have to guess which was intended. The match counts are
	// provided so the caller can enforce the minimum-match threshold without
	// relying on the implementation to gate them.
	TeamForm(teamID int) (recent, season float64, recentMatches, seasonMatches int, ok bool)
}

// teamFormWeight is w in (recent/season)^w. Zero ships and is the whole feature
// off; 0.5 is the value the prediction work found, and the measurement that found
// it does not resolve.
//
// FPL_TEAM_FORM is the sweep knob. It is read once at startup and only read
// afterwards, so the concurrent tool runner is safe.
var teamFormWeight = func() float64 {
	v, err := strconv.ParseFloat(os.Getenv("FPL_TEAM_FORM"), 64)
	if err != nil || v < 0 {
		return 0
	}
	return v
}()

// SetTeamFormWeight overrides it for a diagnostic sweep. Not safe to call
// concurrently with scoring — sweeps run their variants sequentially.
func SetTeamFormWeight(v float64) { teamFormWeight = v }

// TeamFormWeight reports the live value, so a diagnostic can print the setting it
// ran under rather than the one it believes it set.
func TeamFormWeight() float64 { return teamFormWeight }

// The correction is clamped, because a ratio of two small numbers is not evidence.
// A club with a quiet nine gameweeks against a loud season, or the reverse, can
// produce a ratio well outside anything football supports, and an unclamped power
// of it would move every player at that club at once. The bounds are deliberately
// wider than the measured spread — the observed club ratios run about 0.48 to 1.84
// — so the clamp catches arithmetic accidents rather than quietly becoming the
// effect being measured. `TestTeamFormClampIsNotDoingTheWork` fails if it binds on
// a material share of clubs.
const (
	teamFormMin = 0.60
	teamFormMax = 1.60
	// A club needs at least this many matches in each window before the ratio is
	// used at all. Below it the denominator is a handful of games and the
	// correction is mostly noise about noise.
	teamFormMinMatches = 4
)

// teamFormFactors is the per-club attacking multiplier, computed once.
//
// sync.Once for the reason teamRates already is: the tool runner scores
// concurrently, and this is derived state that every scoring call reads. Building
// it eagerly in NewEngine would pay for it in the many runs where the feature is
// off.
type teamFormFactors struct {
	once sync.Once
	by   map[int]float64
}

// teamFormFactor is the multiplier for one club, or 1 when the feature is off or
// the club has too little history.
func (e *Engine) teamFormFactor(teamID int) float64 {
	if teamFormWeight <= 0 || e.TeamForm == nil {
		return 1
	}
	e.teamForm.once.Do(func() {
		by := map[int]float64{}
		for i := range e.Boot.Teams {
			id := e.Boot.Teams[i].ID
			recent, season, recentMatches, seasonMatches, ok := e.TeamForm.TeamForm(id)
			if !ok || recent <= 0 || season <= 0 ||
				recentMatches < teamFormMinMatches || seasonMatches < teamFormMinMatches {
				continue
			}
			by[id] = clamp(math.Pow(recent/season, teamFormWeight),
				teamFormMin, teamFormMax)
		}
		e.teamForm.by = by
	})
	if f, ok := e.teamForm.by[teamID]; ok {
		return f
	}
	return 1
}
