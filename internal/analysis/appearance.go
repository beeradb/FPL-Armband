package analysis

import (
	"math"
	"os"
)

// Whether a player features at all, and whether he lasts the hour.
//
// FPL pays on two events rather than in proportion to time on the pitch, so the
// model needs two probabilities:
//
//   - P(appears) — one appearance point, and it also prices every bench slot,
//     since a substitute is brought on only when a starter records NO minutes, and
//     it sets the exposure the defensive-contribution threshold is computed at.
//     Three consumers: appearanceFactor, blankRate (hence the bench weights), and
//     defconPerGameweek.
//   - P(reaches 60) — the second appearance point, and the clean sheet, which FPL
//     pays nothing of below the hour. Two consumers, both in Score.
//
// # Why this file exists: one quantity had two estimators
//
// P(appears) used to be computed twice, from two different statistics, with
// nothing requiring the two to agree:
//
//   - playsAtAll(ExpectedMinutes), reached by appearanceFactor;
//   - 1 - blankFromNotStarting x (1 - StartShare), reached by the derived bench
//     slot weights and by defconPerGameweek.
//
// They disagree badly, and worst for players nothing else about the model finds
// remarkable. The worked case: a player who never starts but comes on for
// forty-five minutes every week appears in EVERY gameweek, and the start-share
// estimator calls him a 62.4% blanker while the minutes one says he appears 71% of
// the time. Neither is right about him — mean minutes cannot tell him from someone
// who starts half the season and plays ninety — but they are wrong by 0.336 in
// opposite directions. TestTheTwoEstimatorsGenuinelyDisagreed pins that case.
//
// Which one to keep is not a matter of taste. Measured against the realised
// appearance rate over 22,939 windowed predictions — estimate from the season to a
// cutoff, score against the next five gameweeks, which is what the model does — see
// TestDiagStartShare section 1:
//
//	estimator                 mean signed error    rms
//	1 - blankRate                     +0.177      0.351
//	playsAtAll                        -0.024      0.269
//
// The start-share estimator was biased upward by about eighteen points of
// probability and carried 30% more spread. (On season aggregates, where each
// statistic has 36 gameweeks behind it rather than six, the same comparison reads
// +0.080 against -0.010 and 0.178 against 0.111 — smaller in level, same in
// direction and ordering.) Its constant is the reason: blankFromNotStarting was
// fitted through the origin over start share 0.70 and up — "the regime an eleven
// occupies", which is all slotProbabilities ever asks about — and defconPerGameweek
// then applied it to every player in the pool, including the fringe band where the
// measured ratio is 0.91 rather than 0.624.
//
// This is the DefaultBenchWeight-against-Weights.BenchWeight shape: two defaults for
// one quantity, so the measured one is not the one that runs. Now there is one, and
// three tests hold it there — TestBlankRateIsTheAppearanceEstimator walks the pool,
// TestBlankFromNotStartingIsConfinedToThisFile stops a third estimator arriving by
// someone reaching for the old constant again, and TestTheSwitchReachesEveryConsumer
// counts the consumers.
//
// FPL_NO_UNIFIED_APPEARANCE=1 restores the two-estimator behaviour exactly, which a
// test pins. Note it restores TWO estimators on purpose: under that flag
// appearanceFactor keeps reading playsAtAll while blankRate goes back to start
// share, because that disagreement IS the old behaviour.
//
// # Start share is not a second sufficient statistic, and that was measured
//
// The obvious next step is to predict both probabilities from minutes AND starts
// rather than from minutes alone, since mean minutes cannot tell a player who plays
// twenty minutes every week from one who plays ninety once a month. That was built
// as a start / substitute / unused multinomial and it does not pay.
//
// Mean minutes and start share correlate at 0.9934 in the population the model
// faces, so the second statistic is very nearly the first one again. Judged by
// leave-one-season-out on the task the model actually performs — predict the next
// five gameweeks from the season to a cutoff — adding start share to the shipped
// functional forms buys 0.4% on P(appears) and 0.1% on P(60+), against 2.0% and 3.9%
// for refitting the constants of those same forms with no new input.
//
// The descriptive claim needs the same care, because the obvious version of it is
// wrong. Hold mean minutes in a band, split by start-share tercile, and the realised
// sixty-minute rate does rise by about 0.11 in every band — but the FIT rises too,
// because start share and mean minutes stay correlated inside a band. What mean
// minutes cannot express is only the residual, measured spread minus fitted spread,
// and that reads +0.050 / +0.013 / -0.018 / +0.003 / +0.043 across the five bands:
// a few hundredths, not monotone, and in one band the fit over-reacts. So the split
// that motivated this work mostly measures the collinearity rather than an ordering
// error, which is why a start-aware fit buys 0.14% rather than something worth having.
//
// Read that as the useful lesson rather than as a null result. "Hold X fixed and vary
// Y" does not isolate Y when X and Y correlate at 0.9934: the control has to be what
// the model predicts at each cell, not the band label.
//
// See TestDiagStartShare in internal/backtest, which fails if start share ever does
// start paying, so this stays a live claim rather than a frozen comment.

// playsSixty is P(this player reaches sixty minutes in a gameweek), fitted
// against mean minutes per gameweek over four seasons.
//
// A logistic is the natural shape: bounded, monotone, and the outcome is a
// proportion. 1/(1+exp(-0.065(m-48))) fits with an rms of 0.045 across 2,934
// player-seasons. The value it replaces — the minutes reliability rating — is
// closer than it has any right to be, since a convex power of the minutes share
// happens to be S-shaped too, but it is biased low everywhere and worst in the
// middle: at 65 mean minutes it credits 0.668 where the real rate is 0.716.
//
// The predictor is mean minutes per gameweek because that is precisely what
// blend.MinutesPerMatch carries — blanks included, already recency-weighted,
// already blended against the prior and already rested.
func playsSixty(meanMinutes float64) float64 {
	if meanMinutes <= 0 {
		return 0
	}
	p := 1 / (1 + math.Exp(-sixtySlope*(meanMinutes-sixtyMidpoint)))
	// Two exact bounds, both from 0 <= X <= 90 with mean m, and the fit is held
	// between them. They matter because a logistic is wrong at both ends: it
	// reads 0.045 at a mean of one minute where the truth is 0.007, and it
	// saturates around 0.94 so it can never say that a player who is on the
	// pitch for every minute reaches sixty every week.
	//
	// Above: Markov, P(X >= 60) <= m/60. Without it a player who has never
	// played would collect a slice of appearance points forever, and the "no
	// Premier League data scores 0.00" property research_targets is built on
	// would quietly stop holding.
	//
	// Below: X can never exceed 90, so m <= 60(1-p) + 90p gives p >= (m-60)/30.
	// At a mean of ninety that forces 1, which is the only right answer.
	if hi := meanMinutes / 60; hi < p {
		p = hi
	}
	if lo := (meanMinutes - 60) / 30; lo > p {
		p = lo
	}
	return clamp(p, 0, 1)
}

// The shipped fit, kept as constants so the override below has something exact to
// fall back to and to restore. Nothing reads these directly except the four
// variables under them — read those.
const (
	shippedSixtySlope    = 0.065
	shippedSixtyMidpoint = 48.0

	// The conditional-mean fit behind playsAtAll, in minutes:
	// E[minutes | he appears] ~ condMinutesIntercept + condMinutesSlope x mean
	// minutes. Measured by least squares over 2,217 player-seasons; see playsAtAll
	// for why it is an identity rather than a curve, and for why the start-aware
	// replacement this was once queued behind was measured and rejected.
	shippedCondMinutesIntercept = 28.15
	shippedCondMinutesSlope     = 0.779
)

// The four numbers the two curves actually run on.
//
// They are variables rather than constants for one reason: `TestDiagStartShare`
// reports that refitting these forms — with no new input at all — is worth 2.0% on
// P(appears) and 3.9% on P(60+) out of sample, four to twenty times what the
// start-share input it rejected was worth, and there was no way to run that refit
// through anything. A constant nobody can vary is a constant nobody measures, which
// is the reason `FPL_BENCH_SLOTS` and `FPL_DEF_FIXTURE_SCALE` exist too.
//
// **Refitting them is not obviously right, and that is why this is a knob rather
// than a change.** The refit is fitted on a windowed proxy — mean minutes measured
// over a six-gameweek history — while what the model feeds in is
// `ExpectedMinutes`: blended against the prior, recency-weighted and rested. A
// curve fitted against a noisier predictor than the one it will be evaluated at is
// over-hedged by construction, so the flatter fit the windowed data prefers is
// probably too flat for this consumer. Screening that on the prediction benchmark,
// where the input IS `ExpectedMinutes`, is the point of the knob.
//
// Set FPL_APPEARANCE_FIT, or call SetAppearanceFit. Written once at startup and
// only read afterwards, so the concurrent tool runner is safe; the setter is not.
var (
	sixtySlope, sixtyMidpoint, condMinutesIntercept, condMinutesSlope = appearanceFit(
		shippedSixtySlope, shippedSixtyMidpoint,
		shippedCondMinutesIntercept, shippedCondMinutesSlope)
)

// playsAtAll is P(this player records at least one minute in a gameweek), the
// sibling of playsSixty and the thing the model needed in order to pay the
// appearance point FPL gives for turning up.
//
// # An identity, not a curve
//
// Since E[minutes] = P(appears) x E[minutes | appears], the probability is mean
// minutes divided by the mean minutes he plays *when* he appears — and the
// conditional mean is a much better-behaved thing to fit than a probability
// pinned between 0 and 1. Fitted by least squares over 2,217 player-seasons
// (four seasons, single-fixture gameweeks only, so minutes are per match):
//
//	E[minutes | appears] ~ 28.15 + 0.779 x mean minutes
//
// giving rms 0.1112 against the measured appearance rate, against 0.1166 for a
// logistic in the same form as playsSixty. The decisive advantage is not the rms:
// the identity reads exactly 0.0000 at zero mean minutes *by construction*, where
// the logistic needs a Markov bound bolted on top to stop it paying a footballer
// who has never kicked a ball. Where the identity does err at the bottom it
// under-credits — 0.055 against a measured 0.095 — while the logistic
// over-credits by nearly double, at 0.186. Under-crediting is the safe direction
// for exactly the players about whom nothing is known.
//
//	mean minutes    measured   identity   logistic
//	  0 -  5           0.095      0.055      0.186
//	 15 - 25           0.446      0.457      0.385
//	 45 - 55           0.746      0.743      0.763
//	 85 - 91           0.988      0.979      0.964
//
// # "Neither fit is good" was reading the wrong number, and the queued fix was
// measured and refuted
//
// This comment used to say the term was provisional pending a start / substitute /
// unused split using `starts` as well as minutes, on the reasoning that an rms of
// 0.1112 against the sixty-minute curve's 0.045 must mean the predictor is one
// statistic short. Both halves of that turned out to be wrong.
//
// **The rms was being read against the wrong floor.** The target is a binomial
// proportion over the ~36 gameweeks his club played, so even a perfect model cannot
// beat sqrt(mean p(1-p)/n) — which is 0.065 for the appearance rate and 0.062 for
// the sixty-minute rate. The appearance rate sits near 0.5 for much of the range
// where the sixty-minute rate is bimodal, so the two curves were never comparable:
// 0.1112 against a 0.065 floor is a different situation from 0.045 against 0.062,
// and the second is BELOW its pooled floor because most of its mass is at the ends.
// The conflation is real; "this fit is bad" was mostly the sampling noise in the
// thing being fitted.
//
// **And the split does not pay.** It was built and measured by leave-one-season-out
// on the task the model performs. Start share correlates with mean minutes at 0.9934
// in this population, and adding it buys 0.4% on P(appears) and 0.1% on P(60+)
// against the same forms refitted without it. See the note at the top of this file
// and TestDiagStartShare, which fails if that ever stops being true.
//
// So this term is no longer provisional in the sense of awaiting a known better
// version.
//
// # The refit this comment used to defer: measured, and the deferral was right
//
// What was left open was smaller and different — refitting these two constants on a
// windowed population rather than on season aggregates looked worth about 2%, and was
// not done here because the windowed proxy is noisier in its input than the blended,
// recency-weighted, prior-shrunk ExpectedMinutes the model actually feeds in, so the
// flatter curve it prefers is probably over-hedged. That was a prediction, and it has
// now been run rather than left as a reason.
//
// It holds, and the size is larger than the hedge implied. `TestDiagStartShare`
// section 6 fits the same forms against ExpectedMinutes itself and scores all three
// curves on that axis:
//
//	fitted on                              P(appears)   P(60+)
//	the windowed proxy, scored on itself       +2.0%     +3.9%
//	the windowed proxy, scored on E[minutes]   -4.1%     +0.1%
//	E[minutes], scored on E[minutes]           -0.0%     +1.3%
//
// The queued refit is WORSE than what ships on the predictor that is actually used.
// Done honestly it is a wash on P(appears) — this identity is already at the optimum,
// which is the strongest evidence these constants have — and worth about 1.3% on
// P(60+). Both fits were then run through the prediction benchmark, where each is a
// bias-for-variance trade that lowers the rank correlation, so neither earned replay
// time. Nothing changed.
//
// The transferable part is not about minutes: **a constant fitted against a proxy for
// its input is fitted to the proxy's noise as well as to the relationship.** Refit
// against what the model will feed the curve, or do not refit. FPL_APPEARANCE_FIT
// exists so this stays runnable rather than becoming prose again.
func playsAtAll(meanMinutes float64) float64 {
	if meanMinutes <= 0 {
		return 0
	}
	// Two exact bounds hold the fit, as they do in playsSixty. A conditional mean
	// cannot exceed ninety minutes — which forces P = 1 for an ever-present, where
	// the bare fit reads 0.916 — and cannot fall below a minute, which is what
	// stops the division running away.
	cond := clamp(condMinutesIntercept+condMinutesSlope*meanMinutes, 1, 90)
	p := clamp(meanMinutes/cond, 0, 1)
	// Reaching sixty minutes implies appearing, so P(appears) >= P(60+). The two
	// fits are independent and cross by up to 0.002 in a narrow band around 79
	// mean minutes; this makes an impossible ordering impossible rather than rare.
	if s := playsSixty(meanMinutes); s > p {
		p = s
	}
	return p
}

// appearanceFactor converts the per-90 appearance term into what FPL actually
// pays over a gameweek, expressed as a multiple of that per-90 value.
//
// FPL pays 1 point for playing at all and 2 at sixty minutes or more, so the
// expectation is P(appears) + P(reaches 60) — one point for turning up, one more
// for the hour. The per-90 term carries the full 2, so the factor is that
// expectation divided by 2.
//
// # What this fixes
//
// The model had only the upper branch: it scaled the whole appearance term by
// P(60+), which pays a player who appears for fifty minutes *nothing* where FPL
// pays him one. The shortfall is exactly P(appeared and did not reach the hour),
// and that population is not small — 21% to 27% of available gameweeks for
// players averaging 5 to 35 minutes:
//
//	mean minutes    old (2 x P60)   correct   difference
//	  5 - 15               0.153     0.339       +0.186
//	 25 - 35               0.465     0.748       +0.283
//	 55 - 65               1.363     1.529       +0.166
//	 85 - 91               1.875     1.901       +0.026
//
// Pooled, +0.188 appearance points per gameweek, peaking at +0.283 around 25-35
// mean minutes.
//
// The top row and the pooled figure moved slightly — from 1.863 / 1.895 / +0.032
// and +0.185 — when the diagnostic that produced them stopped using its own copy
// of the sixty-minute curve. That copy omitted both exact bounds, so it saturated
// around 0.94 where the shipped floor forces 1.0 at ninety minutes, and the error
// was confined to the top band. Nothing about the argument changes. That is the fringe and rotation population, which is also the
// population the squad-pool floor now admits — so it is measured against the
// current pool rather than an older one.
//
// It remains exactly zero for a player with no minutes, because both
// probabilities are. FPL_NO_SHORT_PLAY=1 restores the old single-branch
// behaviour.
func appearanceFactor(meanMinutes float64) float64 {
	if !shortPlayCredit {
		return playsSixty(meanMinutes)
	}
	return (shortPlayPoints*playsAtAll(meanMinutes) +
		(appearancePoints-shortPlayPoints)*playsSixty(meanMinutes)) / appearancePoints
}

// appearsInGameweek is P(this player records at least one minute this gameweek).
//
// It is the single estimator named at the top of this file. Every consumer of that
// quantity reads it: appearanceFactor pays the appearance point through
// playsAtAll, blankRate is one minus it, and defconPerGameweek uses it both as a
// multiplier and as the denominator that turns mean minutes into mean minutes
// *when he appears*.
//
// The predictor is mean minutes per gameweek, which is what blend.MinutesPerMatch
// carries — blanks included, already recency-weighted, already blended against the
// prior and already rested. See playsAtAll for the fit and for why it is an
// identity rather than a curve.
func appearsInGameweek(m PlayerMetrics) float64 {
	appears, _ := appearanceOdds(m)
	return appears
}

// appearanceOdds returns P(appears) and P(blanks) together, and it is the only
// function in the package that chooses between the two rules.
//
// Returning the pair from one place is not tidiness. Each rule has a natural
// primitive — the unified one computes P(appears) and the legacy one computes the
// blank rate — so whichever is derived costs a subtraction, and 1-(1-p) is not p in
// binary floating point. Deriving in two separate functions would therefore make the
// escape hatch inexact in one direction, on a model whose response surface is a step
// function: AGENTS.md records a 2% nudge to one exponent moving four-season points by
// 67. Here each rule returns its own primitive exactly and the complement once.
//
// It is also the seam that a consumer must not bypass. The first version of this
// change had defconPerGameweek reading the unified estimator directly, so
// FPL_NO_UNIFIED_APPEARANCE restored the old bench weights and left the defcon
// exposure on the new rule — an escape hatch that silently reproduced neither
// behaviour, which is the "wired two engines and missed the third" failure this
// project has shipped before. TestTheSwitchReachesEveryConsumer counts them.
func appearanceOdds(m PlayerMetrics) (appears, blank float64) {
	if !unifiedAppearance {
		blank = clamp(blankFromNotStarting*(1-clamp(m.StartShare, 0, 1)), 0, 1)
		return 1 - blank, blank
	}
	appears = playsAtAll(m.ExpectedMinutes)
	return appears, 1 - appears
}

// blankFromNotStarting is the legacy second estimator's constant: the share of
// non-starts in which the player records no minutes at all.
//
// TestDiagBlankRate measures the ratio as U-shaped — fringe players who do not
// start mostly do not play at all (0.91), rotation players come on (0.51), and a
// near-nailed player who misses a start is usually out altogether (0.69 to 0.80).
// 0.624 is the fit through the origin over start share 0.70 and up, which is what
// an eleven is made of.
//
// **That range is the whole problem.** slotProbabilities only ever asks about an
// eleven, so the constant was honest there; defconPerGameweek asked about every
// player in the pool, where it is not. It is retained only for
// FPL_NO_UNIFIED_APPEARANCE, and TestBlankFromNotStartingIsConfinedToThisFile
// fails if anything else reaches for it — a third estimator of one quantity is the
// failure this file exists to prevent.
const blankFromNotStarting = 0.624

// unifiedAppearance routes blankRate through the single appearance estimator
// rather than through a second, independent fit in start share. Set
// FPL_NO_UNIFIED_APPEARANCE=1 to restore the second estimator and re-measure.
var unifiedAppearance = os.Getenv("FPL_NO_UNIFIED_APPEARANCE") == ""

// blankRate is P(this player records no minutes this gameweek) — definitionally the
// complement of appearsInGameweek, and computed alongside it so the two cannot
// disagree even by a rounding error.
func blankRate(m PlayerMetrics) float64 {
	_, blank := appearanceOdds(m)
	return blank
}

// metricsWithBlankRate returns a player whose blank rate is exactly the target
// under whichever rule is in force.
//
// benchSlotScale needs one: it converts slot probabilities onto BenchWeight's
// scale by pricing a reference eleven, and BenchWeight was swept on the basis that
// the four slots of such an eleven sum to four. Specifying the reference by the
// blank rate it must have — rather than by a start share, which only one of the
// two rules reads — is what keeps benchSlotScale numerically identical across this
// change, so BenchWeight keeps meaning what it was calibrated to mean.
// TestBenchSlotScaleSurvivesTheUnification pins that.
func metricsWithBlankRate(target float64) PlayerMetrics {
	if !unifiedAppearance {
		return PlayerMetrics{StartShare: 1 - target/blankFromNotStarting}
	}
	// playsAtAll is monotone in mean minutes, so bisect. Solving rather than
	// hardcoding the answer means the reference follows the fit if the fit ever
	// changes, instead of silently ceasing to be a reference.
	want := 1 - target
	lo, hi := 0.0, 90.0
	for i := 0; i < 200; i++ {
		mid := (lo + hi) / 2
		if playsAtAll(mid) < want {
			lo = mid
		} else {
			hi = mid
		}
	}
	return PlayerMetrics{ExpectedMinutes: (lo + hi) / 2}
}
