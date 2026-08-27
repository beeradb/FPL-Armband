package backtest

// The out-of-sample prediction benchmark: how wrong is the scoring model about
// one player in one gameweek, and *where* is it wrong?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagPredictionBenchmark -v -timeout 60m
//
// # Why this exists alongside the replay, not instead of it
//
// The replay is a *policy* instrument. Replaying a season means one discrete
// squad choice plus thirty-eight weekly transfer decisions, each an argmax that
// flips on a hair's-breadth change in a score and then changes the squad for
// every following week. Its unit of replication is a replayed season-path, of
// which there are twenty-four, resting on four seasons and therefore three
// degrees of freedom that will never grow.
//
// This is a *prediction* instrument. Its unit is a player-gameweek, of which
// there are tens of thousands. It cannot answer a policy question — the whole
// content of "a better predictor can make a worse policy" is that lower
// prediction error does not imply more points — but it can answer the question
// a policy instrument cannot see: is the model right about football, and about
// which players.
//
// **Read the two together and neither as the other.** A bias-reduction claim is
// a calibration claim and belongs here. "This setting is worth N points" is a
// policy claim and belongs on the replay. The convention this project already
// follows is that the replay's held-squad metric judges scoring constants and
// its transfer metric judges transfer constants; this instrument sits *before*
// both, and a candidate that wins here is a candidate worth spending replay time
// on, not a candidate already proved.
//
// Note also that a minimum detectable effect belongs to a *comparison* and not
// to a harness: on the same grid, a mechanism-certain change like the
// vice-captain fallback resolves about 13 points a season on the transfer metric
// while a season-varying scoring constant needs about 147. So the replay's
// recorded thresholds are what a *scoring constant* costs to resolve there, and
// this instrument exists to give such constants a second, much larger sample —
// not to replace the number that decides whether they earn points.
//
// # The three axes
//
// **Error** — mean absolute error and root-mean-square error. Mean absolute
// error is the average of |predicted − actual|; root-mean-square error is the
// square root of the average of (predicted − actual)², which punishes a single
// large miss much harder. Both are in the units of the thing predicted (FPL
// points, minutes, expected goals plus assists) and **lower is better** for
// both.
//
// **Calibration** — predicted against realised, grouped by what was predicted.
// A model is calibrated when the players it rates at 5.0 average 5.0. This is
// how the mid-season over-confidence was found: predicted rose through a season
// while realised stayed flat.
//
// **Ordering** — Spearman's rank correlation, which is the ordinary correlation
// computed on ranks rather than on values: +1 means the predicted ordering is
// exactly right, 0 means it carries no ordering information, −1 means it is
// exactly reversed. This axis exists because the optimiser consumes an
// *ordering* and never a level. It is why the bonus term is kept despite being
// badly calibrated: it is informative about who is better even where it is wrong
// about how much.
//
// # The conditional breakdown is the point
//
// Aggregate error over every player-gameweek is dominated by thousands of easy
// near-zero predictions, and says nothing about the tail the transfer search
// actually hunts. So error is split by **what the player actually scored**,
// using OpenFPL's categories (arXiv 2508.09992) so these numbers sit directly
// beside published ones:
//
//	Zeros    recorded no minutes
//	Blanks   played, two points or fewer
//	Tickers  three or four points
//	Haulers  five or more
//
// Zeros is defined by *no minutes*, not by no points: a player who came on for
// ten minutes and returned one point is a Blank, because he could not have been
// substituted for and the model's minutes estimate was not simply wrong about
// whether he featured.
//
// A Haulers-only figure is a direct measurement of the tail error the entire
// argmax problem is about, and nothing else in this project sees it.
//
// **The categories condition on the outcome, which is deliberate and is not a
// predictive claim.** Nobody knows in advance which category a player will land
// in; the split is a way of asking where the error lives, in the same spirit as
// grouping defenders by realised defensive-contribution rate. It is a diagnostic
// decomposition, not a forecast.
//
// # The population, and why the club-fixture restriction is load-bearing
//
// A player-gameweek is in the sample when **his club had a fixture in that
// gameweek**. Without that restriction a missing per-gameweek row is ambiguous
// between "he was dropped" and "his club did not play", so the Zeros category
// fills up with blank gameweeks instead of with the dropped and injured players
// it is for — and those are the population whose error is worth knowing, because
// they are what team news is about. With it, a missing row is an unambiguous
// zero: the club played and he recorded nothing.
//
// Two populations are reported, both filtered model-independently so no
// predictor is favoured by the filter:
//
//   - **players who played sixty or more minutes in one of the previous five
//     gameweeks their club played** — the headline. Roughly the set a manager
//     would consider, so the conditional error figures mean something. Sixty
//     minutes because that is the threshold FPL itself pays appearance points and
//     the clean sheet at. It is deliberately *not* "started recently": see
//     sixtyMinutes, where an archive column that is empty for a season and a half
//     silently deleted a season from the sample.
//   - **every registered player whose club played** — the whole game. Its
//     aggregate is dominated by hundreds of reserves nobody would pick, so read
//     it as a sanity check on the filter rather than as a headline: if the two
//     populations disagree about which predictor wins, the filter is doing more
//     work than it should.
//
// A coverage table is printed before anything else, and the run **fails** if a
// season contributed no observations or lost a third of its gameweeks, because a
// pooled figure that is quietly three seasons rather than four is the failure this
// project keeps recording and no error table can show it.
//
// # The baseline, and the one that was deliberately left out
//
// **Mean of the last five gameweeks** is OpenFPL's baseline, so it is comparable
// in shape, and it is clean — it uses only gameweeks that had finished. A flat
// season-to-date average sits beside it, because that is what FPL's own
// bootstrap publishes and therefore what the recency work was measured against.
//
// **FPL's own published expected points is deliberately not used.** The archive
// carries it as `xP`, scraped from `ep_this` *after* each gameweek has ended,
// and the archive's data dictionary warns it may therefore reflect post-match
// information rather than the pre-match figure managers saw. It is not a free
// external reference: it is a contaminated one, and wiring it in would also cost
// a cache-version bump plus a field check in `parsedByThisVersion`, since this
// project has already been burnt by a version bump that a stale file left behind
// by an experiment defeated silently.
//
// # The caveat this instrument must carry in its own output
//
// A better predictor can make a worse policy. Recency-weighted rates improve
// out-of-sample error by about 2% and cost about 49 points a season in the
// replay, because a transfer policy is an argmax living in the *tail* of the
// estimate distribution: buying accuracy on the average player is paid for with
// noise at the top, which is exactly where the search looks.
//
// So every candidate comparison here reports which of two things it is:
//
//   - **bias reduction** — the systematic component of the error shrinks and
//     the spread of the error does not grow. Safe for an argmax, because
//     removing a systematic error cannot reorder candidates by chance.
//   - **a bias-for-variance trade** — the systematic component shrinks and the
//     spread grows. Dangerous, and the recorded reason recency on rates lost
//     points while recency on minutes gained them.
//
// And beside that classification, the quantity the argmax actually consumes:
// the **signed error on the highest-predicted players in each gameweek**. That
// is literally the population a transfer search picks from, and it turns the
// winner's curse from an inference into a number.
//
// External corroboration for the caution is in the published table below: on
// Haulers the leading commercial model beats a five-game moving average by 8%
// and the best open model beats the commercial one by 0.6%. There is very little
// predictable signal in the tail to buy.
//
// # What was verified before this was trusted
//
// Two controls, and they check opposite things.
//
// The **vice-captain fallback** is a positive control for the replay and a
// *negative* control here: it changes how a played-out gameweek is scored and
// nothing about what the model predicts, so this instrument must be
// byte-identical with it on and off. If it moved, the instrument would be
// reading the replay's scoring rather than the model's predictions.
//
// The **minutes half-life** is the directional control. It was set out of sample
// on 8,374 predictions, where sharpening recency on minutes cut minutes error by
// about 9% — so switching it off must make the minutes error *worse*. If it does
// not, the instrument is wrong and should be said to be wrong rather than
// shipped. Both controls fail the test rather than printing a warning, because a
// control that misbehaves makes every other figure in the run unsafe to read.

import (
	"fmt"
	"math"
	"os"
	"sort"
	"testing"

	"armband/internal/analysis"
	"armband/internal/config"
	"armband/internal/fpl"
)

// predictFirstGW is the first gameweek predicted. Five of a club's gameweeks
// have to have finished before "mean of the last five" is defined, so the naive
// baseline and the model are compared on the same footing from GW6 rather than
// the baseline being handicapped for the opening month.
const predictFirstGW = 6

// tailSize is how many of the highest-predicted players a gameweek's tail figure
// covers. Fifteen is a squad, so the top twenty is roughly the set a transfer
// search chooses between once position and price constraints bite — the
// population an argmax lives in.
const tailSize = 20

// Category labels, spelled out because a bare "Tickers" is jargon. Zeros is *no
// minutes*, which is OpenFPL's definition and is not the same as no points.
const (
	catZeros   = "Zeros: recorded no minutes"
	catBlanks  = "Blanks: played, 2 points or fewer"
	catTickers = "Tickers: 3 or 4 points"
	catHaulers = "Haulers: 5 or more points"
	catAll     = "all categories"
)

// categoryOrder is the print order, lowest return first, so a table reads from
// the easy predictions to the hard ones.
var categoryOrder = []string{catZeros, catBlanks, catTickers, catHaulers, catAll}

func returnCategory(minutes, points int) string {
	switch {
	case minutes == 0:
		return catZeros
	case points <= 2:
		return catBlanks
	case points <= 4:
		return catTickers
	default:
		return catHaulers
	}
}

// The two populations. Both filters are model-independent on purpose: filtering
// on the model's own expected minutes would hand the model a population it had
// already selected and quietly handicap every baseline.
const (
	popRelevant = "played 60+ minutes in one of the previous 5 club gameweeks"
	popAll      = "every registered player whose club played"
)

// sixtyMinutes is the squad-relevance bar, and it is measured in minutes rather
// than in starts on purpose.
//
// The obvious filter is "started at least one of the previous five gameweeks",
// and it silently deletes a season and a half. **The archive's per-gameweek
// `starts` column is zero for the whole of 2021-22 and for 2022-23 up to GW15**,
// only populating from GW16 — verified directly against `gws/merged_gw.csv`
// rather than assumed. A starts-based filter therefore admitted nobody in
// 2022-23 before GW20, dropped ten of that season's gameweeks from the headline
// population, and reported a perfectly plausible table for the other three
// seasons. That is this project's signature failure: a measurement that quietly
// measures a different population while printing a believable number.
//
// Minutes are populated in every season, and sixty is the threshold FPL itself
// pays appearance points and the clean sheet at, so it is the natural bar. It is
// also the direction this project has already measured: reliability was changed
// from a mix of minutes and start share to minutes only, worth about 180 points
// over four seasons.
const sixtyMinutes = 60

var populationOrder = []string{popRelevant, popAll}

// The predictors, in print order. The model is first so every table reads
// "ours, then the baselines".
var predictorNames = []string{
	"model",
	"naive: mean of last 5 gameweeks",
	"naive: mean of season to date",
}

// targets are the three things predicted, with the unit spelled out because
// "error 18.98" is unreadable without one.
type predTarget struct {
	name, unit string
}

func predTargets() []predTarget {
	return []predTarget{
		{"points", "FPL points per gameweek"},
		{"minutes", "minutes per gameweek"},
		{"expected goals + assists", "xG+xA per gameweek"},
	}
}

// errAcc accumulates everything a squared-error report needs from a stream of
// observations, so nothing has to hold ninety thousand rows in memory.
type errAcc struct {
	n               int
	sumAbs, sumSq   float64
	sumPred, sumAct float64
}

func (a *errAcc) add(pred, act float64) {
	d := pred - act
	a.n++
	a.sumAbs += math.Abs(d)
	a.sumSq += d * d
	a.sumPred += pred
	a.sumAct += act
}

func (a *errAcc) mae() float64 {
	if a.n == 0 {
		return math.NaN()
	}
	return a.sumAbs / float64(a.n)
}

func (a *errAcc) rmse() float64 {
	if a.n == 0 {
		return math.NaN()
	}
	return math.Sqrt(a.sumSq / float64(a.n))
}

// bias is mean predicted minus mean actual: the systematic half of the error.
// Positive means the predictor over-predicts.
func (a *errAcc) bias() float64 {
	if a.n == 0 {
		return math.NaN()
	}
	return (a.sumPred - a.sumAct) / float64(a.n)
}

// errorSD is the spread of the error around its own mean — the part no
// systematic correction can remove.
//
// Root-mean-square error decomposes exactly as rmse² = bias² + errorSD², which
// is the arithmetic behind the bias-reduction-versus-variance-trade
// classification: a candidate can lower rmse by shrinking either term, and only
// the first is safe for an argmax.
func (a *errAcc) errorSD() float64 {
	if a.n == 0 {
		return math.NaN()
	}
	v := a.sumSq/float64(a.n) - a.bias()*a.bias()
	if v < 0 {
		v = 0 // floating-point slack only
	}
	return math.Sqrt(v)
}

// predVariant is one setting of the model, evaluated over the whole grid.
type predVariant struct {
	label string
	why   string // one sentence: what this arm is for
	// weights returns the config this arm scores with.
	weights func(config.Config) analysis.Weights
	// before runs immediately before the arm and returns a restore function. It
	// exists for the vice-captain control, which is a package variable rather
	// than a weight.
	before func() func()
}

const (
	armShipped     = "shipped"
	armMinutesFlat = "CONTROL, directional: minutes recency off"
	armViceOff     = "CONTROL, invariance: vice-captain fallback off"
	armRateRecency = "CANDIDATE: rate recency, half-life 8"
	armTwoAppear   = "CANDIDATE: two estimators of P(appears), as before the unification"
	armFitWindowed = "CANDIDATE: appearance constants refit on the windowed population"
	armFitExpected = "CANDIDATE: appearance constants refit against ExpectedMinutes"
	armFitSixty    = "CANDIDATE: the sixty-minute curve alone, refit against ExpectedMinutes"
)

// The two refits of the appearance curves, in SetAppearanceFit's argument order:
// sixty_slope, sixty_midpoint, cond_intercept, cond_slope.
//
// Both come from TestDiagStartShare sections 5 and 6, fitted on all four seasons.
// Regenerate with:
//
//	DIAG=1 go test ./internal/backtest -run TestDiagStartShare -v
//
// They are pinned here rather than fitted at benchmark time on purpose. Fitting
// inside the arm would make the arm's result depend on a search this test does not
// report, so a movement could come from the fit or from the model and nobody could
// tell which — and it would put a two-minute fit inside a six-second instrument.
var (
	// The refit worth +2.0% on P(appears) and +3.9% on P(60+) out of sample, on the
	// windowed proxy it was fitted against. Note how far it is from shipped: the
	// conditional-mean intercept falls from 28.15 to 9.91 and its slope rises from
	// 0.779 to 1.281, which is a materially flatter appearance curve.
	fitWindowed = [4]float64{0.0364, 50.83, 9.91, 1.2813}
	// The same forms fitted against ExpectedMinutes — blended, recency-weighted and
	// rested — which is what the curves are actually evaluated at. Three of the four
	// constants barely move from shipped; only the sixty-minute slope does.
	fitExpected = [4]float64{0.0486, 48.75, 28.66, 0.7708}
	// The sixty-minute curve from that same fit, with the two P(appears) constants
	// left exactly as they ship. It exists because fitExpected moves all four at
	// once while the fit itself says only this curve wants moving — the identity's
	// constants come back within 2% of shipped — so an arm that moves everything
	// cannot attribute its result to the one parameter that changed materially.
	fitSixtyOnly = [4]float64{0.0486, 48.75,
		shippedCondIntercept, shippedCondSlope}
)

// The shipped P(appears) constants, so fitSixtyOnly cannot drift from them. Read
// from the package at init rather than retyped, which is the same rule the rest of
// this pass applies to curves.
var shippedCondIntercept, shippedCondSlope = func() (float64, float64) {
	_, _, intercept, slope := analysis.ShippedAppearanceFit()
	return intercept, slope
}()

// predictionVariants declares the arms. Index 0 is the baseline everything is
// paired against, and the controls say in their own labels that they are
// controls, so a reader does not have to infer which arms are validation and
// which are candidates.
func predictionVariants() []predVariant {
	return []predVariant{
		{
			label:   armShipped,
			why:     "the config as it ships; the baseline every other arm is paired against",
			weights: func(c config.Config) analysis.Weights { return c.Weights },
		},
		{
			label: armMinutesFlat,
			why: "minutes_half_life 0 instead of the shipped value. Recency on minutes was " +
				"set out of sample on 8,374 predictions, where it cut minutes error about " +
				"9%, so the minutes error here MUST get worse. If it does not, this " +
				"instrument is broken.",
			weights: func(c config.Config) analysis.Weights {
				w := c.Weights
				w.MinutesHalfLife = 0
				return w
			},
		},
		{
			label: armViceOff,
			why: "changes how a played-out gameweek is SCORED and nothing about what the " +
				"model predicts, so every figure MUST be identical to shipped. A movement " +
				"means the instrument is reading the replay rather than the model.",
			weights: func(c config.Config) analysis.Weights { return c.Weights },
			before: func() func() {
				old := viceCaptainFallback
				viceCaptainFallback = false
				return func() { viceCaptainFallback = old }
			},
		},
		{
			label: armTwoAppear,
			why: "restores the SECOND estimator of P(appears) that blankRate used to " +
				"carry — 1 - 0.624 x (1 - StartShare) — beside playsAtAll. It reaches " +
				"the prediction through defconPerGameweek's exposure only, and defensive " +
				"contribution scores in 2025-26 alone, so a small movement concentrated " +
				"in the defender-heavy low-return categories is what to expect. The " +
				"bench-slot channel does not appear here at all: this instrument predicts " +
				"a player's points and never picks a squad.",
			weights: func(c config.Config) analysis.Weights { return c.Weights },
			before: func() func() {
				analysis.SetUnifiedAppearance(false)
				return func() { analysis.SetUnifiedAppearance(true) }
			},
		},
		{
			label: armFitWindowed,
			why: "the queued refit. TestDiagStartShare records that refitting these two " +
				"curves with NO new input is worth +2.0% on P(appears) and +3.9% on " +
				"P(60+) out of sample, four to twenty times what the start-share input " +
				"it rejected bought — but every one of those fits is against a windowed " +
				"mean-minutes proxy, while the curves are evaluated at ExpectedMinutes. " +
				"A curve fitted against a noisier predictor than it is scored on is " +
				"over-hedged, so this arm is expected to LOSE here. If it wins, the " +
				"over-hedging argument in appearance.go is wrong.",
			weights: func(c config.Config) analysis.Weights { return c.Weights },
			before: func() func() {
				// Restore what was LIVE, not what ships. The two differ whenever
				// the whole run is invoked with FPL_APPEARANCE_FIT already set:
				// restoring to shipped would leave every later arm scoring against
				// a baseline the earlier arms did not use, and the deltas would
				// silently carry two changes at once with nothing erroring.
				a, b, c, d := analysis.AppearanceFit()
				analysis.SetAppearanceFit(fitWindowed[0], fitWindowed[1],
					fitWindowed[2], fitWindowed[3])
				return func() { analysis.SetAppearanceFit(a, b, c, d) }
			},
		},
		{
			label: armFitExpected,
			why: "the same refit done against the predictor the model feeds in, which is " +
				"the version the over-hedging objection does not apply to. On the curves' " +
				"own rms it is a wash on P(appears) and about 1.3% better on P(60+), so " +
				"the expected movement here is small and concentrated in the part-timers " +
				"the sixty-minute curve separates. Reaching four scoring consumers at " +
				"once — appearance points, the clean sheet's sixty-minute scaling, the " +
				"bench slot weights and the defcon exposure — it is not confined to one " +
				"term the way most arms here are.",
			weights: func(c config.Config) analysis.Weights { return c.Weights },
			before: func() func() {
				// Restores what was live rather than what ships. See the arm above.
				a, b, c, d := analysis.AppearanceFit()
				analysis.SetAppearanceFit(fitExpected[0], fitExpected[1],
					fitExpected[2], fitExpected[3])
				return func() { analysis.SetAppearanceFit(a, b, c, d) }
			},
		},
		{
			label: armFitSixty,
			why: "isolates the one parameter the honest refit actually wants moved. Fitted " +
				"against ExpectedMinutes, the sixty-minute slope wants 0.065 -> 0.049 — a " +
				"25% change in the steepest constant in the model, worth about 1.3% on " +
				"P(60+) — while the two P(appears) constants come back within 2% of what " +
				"ships. The arm above moves all four, so its bias cost cannot be " +
				"attributed to this one. Here the two identity constants are pinned at " +
				"shipped, so whatever moves is the sixty-minute curve.",
			weights: func(c config.Config) analysis.Weights { return c.Weights },
			before: func() func() {
				a, b, c, d := analysis.AppearanceFit()
				analysis.SetAppearanceFit(fitSixtyOnly[0], fitSixtyOnly[1],
					fitSixtyOnly[2], fitSixtyOnly[3])
				return func() { analysis.SetAppearanceFit(a, b, c, d) }
			},
		},
		{
			label: armRateRecency,
			why: "the recorded trap. Out of sample, gentle recency on rates predicts points " +
				"about 2% better; in the replay it cost about 49 points a season. Included " +
				"so the instrument can be seen to reproduce both halves of that at once.",
			weights: func(c config.Config) analysis.Weights {
				w := c.Weights
				w.RateHalfLife = 8
				return w
			},
		},
	}
}

// predRun is everything one arm measured.
type predRun struct {
	label string
	// err is keyed by population | target | predictor | category.
	err map[string]*errAcc
	// calib is keyed by population | predictor | predicted-value band.
	calib map[string]*errAcc
	// rank holds the per-gameweek Spearman correlations, keyed population|predictor.
	rank map[string][]float64
	// tail holds the per-gameweek mean signed error over the highest-predicted
	// players, keyed population|predictor.
	tail map[string][]float64
	// coverage counts, per season, the gameweeks that contributed at least one
	// headline-population observation and the observations themselves. It exists
	// because a population filter that silently empties a season is this
	// instrument's most dangerous failure and the printed tables cannot show it.
	coverageGWs, coverageObs map[string]int
	// seasonOrder keeps the seasons in the order the grid declared them, since
	// map iteration would otherwise reorder a printed table between runs.
	seasonOrder []string
}

func newPredRun(label string) *predRun {
	return &predRun{
		label:       label,
		err:         map[string]*errAcc{},
		calib:       map[string]*errAcc{},
		rank:        map[string][]float64{},
		tail:        map[string][]float64{},
		coverageGWs: map[string]int{},
		coverageObs: map[string]int{},
	}
}

func (r *predRun) acc(m map[string]*errAcc, k string) *errAcc {
	a, ok := m[k]
	if !ok {
		a = &errAcc{}
		m[k] = a
	}
	return a
}

// predKey joins the parts of an accumulator key with a separator that cannot
// occur inside a label.
func predKey(parts ...string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += " | "
		}
		out += p
	}
	return out
}

// predictedBand groups a prediction by how large it is, so calibration is
// readable at the level decisions are made at. The top band is open-ended
// because that is where the argmax lives and splitting it further would leave
// too few observations to read.
func predictedBand(v float64) string {
	switch {
	case v < 1:
		return "predicted under 1.0"
	case v < 2:
		return "predicted 1.0 to 2.0"
	case v < 3:
		return "predicted 2.0 to 3.0"
	case v < 4:
		return "predicted 3.0 to 4.0"
	case v < 5:
		return "predicted 4.0 to 5.0"
	case v < 6:
		return "predicted 5.0 to 6.0"
	default:
		return "predicted 6.0 and above"
	}
}

var bandOrder = []string{
	"predicted under 1.0", "predicted 1.0 to 2.0", "predicted 2.0 to 3.0",
	"predicted 3.0 to 4.0", "predicted 4.0 to 5.0", "predicted 5.0 to 6.0",
	"predicted 6.0 and above",
}

// playerGW is one observation: what every predictor said about a player in a
// gameweek, and what he actually did.
//
// Predictions are fixed-length slices indexed by the predictor's position in
// predictorNames rather than maps, because there are of the order of ninety
// thousand of these per arm and a map per observation costs memory and speed for
// no readability gain.
type playerGW struct {
	id       int
	relevant bool

	actPoints  float64
	actMinutes float64
	actXGI     float64
	category   string

	points  []float64
	minutes []float64
	xgi     []float64
}

func (r playerGW) pred(target string, i int) float64 {
	switch target {
	case "points":
		return r.points[i]
	case "minutes":
		return r.minutes[i]
	default:
		return r.xgi[i]
	}
}

func (r playerGW) act(target string) float64 {
	switch target {
	case "points":
		return r.actPoints
	case "minutes":
		return r.actMinutes
	default:
		return r.actXGI
	}
}

func TestDiagPredictionBenchmark(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)
	sink := openModelSinkFor(t.Logf)
	defer sink.close()
	cells := openPredictionSinkFor(t.Logf)
	defer cells.close()

	pairs := loadPairs(t, cfg)
	grid := fmt.Sprintf("%d seasons, gameweeks %d-38, one gameweek ahead",
		len(pairs), predictFirstGW)

	fmt.Printf("\n%s", predictionIntro(len(pairs)))

	runs := make([]*predRun, 0, len(predictionVariants()))
	for _, v := range predictionVariants() {
		var restore func()
		if v.before != nil {
			restore = v.before()
		}
		run := runPredictionArm(t, cfg, v, pairs, cells)
		if restore != nil {
			restore()
		}
		runs = append(runs, run)
	}

	base := runs[0]
	reportPredictionCoverage(t, base, sink, grid)
	reportPredictionErrors(base, sink, grid)
	reportPredictionCalibration(base, sink, grid)
	reportPredictionOrdering(base, sink, grid)
	reportPredictionCandidates(runs, sink, grid)
	reportPredictionControls(t, runs)
}

func predictionIntro(seasons int) string {
	return fmt.Sprintf(`OUT-OF-SAMPLE PREDICTION BENCHMARK
%d seasons, gameweeks %d to 38, predicting ONE gameweek ahead from a model built
through the gameweek before it. A player-gameweek is in the sample when his club
had a fixture that gameweek, so a missing row is an unambiguous zero rather than
a blank gameweek in disguise.

This measures whether the scoring model is right about football. It does NOT
measure whether a change to it is worth points — that is the replay's job, and
this project has a measured case where 2%% lower prediction error cost 49 points
a season. Read the candidates section's bias-versus-variance verdict, and its
tail figure, before acting on any error number here.

`, seasons, predictFirstGW)
}

// clubGameweeks is how many fixtures each club has in each gameweek: 1 normally,
// 2 in a double, and absent when it blanks.
//
// This is the population filter and it has to come from the fixture list rather
// than from the presence of a per-gameweek row, which is the whole point of the
// restriction: a row's absence is then a genuine zero.
func clubGameweeks(s *Season) map[int]map[int]int {
	out := map[int]map[int]int{}
	bump := func(team, gw int) {
		if out[team] == nil {
			out[team] = map[int]int{}
		}
		out[team][gw]++
	}
	for _, f := range s.Fixtures {
		if f.Event == nil {
			continue
		}
		bump(f.TeamH, *f.Event)
		bump(f.TeamA, *f.Event)
	}
	return out
}

// runPredictionArm scores one setting of the model over the whole grid.
func runPredictionArm(t *testing.T, cfg config.Config, v predVariant,
	pairs []seasonPair, cells *predictionSink) *predRun {
	t.Helper()
	run := newPredRun(v.label)
	w := v.weights(cfg)
	// A one-gameweek-ahead prediction wants the one-gameweek view of the model.
	// Score averages fixture difficulty over Weights.Horizon gameweeks, and the
	// fixture-load term — matches per gameweek, which is what makes a double
	// gameweek worth two — is deliberately confined to the horizon-1 view. So
	// horizon 1 is not a tweak here, it is the object being evaluated. This is
	// the same configuration analysis.Engine.WeekEngine produces; it is built
	// directly rather than through that method so each cutoff pays for one
	// engine instead of two.
	w.Horizon = 1

	for _, pair := range pairs {
		prior, cur := pair.Prior, pair.Cur
		idx := newPriorIndex(prior)
		played := clubGameweeks(cur)
		run.seasonOrder = append(run.seasonOrder, cur.Name)
		for gw := predictFirstGW; gw <= 38; gw++ {
			cut := gw - 1
			boot, fx := PointInTime(cur, prior, cut)
			e := analysis.NewEngineFull(boot, fx, w, analysis.Congestion{}, analysis.RoleRisk{})
			e.Priors = idx
			e.Recent = newRecentIndexWith(cur, cut, w.MinutesHalfLife, w.RateHalfLife)

			rows := collectPredictions(cur, e, boot, played, gw)
			relevant := 0
			for _, r := range rows {
				if r.relevant {
					relevant++
				}
			}
			if relevant > 0 {
				run.coverageGWs[cur.Name]++
				run.coverageObs[cur.Name] += relevant
			}
			foldPredictions(run, rows)
			cells.emitGameweek(v.label, cur.Name, pair.PriorName, gw, rows)
		}
	}
	return run
}

// collectPredictions builds one observation per player whose club played.
func collectPredictions(cur *Season, e *analysis.Engine, boot *fpl.Bootstrap,
	played map[int]map[int]int, gw int) []playerGW {
	out := make([]playerGW, 0, len(boot.Elements))
	for i := range boot.Elements {
		el := &boot.Elements[i]
		p := cur.Players[el.ID]
		if p == nil {
			continue
		}
		if played[el.Team][gw] == 0 {
			continue // his club blanked; there was nothing to predict
		}
		// A missing row now means he recorded nothing in a gameweek his club
		// played, which is a genuine zero and belongs in the Zeros category.
		g := p.GWs[gw]

		m := e.Metrics(el)
		// Expected minutes and the raw rates are per *match*; the fixture load
		// converts them to per *gameweek*, which is the unit the realised figure
		// is in. Getting this the wrong way round is the recorded units bug that
		// made a 22-minute cameo read as 8.18 bonus a gameweek, arriving from the
		// other direction.
		load := m.FixtureLoad
		if load <= 0 {
			load = 1
		}
		modelMinutes := m.ExpectedMinutes * load
		modelXGI := m.XGI90 * (m.ExpectedMinutes / 90) * load

		out = append(out, playerGW{
			id:         el.ID,
			relevant:   playedRecently(p, played[el.Team], gw),
			actPoints:  float64(g.Points),
			actMinutes: float64(g.Minutes),
			actXGI:     g.XG + g.XA,
			category:   returnCategory(g.Minutes, g.Points),
			points: []float64{
				m.Score,
				meanRecentClubGWs(p, played[el.Team], gw, 5, gwPoints),
				meanSeasonToDate(p, played[el.Team], gw, gwPoints),
			},
			minutes: []float64{
				modelMinutes,
				meanRecentClubGWs(p, played[el.Team], gw, 5, gwMinutes),
				meanSeasonToDate(p, played[el.Team], gw, gwMinutes),
			},
			xgi: []float64{
				modelXGI,
				meanRecentClubGWs(p, played[el.Team], gw, 5, gwXGI),
				meanSeasonToDate(p, played[el.Team], gw, gwXGI),
			},
		})
	}
	// Fixed order so a tie in the tail selection resolves the same way every
	// run. Selecting from a map is what made the clean-sheet diagnostic disagree
	// with itself by 0.7% between identical runs.
	sort.Slice(out, func(i, j int) bool { return out[i].id < out[j].id })
	return out
}

func gwPoints(g GW) float64  { return float64(g.Points) }
func gwMinutes(g GW) float64 { return float64(g.Minutes) }
func gwXGI(g GW) float64     { return g.XG + g.XA }

// playedRecently is the squad-relevance filter: did he play at least sixty
// minutes in one of the previous five gameweeks his club played?
//
// Minutes rather than starts, for the reason recorded at sixtyMinutes: the
// archive's per-gameweek starts column is empty for a season and a half.
//
// Counted in *club* gameweeks rather than calendar gameweeks so a club with a
// blank does not have its players quietly aged out of the population.
func playedRecently(p *Player, clubGWs map[int]int, gw int) bool {
	found := 0
	for w := gw - 1; w >= 1 && found < 5; w-- {
		if clubGWs[w] == 0 {
			continue
		}
		found++
		if g, ok := p.GWs[w]; ok && g.Minutes >= sixtyMinutes {
			return true
		}
	}
	return false
}

// meanRecentClubGWs is OpenFPL's baseline: the mean over the last n gameweeks
// his club played.
//
// Gameweeks the club did not play are skipped, and gameweeks it played in which
// he has no row count as zero — the same convention as the population filter, so
// the baseline and the target are measured on one definition of a gameweek.
func meanRecentClubGWs(p *Player, clubGWs map[int]int, gw, n int, f func(GW) float64) float64 {
	var sum float64
	found := 0
	for w := gw - 1; w >= 1 && found < n; w-- {
		if clubGWs[w] == 0 {
			continue
		}
		sum += f(p.GWs[w])
		found++
	}
	if found == 0 {
		return 0
	}
	return sum / float64(found)
}

// meanSeasonToDate is the same quantity over every gameweek his club has played
// — the flat season average, which is what FPL's bootstrap publishes and
// therefore the predictor the recency work was measured against.
func meanSeasonToDate(p *Player, clubGWs map[int]int, gw int, f func(GW) float64) float64 {
	var sum float64
	n := 0
	for w := 1; w < gw; w++ {
		if clubGWs[w] == 0 {
			continue
		}
		sum += f(p.GWs[w])
		n++
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

// foldPredictions folds one gameweek's observations into the arm's accumulators.
func foldPredictions(run *predRun, rows []playerGW) {
	for _, pop := range populationOrder {
		sel := rows
		if pop == popRelevant {
			sel = make([]playerGW, 0, len(rows))
			for _, r := range rows {
				if r.relevant {
					sel = append(sel, r)
				}
			}
		}
		if len(sel) == 0 {
			continue
		}
		for _, tg := range predTargets() {
			for pi, name := range predictorNames {
				all := run.acc(run.err, predKey(pop, tg.name, name, catAll))
				for _, r := range sel {
					pred, act := r.pred(tg.name, pi), r.act(tg.name)
					all.add(pred, act)
					run.acc(run.err, predKey(pop, tg.name, name, r.category)).add(pred, act)
				}
			}
		}
		// Calibration, ordering and the tail are about points only: they ask
		// what the optimiser consumes, and the optimiser consumes Score.
		for pi, name := range predictorNames {
			preds := make([]float64, len(sel))
			acts := make([]float64, len(sel))
			for i, r := range sel {
				preds[i], acts[i] = r.points[pi], r.actPoints
				run.acc(run.calib, predKey(pop, name, predictedBand(preds[i]))).
					add(preds[i], acts[i])
			}
			k := predKey(pop, name)
			if rho, ok := spearman(preds, acts); ok {
				run.rank[k] = append(run.rank[k], rho)
			}
			if v, ok := tailSignedError(preds, acts, tailSize); ok {
				run.tail[k] = append(run.tail[k], v)
			}
		}
	}
}

// tailSignedError is the mean of (predicted − actual) over the n
// highest-predicted observations in a gameweek.
//
// This is the winner's curse as a measured quantity rather than an inference.
// The transfer search is an argmax, so it picks from exactly this set; a
// candidate that lowers aggregate error while pushing this figure further
// positive has bought accuracy on the average player and paid for it where the
// search looks. **Positive means the top of the predicted distribution is
// over-rated**, and closer to zero is better.
func tailSignedError(pred, act []float64, n int) (float64, bool) {
	if len(pred) == 0 || len(pred) != len(act) {
		return 0, false
	}
	idx := make([]int, len(pred))
	for i := range idx {
		idx[i] = i
	}
	// Descending by prediction, ties broken by position so the selection is
	// deterministic.
	sort.SliceStable(idx, func(a, b int) bool {
		if pred[idx[a]] != pred[idx[b]] {
			return pred[idx[a]] > pred[idx[b]]
		}
		return idx[a] < idx[b]
	})
	if n > len(idx) {
		n = len(idx)
	}
	var sum float64
	for _, i := range idx[:n] {
		sum += pred[i] - act[i]
	}
	return sum / float64(n), true
}

// reportPredictionCoverage prints how much of each season reached the headline
// population, and fails when a season contributed nothing.
//
// This is not bookkeeping. The first version of this benchmark filtered on
// "started at least one of the previous five gameweeks", and the archive's
// per-gameweek `starts` column is empty for the whole of 2021-22 and for 2022-23
// before GW16 — so that season contributed **no observations at all** for ten of
// its thirty-three predicted gameweeks, and every printed table looked entirely
// reasonable. A pooled figure that is quietly three seasons rather than four is
// exactly the orphaned measurement this project keeps recording, so the count is
// printed and checked rather than assumed.
func reportPredictionCoverage(t *testing.T, run *predRun, sink *modelSink, grid string) {
	t.Helper()
	fmt.Printf("## How much of each season reached the sample\n\n")
	fmt.Printf("The headline population is: %s.\n", popRelevant)
	fmt.Printf("A season contributing few gameweeks or few observations means the filter is\n")
	fmt.Printf("doing something unintended in that season, not that the season was quiet.\n\n")
	fmt.Printf("%-12s %14s %20s %16s\n",
		"season", "gameweeks", "observations", "per gameweek")
	expected := 38 - predictFirstGW + 1
	for _, s := range run.seasonOrder {
		gws, obs := run.coverageGWs[s], run.coverageObs[s]
		per := 0.0
		if gws > 0 {
			per = float64(obs) / float64(gws)
		}
		fmt.Printf("%-12s %8d of %-3d %20d %16.1f\n", s, gws, expected, obs, per)
		sink.emitAll("prediction_coverage", grid, s, obs,
			measure{"gameweeks contributing observations", float64(gws)},
			measure{"observations in the headline population", float64(obs)},
			measure{"observations per gameweek", per})
		if obs == 0 {
			t.Errorf("season %s contributed NO observations to the headline "+
				"population.\nEvery pooled figure in this run is therefore about "+
				"%d seasons and not %d, with nothing in the tables to say so. The "+
				"known cause is an archive column that is empty in some seasons — "+
				"the per-gameweek `starts` field is absent for all of 2021-22 and "+
				"for 2022-23 before GW16 — so check what the population filter "+
				"reads before trusting anything else here.",
				s, len(run.seasonOrder)-1, len(run.seasonOrder))
		}
	}
	// A season missing a few gameweeks is legitimate — 2022-23's GW7 was
	// postponed in its entirety — but losing a third of them is not.
	for _, s := range run.seasonOrder {
		if g := run.coverageGWs[s]; g > 0 && g < expected*2/3 {
			t.Errorf("season %s contributed only %d of %d predicted gameweeks. A "+
				"gameweek postponed outright is expected (2022-23 GW7) and a third "+
				"of a season is not; the population filter is reading something "+
				"that season does not carry.", s, g, expected)
		}
	}
	fmt.Printf("\n")
}

func reportPredictionErrors(run *predRun, sink *modelSink, grid string) {
	fmt.Printf("## Error by what the player actually scored\n\n")
	fmt.Printf("Mean absolute error and root-mean-square error, both in the target's own\n")
	fmt.Printf("units, and LOWER IS BETTER for both. `bias` is mean predicted minus mean\n")
	fmt.Printf("actual, so positive means over-prediction; `error sd` is the spread of the\n")
	fmt.Printf("error around that bias, and rmse squared equals bias squared plus error sd\n")
	fmt.Printf("squared exactly.\n\n")
	fmt.Printf("The categories are OpenFPL's and are defined by the REALISED outcome, so\n")
	fmt.Printf("they say where the error lives rather than forming a forecast. Haulers is\n")
	fmt.Printf("the tail the transfer search hunts.\n\n")

	for _, pop := range populationOrder {
		for _, tg := range predTargets() {
			fmt.Printf("### %s — %s\n", tg.name, tg.unit)
			fmt.Printf("population: %s\n\n", pop)
			fmt.Printf("%-52s %8s %9s %9s %9s %9s\n",
				"predictor / realised category", "n", "MAE", "RMSE", "bias", "error sd")
			for _, name := range predictorNames {
				for _, cat := range categoryOrder {
					a := run.err[predKey(pop, tg.name, name, cat)]
					if a == nil || a.n == 0 {
						continue
					}
					fmt.Printf("%-52s %8d %9.3f %9.3f %+9.3f %9.3f\n",
						name+" / "+shortCategory(cat), a.n,
						a.mae(), a.rmse(), a.bias(), a.errorSD())
					if pop == popRelevant {
						sink.emitAll("prediction_benchmark", grid,
							tg.name+" — "+name+" — "+shortCategory(cat), a.n,
							measure{"mean absolute error", a.mae()},
							measure{"root-mean-square error", a.rmse()},
							measure{"bias (predicted minus actual)", a.bias()},
							measure{"error sd", a.errorSD()})
					}
				}
			}
			fmt.Printf("\n")
		}
	}

	fmt.Printf("### Published reference points\n\n")
	fmt.Printf("Root-mean-square error in points, one gameweek ahead, from OpenFPL (arXiv\n")
	fmt.Printf("2508.09992). LOWER IS BETTER. INDICATIVE ONLY: seven gameweeks of one\n")
	fmt.Printf("season, and the population behind them is not published — an\n")
	fmt.Printf("outcome-conditioned error figure is very sensitive to who was eligible to\n")
	fmt.Printf("be predicted, so do not read the gap against our columns as a scoreboard.\n\n")
	fmt.Printf("%-24s %14s %12s %12s\n", "category", "naive last-5", "FPL Review", "OpenFPL")
	for _, r := range [][4]string{
		{"Zeros", "0.791", "0.689", "0.818"},
		{"Blanks", "1.400", "1.189", "1.291"},
		{"Tickers", "2.136", "1.594", "1.517"},
		{"Haulers", "5.613", "5.172", "5.142"},
	} {
		fmt.Printf("%-24s %14s %12s %12s\n", r[0], r[1], r[2], r[3])
	}
	fmt.Printf("\nWhat is comparable is the SHAPE. The tail is where the error is; the\n")
	fmt.Printf("frontier there is nearly flat — the leading commercial model beats a\n")
	fmt.Printf("five-game moving average on Haulers by 8%% and the best open model beats\n")
	fmt.Printf("the commercial one by 0.6%%; and the resolvable edge sits in the low-return\n")
	fmt.Printf("categories, which the paper attributes to expected minutes. That last point\n")
	fmt.Printf("agrees with this project's own largest information bound, which is the\n")
	fmt.Printf("minutes family — though none of it RESOLVES. Against a common baseline:\n")
	fmt.Printf("perfect lineups (who starts) is worth ~73 points a season held at CR2\n")
	fmt.Printf("t = 1.32, perfect minutes ~47 at t = 0.62, and perfect team news ~14.\n")
	fmt.Printf("Perfect price timing is +15 and does not resolve either. So the ordering,\n")
	fmt.Printf("not the significance, is the finding: the value is not in 'never buy a\n")
	fmt.Printf("player who has left' but in rotation, cameos and injuries that resolve —\n")
	fmt.Printf("and it is SELECTION that carries it rather than minute-level precision.\n\n")
}

// shortCategory is the label without its definition, for a narrow column. The
// definition is printed in the section header instead.
func shortCategory(c string) string {
	for i := 0; i < len(c); i++ {
		if c[i] == ':' {
			return c[:i]
		}
	}
	return c
}

// reportPredictionCalibration groups by PREDICTED value. That conditioning is the
// point: the benchmark table above conditions on the OUTCOME, and a spread measured
// on players who turned out to haul cannot be attached to a player the model merely
// says will.
//
// `error sd` — the spread of the error around its own bias, within the band — is
// printed and emitted because `errAcc` already carries the `sumSq` it needs for
// every band. It was being accumulated on every push and discarded at print time,
// which is what made spread-conditioned-on-the-prediction look unmeasured when it
// was only unreported.
//
// ⚠️ It is NOT a per-player interval and may not be quoted as one. The spread is
// POOLED over every player in the band, so it describes the band rather than the
// player; and the top band is open-ended, so its sd mixes a prediction of 6 with a
// prediction of 15. A band-level spread is a coarse INPUT to an interval, not an
// interval.
func reportPredictionCalibration(run *predRun, sink *modelSink, grid string) {
	fmt.Printf("## Calibration: do the players rated at 5.0 score 5.0?\n\n")
	fmt.Printf("Grouped by what was PREDICTED, so this reads at the level decisions are made\n")
	fmt.Printf("at. `ratio` is actual divided by predicted: 1.000 is perfect calibration and\n")
	fmt.Printf("BELOW 1.000 means the band is over-predicted. The top band is where the\n")
	fmt.Printf("transfer search picks, so its ratio matters more than the aggregate — a bias\n")
	fmt.Printf("shared by every player is invisible to an argmax, and this project has\n")
	fmt.Printf("measured that correcting one costs points.\n\n")
	fmt.Printf("`error sd` is the spread of the error around its own bias inside the band.\n")
	fmt.Printf("Unlike the error sd in the benchmark table above, which conditions on what\n")
	fmt.Printf("the player ACTUALLY scored, this one conditions on what was PREDICTED — the\n")
	fmt.Printf("only conditioning a per-player interval could be built from. It is not one\n")
	fmt.Printf("yet: the spread is pooled over the whole band, and the top band is\n")
	fmt.Printf("open-ended, so its figure mixes a prediction of 6 with a prediction of 15.\n\n")
	fmt.Printf("Each error sd is a point estimate and carries its own sampling error, which\n")
	fmt.Printf("is not reported here. Normal theory puts it near 2%% at every band size\n")
	fmt.Printf("shown, and that is optimistic in the top bands: a prediction above 6 is a\n")
	fmt.Printf("small set of premium players recurring across weeks, so the rows there are\n")
	fmt.Printf("further from independent draws than their count suggests.\n\n")
	fmt.Printf("Bands with fewer than 30 observations are omitted rather than printed as\n")
	fmt.Printf("noise.\n\n")

	pop := popRelevant
	fmt.Printf("population: %s\n\n", pop)
	for _, name := range predictorNames {
		fmt.Printf("%s\n", name)
		fmt.Printf("  %-26s %8s %10s %10s %8s %9s\n",
			"band", "n", "predicted", "actual", "ratio", "error sd")
		for _, band := range bandOrder {
			a := run.calib[predKey(pop, name, band)]
			if a == nil || a.n < 30 {
				continue
			}
			mp := a.sumPred / float64(a.n)
			ma := a.sumAct / float64(a.n)
			ratio := math.NaN()
			if mp != 0 {
				ratio = ma / mp
			}
			fmt.Printf("  %-26s %8d %10.3f %10.3f %8.3f %9.3f\n",
				band, a.n, mp, ma, ratio, a.errorSD())
			if name == "model" {
				sink.emitAll("prediction_calibration", grid, band, a.n,
					measure{"predicted", mp},
					measure{"actual", ma},
					measure{"ratio", ratio},
					measure{"error sd", a.errorSD()})
			}
		}
		fmt.Printf("\n")
	}
}

func reportPredictionOrdering(run *predRun, sink *modelSink, grid string) {
	fmt.Printf("## Ordering: does the model rank players correctly?\n\n")
	fmt.Printf("Spearman's rank correlation — the ordinary correlation computed on ranks\n")
	fmt.Printf("rather than values — between predicted and actual points, computed WITHIN\n")
	fmt.Printf("each gameweek and then averaged over gameweeks. +1 is a perfect ordering, 0\n")
	fmt.Printf("is no ordering information, and HIGHER IS BETTER. Within-gameweek because\n")
	fmt.Printf("pooling would let the differences between gameweeks do the work.\n\n")
	fmt.Printf("This axis exists because the optimiser consumes an ordering and never a\n")
	fmt.Printf("level. It is why the bonus term is kept despite being badly calibrated: a\n")
	fmt.Printf("uniform under-prediction costs an argmax nothing, and losing the ranking\n")
	fmt.Printf("signal costs it a lot.\n\n")
	fmt.Printf("Beside it, the signed error over the %d highest-predicted players in each\n", tailSize)
	fmt.Printf("gameweek — the set an argmax actually picks from. POSITIVE means the top of\n")
	fmt.Printf("the predicted distribution is over-rated, which is the winner's curse as a\n")
	fmt.Printf("measured number rather than an inference. Closer to zero is better.\n\n")

	for _, pop := range populationOrder {
		fmt.Printf("population: %s\n\n", pop)
		fmt.Printf("%-40s %8s %12s %16s\n",
			"predictor", "gws", "rank corr", "tail signed err")
		for _, name := range predictorNames {
			rs := run.rank[predKey(pop, name)]
			ts := run.tail[predKey(pop, name)]
			if len(rs) == 0 {
				continue
			}
			fmt.Printf("%-40s %8d %12.4f %+16.3f\n", name, len(rs), meanOf(rs), meanOf(ts))
			if pop == popRelevant {
				sink.emitAll("prediction_ordering", grid, name, len(rs),
					measure{"mean within-gameweek rank correlation", meanOf(rs)},
					measure{fmt.Sprintf("signed error over the top %d predicted", tailSize),
						meanOf(ts)})
			}
		}
		fmt.Printf("\n")
	}
}

// reportPredictionCandidates is the part that decides anything.
func reportPredictionCandidates(runs []*predRun, sink *modelSink, grid string) {
	fmt.Printf("## Candidates, and whether each is safe for an argmax\n\n")
	fmt.Printf("Every arm is paired against `%s` on the same population and the same\n", armShipped)
	fmt.Printf("observations, so the columns are differences and NEGATIVE MEANS BETTER for\n")
	fmt.Printf("the error columns.\n\n")
	fmt.Printf("The verdict column is the one to read. Root-mean-square error decomposes\n")
	fmt.Printf("exactly as rmse squared = bias squared + error-spread squared, so a\n")
	fmt.Printf("candidate can lower it by shrinking either term:\n\n")
	fmt.Printf("  BIAS REDUCTION        the systematic part shrinks and the spread does not\n")
	fmt.Printf("                        grow. Safe for an argmax: removing a systematic\n")
	fmt.Printf("                        error cannot reorder candidates by chance.\n")
	fmt.Printf("  VARIANCE TRADE        the systematic part shrinks and the spread grows.\n")
	fmt.Printf("                        Dangerous, and the recorded reason recency on rates\n")
	fmt.Printf("                        lost points while recency on minutes gained them.\n")
	fmt.Printf("  WORSE ON BOTH         no case at all.\n")
	fmt.Printf("  NO MEASURABLE CHANGE  every figure identical; for the invariance control\n")
	fmt.Printf("                        that is the check passing.\n\n")

	base := runs[0]
	pop := popRelevant
	whyOf := map[string]string{}
	for _, v := range predictionVariants() {
		whyOf[v.label] = v.why
	}

	for _, run := range runs[1:] {
		fmt.Printf("### %s\n", run.label)
		fmt.Printf("what this arm is for: %s\n\n", whyOf[run.label])
		fmt.Printf("%-26s %10s %10s %10s %10s  %s\n",
			"target", "d MAE", "d RMSE", "d |bias|", "d err sd", "verdict")
		for _, tg := range predTargets() {
			b := base.err[predKey(pop, tg.name, "model", catAll)]
			c := run.err[predKey(pop, tg.name, "model", catAll)]
			if b == nil || c == nil || b.n == 0 || c.n == 0 {
				continue
			}
			dMAE := c.mae() - b.mae()
			dRMSE := c.rmse() - b.rmse()
			dBias := math.Abs(c.bias()) - math.Abs(b.bias())
			dSD := c.errorSD() - b.errorSD()
			fmt.Printf("%-26s %+10.4f %+10.4f %+10.4f %+10.4f  %s\n",
				tg.name, dMAE, dRMSE, dBias, dSD, argmaxVerdict(dBias, dSD))
			sink.emitAll("prediction_candidates", grid, run.label+" — "+tg.name, c.n,
				measure{"change in mean absolute error", dMAE},
				measure{"change in root-mean-square error", dRMSE},
				measure{"change in absolute bias", dBias},
				measure{"change in error sd", dSD})
		}

		bt := meanOf(base.tail[predKey(pop, "model")])
		ct := meanOf(run.tail[predKey(pop, "model")])
		br := meanOf(base.rank[predKey(pop, "model")])
		cr := meanOf(run.rank[predKey(pop, "model")])
		fmt.Printf("\n  signed error over the top %d predicted: %+0.3f -> %+0.3f (change %+0.3f)\n",
			tailSize, bt, ct, ct-bt)
		fmt.Printf("  within-gameweek rank correlation:      %.4f -> %.4f (change %+0.4f)\n",
			br, cr, cr-br)
		fmt.Printf("  A candidate that lowers aggregate error while pushing the tail figure\n")
		fmt.Printf("  further from zero, or lowering the rank correlation, has the recorded\n")
		fmt.Printf("  better-predictor-worse-policy shape. Take it to the replay before\n")
		fmt.Printf("  believing the error columns.\n\n")
		sink.emitAll("prediction_candidates", grid, run.label+" — tail and ordering", 0,
			measure{"change in signed error over the top predicted", ct - bt},
			measure{"change in rank correlation", cr - br})

		fmt.Printf("  change in points root-mean-square error, by realised category:\n")
		for _, cat := range categoryOrder {
			bb := base.err[predKey(pop, "points", "model", cat)]
			cc := run.err[predKey(pop, "points", "model", cat)]
			if bb == nil || cc == nil || bb.n == 0 {
				continue
			}
			fmt.Printf("    %-38s n=%-8d %+0.4f\n",
				shortCategory(cat), bb.n, cc.rmse()-bb.rmse())
		}
		fmt.Printf("\n")
	}
}

func argmaxVerdict(dBias, dSD float64) string {
	const eps = 1e-9
	switch {
	case math.Abs(dBias) < eps && math.Abs(dSD) < eps:
		return "NO MEASURABLE CHANGE"
	case dBias <= eps && dSD <= eps:
		return "BIAS REDUCTION (safe for an argmax)"
	case dBias < -eps && dSD > eps:
		return "VARIANCE TRADE (dangerous)"
	case dBias > eps && dSD <= eps:
		return "bias worse, spread better — not a bias reduction"
	default:
		return "WORSE ON BOTH"
	}
}

// reportPredictionControls states the verdict on the two controls in words, and
// fails the test when a control does not behave.
//
// It fails rather than warning because a control that misbehaves makes every
// other figure in the run unsafe to read, and this project has already shipped a
// measurement that quietly measured nothing while looking like a clean null.
func reportPredictionControls(t *testing.T, runs []*predRun) {
	t.Helper()
	fmt.Printf("## Are the controls behaving?\n\n")
	base := runs[0]

	byLabel := map[string]*predRun{}
	for _, r := range runs {
		byLabel[r.label] = r
	}

	if inv := byLabel[armViceOff]; inv != nil {
		worst, where := 0.0, ""
		for k, a := range base.err {
			b := inv.err[k]
			if b == nil {
				continue
			}
			if d := math.Abs(a.sumSq - b.sumSq); d > worst {
				worst, where = d, k
			}
		}
		if worst == 0 {
			fmt.Printf("INVARIANCE CONTROL PASSES. Toggling the vice-captain fallback moves no\n")
			fmt.Printf("figure at all, across all %d accumulated cells. That is the correct\n", len(base.err))
			fmt.Printf("result: the fallback changes how a played-out gameweek is scored and\n")
			fmt.Printf("nothing about what the model predicts, so this instrument must be blind\n")
			fmt.Printf("to it. It is a POSITIVE control for the replay and a NEGATIVE control\n")
			fmt.Printf("here, and confusing the two would mean reading a scoring fix as a\n")
			fmt.Printf("modelling one.\n\n")
		} else {
			t.Errorf("invariance control FAILED: toggling the vice-captain fallback moved a "+
				"prediction figure by %g (worst cell: %s).\nThat fallback changes how a "+
				"played-out gameweek is scored, not what the model predicts, so this "+
				"instrument is reading something it must be blind to. Do not trust any "+
				"other figure in this run.", worst, where)
		}
	}

	if dir := byLabel[armMinutesFlat]; dir != nil {
		b := base.err[predKey(popRelevant, "minutes", "model", catAll)]
		c := dir.err[predKey(popRelevant, "minutes", "model", catAll)]
		if b != nil && c != nil && b.n > 0 && c.n > 0 {
			d := c.mae() - b.mae()
			fmt.Printf("DIRECTIONAL CONTROL. Switching minutes recency off changes the minutes\n")
			fmt.Printf("mean absolute error by %+0.3f minutes a gameweek (%.3f to %.3f), on\n",
				d, b.mae(), c.mae())
			fmt.Printf("%d observations.\n", b.n)
			if d > 0 {
				fmt.Printf("PASSES: worse without recency, which is the direction the\n")
				fmt.Printf("out-of-sample minutes work established on 8,374 predictions. The\n")
				fmt.Printf("instrument can see an effect that is known to be real.\n\n")
			} else {
				t.Errorf("directional control FAILED: switching minutes recency off changed "+
					"the minutes mean absolute error by %+0.3f, i.e. it did not get worse.\n"+
					"Recency on minutes is this project's best-established out-of-sample "+
					"result (about 9%% better on 8,374 predictions), so an instrument that "+
					"cannot see it is wrong. Do not ship figures from this run.", d)
			}
		}
	}
}
