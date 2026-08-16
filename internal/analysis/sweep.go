package analysis

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
)

// Diagnostic overrides for constants that are otherwise unreachable from a
// replay, so they can be swept without editing code.
//
// Every one of these defaults to the shipped behaviour and is read once at
// startup. They exist because a constant nobody can vary is a constant nobody
// measures: the fixture ladders and the bench slot weights were both set by
// hand and stayed that way for exactly as long as changing them meant a patch.
// See FPL_FLAT_BENCH and FPL_NO_FUNDED_UPGRADE for the same pattern.
//
// Never set in normal operation.

var (
	// atkFixtureScale and defFixtureScale stretch the difficulty ladders around
	// 1.0 without changing their shape: a scale of s maps a shipped multiplier
	// m to 1 + s(m-1). Zero flattens the ladder entirely, which is the
	// "fixtures do not matter" control; 1 is shipped; above 1 widens it.
	//
	// Scaling the ladder is the only way to reach past full strength, because
	// FixtureWeight is clamped to [0,1] — setting it to 1.4 is silently
	// identical to 1.0.
	//
	// The two are separate knobs because the ladders are not symmetric with
	// each other. Attack spans 1.30 to 0.72 and defence 0.70 to 1.40, and the
	// clean sheet is 26-45% of a defender's or keeper's score against near zero
	// for a forward — so the defensive ladder already does more work, and the
	// documented range sweep only ever varied the attacking one.
	//
	// ⚠️ Read STRICTLY, for the reason envScaleStrict states and cleanSheetScale
	// records: both names are stamped into the run fingerprint by
	// internal/snapshot, so a value that cannot be parsed must not fall back.
	// `FPL_DEF_FIXTURE_SCALE=1,5` — a comma for a decimal point, and a plausible
	// typo — used to score every cell at the shipped 1.0 while the fingerprint
	// recorded the run as configured at 1.5. That is this record's
	// byte-identical-null trap wearing a provenance stamp: the arm looks
	// configured, returns a clean null, and the null is indistinguishable from
	// "the ladder does nothing" — which is very close to what this project
	// already believes about these two knobs, so it would have been believed.
	//
	// 0 is accepted and is the meaningful extreme here, not an error: it flattens
	// the ladder entirely, which is the "fixtures do not matter" control named
	// above. A negative scale inverts the ladder and is refused.
	//
	// These two shared a lenient parser with nothing else, so converging them on
	// the strict one removed it rather than leaving a helper whose only property
	// is the trap. Same precedent as parseSweepStarts: silence must not read as
	// success.
	atkFixtureScale = envScaleStrict("FPL_ATK_FIXTURE_SCALE", 1)
	defFixtureScale = envScaleStrict("FPL_DEF_FIXTURE_SCALE", 1)
)

// ladder applies a fixture scale to one rung.
func ladder(base, scale float64) float64 {
	if scale == 1 {
		return base
	}
	return 1 + scale*(base-1)
}

// benchSlotWeights overrides the per-slot bench credit, as
// FPL_BENCH_SLOTS="out1,out2,out3,gk".
//
// The values are renormalised to sum to four, which is what the shipped
// 1.9/1.2/0.6/0.3 do. Without that a sweep of the shape would also be a sweep
// of the overall scale, and BenchWeight already owns the scale — the two would
// be inseparable in the result.
func benchSlotWeights() ([3]float64, float64) {
	raw := strings.Split(os.Getenv("FPL_BENCH_SLOTS"), ",")
	if len(raw) != 4 {
		return defaultBenchOutfield, defaultBenchGK
	}
	var v [4]float64
	for i, s := range raw {
		f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil {
			return defaultBenchOutfield, defaultBenchGK
		}
		v[i] = f
	}
	no, ngk, ok := normaliseBenchSlots(v)
	if !ok {
		return defaultBenchOutfield, defaultBenchGK
	}
	return no, ngk
}

// defaultBenchOutfield and defaultBenchGK are the shipped tuple, named so both
// entry points fall back to the same thing rather than to two literals.
var defaultBenchOutfield, defaultBenchGK = [3]float64{2.4, 1.0, 0.4}, 0.2

// normaliseBenchSlots validates a raw tuple and renormalises it to sum to four.
//
// It carries the **validation as well as the arithmetic**, and that is the
// point. A first version shared only the renormalisation, so the environment
// path rejected a negative component and the setter installed it — one quantity
// with two input contracts, which is the same failure as two implementations
// wearing a shared helper. Code review caught it by probing both paths with the
// same tuple and getting [-1 5 0]/0 from one and the default from the other.
//
// Non-finite components are refused rather than propagated. Both paths
// previously accepted them and produced the same degenerate weights, so this
// changes behaviour identically on both and in the only direction a sweep wants.
func normaliseBenchSlots(v [4]float64) (out [3]float64, gk float64, ok bool) {
	total := 0.0
	for _, f := range v {
		if f < 0 || math.IsNaN(f) || math.IsInf(f, 0) {
			return out, 0, false
		}
		total += f
	}
	if total <= 0 {
		return out, 0, false
	}
	for i := range v {
		v[i] = v[i] * 4 / total
	}
	return [3]float64{v[0], v[1], v[2]}, v[3], true
}

// SetBenchSlots pins the fixed bench tuple in process, for a sweep that varies
// the bench *shape*.
//
// It renormalises to sum to four through the same function the environment path
// uses, so varying the shape does not also vary BenchWeight's scale — the two
// would otherwise be inseparable in the result.
//
// ⚠️ It does nothing while derived slots are on, because the derived weights are
// computed per eleven and never read this tuple (see squad.go's benchValue). A
// shape sweep must call SetDerivedBenchSlots(false) alongside it, or every arm
// is byte-identical for a reason that looks exactly like the shape not
// mattering. Not safe to call concurrently with scoring — sweeps run their
// variants sequentially.
// ⚠️ A refused tuple installs the **shipped default**, exactly as the
// environment path does — it does not leave the previous arm's tuple in place.
// That was the first version's behaviour and it is the worst available: an arm
// of {0,0,0},0, which is the obvious zero control for a bench sweep, silently
// replayed the *preceding* arm and reported a byte-identical null
// indistinguishable from "the bench weights do not matter".
//
// It also writes FPL_BENCH_SLOTS, so the constants fingerprint records what
// actually ran. `internal/snapshot`'s envSwitches list exists so a run at a
// non-default tuple cannot be called comparable with one at the default, and it
// reads the environment; a setter that reached the same state without touching
// it left three arms stamped with the shipped-config digest. Same principle as
// every provenance stamp in this project: the record derives from the value the
// simulation consumed,
// rather than being a second mechanism kept in sync by hand.
func SetBenchSlots(out [3]float64, gk float64) {
	no, ngk, ok := normaliseBenchSlots([4]float64{out[0], out[1], out[2], gk})
	if !ok {
		no, ngk = defaultBenchOutfield, defaultBenchGK
	}
	benchOutfieldWeights, benchGKWeight = no, ngk
	os.Setenv("FPL_BENCH_SLOTS", fmt.Sprintf("%g,%g,%g,%g", no[0], no[1], no[2], ngk))
}

// BenchSlotState reports the effective bench-slot configuration, so a sweep can
// restore what it found rather than what it assumes the default to be.
//
// The diagnostic that first used these setters restored by writing `true` and
// the shipped tuple unconditionally. Under `FPL_FIXED_BENCH_SLOTS=1` — a real
// way to run the suite — that silently flipped every *later* diagnostic in the
// process onto derived slots while the fingerprint still stamped the environment
// as fixed, which is a provenance failure rather than a scoring one and so would
// not have shown up in any number.
func BenchSlotState() (out [3]float64, gk float64, derived bool) {
	return benchOutfieldWeights, benchGKWeight, derivedBenchSlots
}

// envOpt reads an optional float. The second result distinguishes "not set"
// from a deliberate zero, which matters for both knobs below: an exponent of
// zero makes minutes irrelevant and a position scale of zero makes the
// rotation penalty neutral, and both are settings a sweep wants to reach.
func envOpt(name string) (float64, bool) {
	v, err := strconv.ParseFloat(os.Getenv(name), 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

var (
	// minutesWeightOverride replaces Weights.MinutesWeight, the convexity
	// exponent on minutes reliability. It sits in the environment rather than
	// only in config because the replay builds engines from several places and
	// a sweep has to reach all of them at once.
	minutesWeightOverride, minutesWeightSet = envOpt("FPL_MINUTES_WEIGHT")

	// posMinutesScaleOverride replaces entries in MinutesWeightByPosition — how
	// much of the global rotation severity each position carries. 1 applies it
	// in full, 0 makes the penalty neutral. Written as "MID=0.75,FWD=0.5".
	//
	// Any sweep of one position must be run at the current setting of the
	// others and of the reliability mix: the midfield scale spanned 13 points
	// against the old mix and 226 against the new one, because the two act on
	// the same players.
	posMinutesScaleOverride = envScaleMap("FPL_POS_MINUTES_SCALE")

	// reliabilityMinutesShare is how much of the reliability score comes from
	// minutes per match rather than start share. It ships at 1.0 — minutes
	// only — and the parameterisation is kept so the mix can be re-measured.
	// See reliabilityFrom for why the start-share term was dropped.
	reliabilityMinutesShare = envDefault("FPL_RELIABILITY_SPLIT", 1.0)
)

// envDefault reads a float in [0,1], falling back to def.
func envDefault(name string, def float64) float64 {
	v, ok := envOpt(name)
	if !ok || v < 0 || v > 1 {
		return def
	}
	return v
}

// cleanSheetXGCFactor scales expected goals conceded *only* where the
// clean-sheet probability is computed.
//
// exp(-xGC) over-predicts clean sheets at every level of xGC — 30.5% against
// 23.8% actual over four seasons, worth +0.27 points a match to every defender
// and keeper. The bias is not in xGC itself, whose mean matches goals conceded
// to within 1.1%; it is that the same xGC produces fewer clean sheets than a
// Poisson says, because xGC is regressive match to match. Low-xGC matches
// concede more than predicted (0.45 expected, 0.55 actual) and high-xGC matches
// concede less (2.64 expected, 2.49 actual).
//
// ⚠️ **Every figure in the paragraph above is measured against REALISED SINGLE-
// MATCH xGC, which is not what this factor multiplies.** It multiplies XGC90 — a
// per-player per-90 rate, blended toward a prior season and shrunk. Those are
// different regressors with different variances, and exp() is convex, so the
// aggregate over-prediction differs between them by construction.
//
// Refit against XGC90 itself (TestDiagCleanSheetRegressor, point-in-time, one
// row per team-gameweek), predicted against actual is **1.052 on the three
// native-xGC seasons and 1.004 pooled over six**, against the 1.281 above —
// rejected at |t| 6.7 and 9.5 on season-clustered SEs. ⚠️ Take the df from the
// comparison: native is **df 2** (three season clusters, t_crit 4.303) and
// pooled df 5 (2.571), so 6.7 clears its bar by 1.6x rather than the 3.3x a
// reader assuming t_crit 2 would infer. **Native is the headline**; the pooled
// stratum's errors-in-variables bias runs toward the rejection, so do not lean
// on it. So "+0.27 points a match" is **+0.05 native, +0.004 pooled** on the
// regressor this knob actually scales. The arithmetic that explains it is TWO
// mechanisms, and neither replaces the other:
//
//   - CROSS-MATCH CONVEXITY explains the gap BETWEEN the two regressors. exp() is
//     convex, so E[exp(-x)]/exp(-mean x) grows with the dispersion of x, and it
//     predicts the observed gap to 0.3% (1.2759 against 1.2799, season-matched).
//     ⚠️ Quote that exact ratio, never exp(sigma^2/2) at "sigma ~0.70": sigma
//     there is the sd of the DEVIATION, sd(x) is 0.848, and the approximation is
//     8% high at the realised dispersion though excellent at XGC90's.
//   - A SHOT-LEVEL wedge explains why exp(-x) over-predicts on realised x at all:
//     the model computes exp(-Sum x_i) but the true probability is
//     Prod(1-x_i) ~= exp(-c*Sum x_i) with c > 1. ⚠️ Its SIZE is not established
//     here. Fitting c from the clean-sheet rate is exactly identified -- one
//     parameter, one moment -- so it reproduces that moment by construction and
//     cannot test any mechanism. stats/xg_provider_scale.py's c = 1.27 is a
//     different season on a different feed (season-matched: 1.3291).
//
// ⚠️ And the near-calibration on XGC90 is a CANCELLATION rather than a structural
// property: the realised rate lands within 2.6% of exp(-mean x), which is what a
// smooth-regressor model computes, because two opposing wedges net to ~1.026 on
// the realised-xGC rows where both are visible. ⚠️ Quote that product and never
// the wedges: c is fitted so E[exp(-c*x)] equals the observed rate, so the
// decomposition TELESCOPES and the parts are an artefact of where the identity is
// cut. ⚠️ The fragility is in the MEAN, not the dispersion -- the whole dispersion
// channel is 1.0410, so annihilating XGC90's dispersion moves calibration 4.0%,
// while calibration goes as exp((c-1)*x_bar) and a 10% shift in the MEAN moves it
// 4.1%. Design any future arm on this term off the level channel. Sizes live in
// stats/findings/2026-08-15-clean-sheet-2x2.md.
//
// ⚠️ That does NOT establish the term is correctly calibrated — the refit's slope
// separates neither b=1 (t -0.05) nor b=1.1731 (t -1.19); its 80%-power MDE on
// |b-1| is 0.424 against a candidate of 0.173, and the native ratio interval is
// [0.90, 1.20], so a fifth of the recorded bias is still inside it. It measures
// the NEUTRAL path, fixing def=1. It refutes the recorded MAGNITUDE, nothing more.
//
// ✅ The population does flatter it, and it is now SIZED rather than disclosed.
// The 90-minute guard drops 14.2% of single-fixture club-gameweeks and the
// dropped set is genuinely worse defensively (clean-sheet rate 0.1992 against
// 0.2636, fixture-derived so it exists for dropped rows too). Carrying `pred`
// on the dropped rows too, the pooled ratio goes 1.0051 -> 1.0305 unselected;
// applying the defcon coupling the dump omits takes it to 1.0112 on kept. Both
// run the predicted way, and together they move the pooled over-prediction from
// ~0.4% to roughly 3.7% (1.0038 + 0.0254 + 0.0074) -- which leaves the
// refutation of 1.281 untouched and rules out "calibrated to within half a
// percent". ⚠️ A composition of two separately-measured shifts rather than a
// joint measurement, so an interaction of order 0.1pp is unmeasured.
// stats/snapshots/2026-08-15-clean-sheet-2x2/.
//
// The correction still belongs here rather than on XGC90, which also drives the
// goals-conceded deduction — that term is fed by the mean, which is right.
// See TestDiagCleanSheetPoisson and TestDiagCleanSheetRegressor.
var cleanSheetXGCFactor = envDefaultAbove("FPL_CS_XGC_FACTOR", 1.0)

// SetCleanSheetXGCFactor overrides the xGC factor for a diagnostic sweep. Not
// safe to call concurrently with scoring — sweeps run their variants
// sequentially.
//
// It exists because the env var alone forced one process per arm, and separate
// processes produce separate run_ids that must not be pooled. The 2x2 this and
// SetCleanSheetScale were added for has to run in ONE process for that reason.
func SetCleanSheetXGCFactor(v float64) { cleanSheetXGCFactor = v }

// cleanSheetScale multiplies the clean-sheet probability by a constant, where
// cleanSheetXGCFactor scales the exponent.
//
// The two are the separate halves of one fitted curve and must be swept
// together. Fitting -ln p against xGC per observation, on native-xGC rows and
// with the Fixtures != 1 guard, gives
//
//	-ln p = 0.1003 + 1.1731*x   =>   P = 0.9046 * exp(-1.1731*x)
//
// so the intercept is this knob (0.905) and the slope is the other (1.173).
// Both one-parameter restrictions of that fit are rejected, which is why a
// ladder over the factor alone is the wrong experiment: exp(-(f-1)x) depends on
// x, so moving the factor to carry a level pushes the level through the slope
// and over-corrects the ordering by roughly 40% at f = 1.27.
//
// ⚠️ Both figures above describe the fit against REALISED SINGLE-MATCH xGC. On
// XGC90 — what these knobs actually multiply — the joint fit is f = 0.992,
// flat = 0.939 on native rows and NEITHER restriction is rejected (LRT p 0.76
// scale, 0.96 offset; b = 0.9922, clustered SE 0.1516). That is non-separation,
// not acceptance: the fit cannot tell b = 1 from b = 1.1731 either (t -1.19).
//
// ⚠️ The 2x2 RAN 2026-08-15 at f = 1.1731, flat = 0.9046 and resolved nothing:
// +1.9 / +6.2 / +7.0 a season against thresholds 23 / 16 / 20, Holm 1.000. Its
// canary — halving every clean sheet — costs only -21.6 against its own
// threshold of 28, so this family is about 4x below detection on points. Do not
// re-run it at the refitted constants; a factor arm of 0.992 is a no-op.
// stats/snapshots/2026-08-15-clean-sheet-2x2/.
//
// The two knobs are different KINDS of change, which is why the interaction was
// worth a cell. ⚠️ **But "the flat scale is ordering-inert within a position" is
// WITHDRAWN (2026-08-15).** It multiplies one ADDITIVE component of Score, not
// Score itself, so two defenders whose clean-sheet share of Score differs — the
// term is 26-45% of a defender's and 0% of a forward's — reorder under it.
// xgcrepair.go records the same correction for a uniform xGC scaling and warns
// that the position-wide precedent is about an ADDITIVE bias. The factor's
// within-position reprice is larger and better identified; it is not the only one.
// ⚠️ Read STRICTLY rather than through envDefaultAbove, and the difference is
// the point. envDefaultAbove silently falls back on any value <= 0, so
// FPL_CS_SCALE=0 would score every cell at the shipped 1.0 while the fingerprint
// stamped 0 — a fully configured-looking run that measured nothing, which is this
// record's byte-identical-null trap wearing a provenance stamp. And 0 is a
// MEANINGFUL setting here, unlike for the factor: it is "no clean sheets at all",
// the natural extreme arm. So a negative panics and 0 is accepted. Same precedent
// as parseSweepStarts, which panics rather than falling back on anything it
// cannot read, for the same reason: silence must not read as success.
var cleanSheetScale = envScaleStrict("FPL_CS_SCALE", 1.0)

// envScaleStrict reads a non-negative float, panicking on anything it cannot
// read rather than falling back to the default.
//
// It is the only STRICT parser here, and the three knobs that read it —
// FPL_CS_SCALE and the two fixture ladders — are the only ones that cannot lie
// about what ran. It was written for FPL_CS_SCALE, and the ladders kept a
// lenient sibling for a while: one quantity with two input contracts, this
// project's signature failure, and here the lenient contract was the bug. Do
// not add a third parser; route a new scale through this one.
//
// ⚠️ **The rest of this file is still lenient, and every one of those knobs is
// fingerprinted too.** envDefaultAbove (FPL_CS_XGC_FACTOR, FPL_CAPTAIN_SHRINK,
// FPL_BUY_DISCOUNT, FPL_BLANK_RUN_PENALTY, FPL_BLANK_RUN_MAX,
// FPL_MAGNITUDE_ALPHA), envDefault (FPL_RELIABILITY_SPLIT), envOpt
// (FPL_MINUTES_WEIGHT), envScaleMap (FPL_POS_MINUTES_SCALE), benchSlotWeights
// and appearanceFit all fall back silently, so each carries the same trap. Two
// are worse than the one fixed here, because they refuse a MEANINGFUL setting:
// envDefaultAbove defaults on v <= 0, so FPL_CS_XGC_FACTOR=0 — no clean-sheet
// exponent at all — scores as shipped, and envScaleMap drops an unparseable
// entry while applying its siblings, so half a position map arrives. Converging
// them is a larger change than this one and is not attempted here.
//
// Non-finite input is NOT refused: ParseFloat accepts "NaN" and "Inf", and
// neither is < 0. That is inherited rather than introduced — the lenient parser
// accepted them identically — and it is provenance-HONEST, since the fingerprint
// then stamps "NaN" rather than a value that did not run. normaliseBenchSlots
// above refuses non-finite components and its argument transfers; doing the same
// here would also change FPL_CS_SCALE, so it is left for a change that says so.
//
// The raw value is TRIMMED before parsing, so " 1.5" is honoured rather than
// falling back. Under the lenient parser that input was one of the silent
// fallbacks this function exists to remove — ParseFloat never accepts
// surrounding whitespace, so trimming can only affect inputs that used to
// default.
func envScaleStrict(name string, def float64) float64 {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return def
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v < 0 {
		panic(fmt.Sprintf("%s=%q: want a non-negative float. It is not defaulted, "+
			"because a run configured with an unreadable scale would score at the "+
			"shipped value while its fingerprint recorded the one you asked for.", name, raw))
	}
	return v
}

// SetCleanSheetScale overrides the flat clean-sheet scale for a diagnostic
// sweep. Same concurrency caveat as SetCleanSheetXGCFactor.
func SetCleanSheetScale(v float64) { cleanSheetScale = v }

// CleanSheetState reads both knobs back out, so a sweep can restore what it
// actually found rather than what it assumes the shipped default is. Both are
// env-settable, so assuming 1.0 would silently rewrite a run made under
// FPL_CS_XGC_FACTOR — and the fingerprint would still stamp the environment.
func CleanSheetState() (factor, scale float64) { return cleanSheetXGCFactor, cleanSheetScale }

// cleanSheetProb is P(clean sheet) as the model scores it: the Poisson zero
// against expected goals conceded, with both swept knobs applied.
//
// It is a function rather than an expression repeated at each site because
// there were four copies of the exponent and the desync between two of them is
// a bug this package has already shipped — fixtureSensitiveAt wrote
// exp(-XGC90) while baseXP90 wrote exp(-cleanSheetXGCFactor x XGC90 x
// defconCleanFactor), which was harmless only while the extra factors were 1.
// Adding a fifth factor to four copies would rebuild that bug with a wider
// blast radius, so the copies are collapsed here first. One quantity, one
// implementation.
//
// def is the opponent's defensive multiplier and cf the defcon coupling; pass
// 1 for either where the caller does not vary it.
func cleanSheetProb(xgc90, def, cf float64) float64 {
	return cleanSheetScale * math.Exp(-cleanSheetXGCFactor*xgc90*def*cf)
}

// envDefaultAbove reads a positive float, falling back to def.
func envDefaultAbove(name string, def float64) float64 {
	v, ok := envOpt(name)
	if !ok || v <= 0 {
		return def
	}
	return v
}

// envScaleMap parses "MID=0.75,FWD=0.5" into a position scale override.
func envScaleMap(name string) map[string]float64 {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return nil
	}
	out := map[string]float64{}
	for _, part := range strings.Split(raw, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(kv[1]), 64)
		if err != nil || v < 0 {
			continue
		}
		out[strings.ToUpper(strings.TrimSpace(kv[0]))] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// thresholdSplit scales appearance points and the clean sheet by P(60 minutes)
// rather than by minutes reliability. Set FPL_NO_THRESHOLD_SPLIT=1 to restore
// the old behaviour and re-measure. See Engine.thresholdXP90.
var thresholdSplit = os.Getenv("FPL_NO_THRESHOLD_SPLIT") == ""

// shortPlayCredit pays the appearance point FPL gives for playing at all, not
// only the second point it gives at sixty minutes.
//
// The model had only the upper branch, so a player who appeared for fifty minutes
// was credited nothing where FPL pays him one. Worth +0.185 appearance points per
// gameweek pooled, peaking at +0.283 around 25-35 mean minutes — the fringe and
// rotation population. Set FPL_NO_SHORT_PLAY=1 to restore the single branch and
// re-measure. See appearanceFactor and playsAtAll.
var shortPlayCredit = os.Getenv("FPL_NO_SHORT_PLAY") == ""

var (
	// magnitudeDifficulty replaces FPL's integer fixture ladder with one
	// proportional to how good the opponent actually is. Off by default until
	// measured; set FPL_MAGNITUDE=1. See teamstrength.go.
	magnitudeDifficulty = os.Getenv("FPL_MAGNITUDE") != ""

	// magnitudeAlpha is the exponent on the opponent's goal ratio. 0.5 makes the
	// response sub-proportional, which is what the +23% measured against
	// defences conceding ~50% more implies; 1.0 would be straight proportion and
	// is the overshoot that sank the previous attempt.
	magnitudeAlpha = envDefaultAbove("FPL_MAGNITUDE_ALPHA", 0.5)
)

// fixtureLoadScaling multiplies Score by matches per gameweek. On, and confined
// to the horizon-1 view by fixtureLoadWeeklyOnly: worth +33 points a season at
// t = +5.74 with squad selection byte-identical. Applied everywhere it damages
// the opening fifteen instead. FPL_NO_FIXTURE_LOAD=1 restores the old behaviour.
var fixtureLoadScaling = os.Getenv("FPL_NO_FIXTURE_LOAD") == ""

// fixtureLoadWeeklyOnly restricts the scaling to a horizon-1 engine — the view
// that picks the eleven actually fielded — leaving squad building and transfer
// decisions alone. See fixtureLoadFor.
var fixtureLoadWeeklyOnly = true

// fixtureLoadTransfers applies matches-per-gameweek inside XIValue, which is the
// transfer search's objective and is not reached by squad construction. It lets
// the weekly decision see a double coming without letting it distort the opening
// fifteen. See XIValue.
var fixtureLoadTransfers = os.Getenv("FPL_NO_LOAD_TRANSFERS") == ""

// SetFixtureLoadTransfers is for sweeps; see SetFixtureLoad.
func SetFixtureLoadTransfers(v bool) { fixtureLoadTransfers = v }

// SetFixtureLoadWeeklyOnly is for sweeps; see SetFixtureLoad.
func SetFixtureLoadWeeklyOnly(v bool) { fixtureLoadWeeklyOnly = v }

// SetFixtureLoad overrides it for a diagnostic sweep. Not safe to call
// concurrently with scoring — sweeps run their variants sequentially.
func SetFixtureLoad(on bool) { fixtureLoadScaling = on }

// buyDiscount is the measured buy-side over-rating, charged against a player
// being acquired. See discountIncoming. Zero disables it.
var buyDiscount = envDefaultAbove("FPL_BUY_DISCOUNT", 0)

// SetBuyDiscount overrides it for a diagnostic sweep. Not safe to call
// concurrently with scoring — sweeps run their variants sequentially.
func SetBuyDiscount(v float64) { buyDiscount = v }

// BuyDiscount reports it, so a diagnostic that recomputes a swap's gain from
// XIValue rather than reading Swap.Gain can refuse to run when the two are not
// the same quantity.
func BuyDiscount() float64 { return buyDiscount }

// captainShrink pulls the captain term toward the runner-up. See xiValue.
//
// A package-level var rather than a Weights field because xiValue is a free
// function reached from bestXI, objective and XIValue, and threading the engine
// through all three to read one constant is a larger change than the constant
// deserves. It is written once at init and only read afterwards, so the
// concurrent tool runner is safe; SetCaptainShrink exists for sweeps and must
// not be called while anything is scoring.
var captainShrink = envDefaultAbove("FPL_CAPTAIN_SHRINK", 1.0)

// SetCaptainShrink overrides the captain shrink for a diagnostic sweep. Not safe
// to call concurrently with scoring — sweeps run their variants sequentially.
func SetCaptainShrink(v float64) { captainShrink = v }

// appearanceFit overrides the four constants of the two appearance curves, as
// FPL_APPEARANCE_FIT="sixty_slope,sixty_midpoint,cond_intercept,cond_slope" — the
// order they appear in appearance.go, which is also the order playsSixty then
// playsAtAll use them.
//
// All four or none. A partial override is refused rather than half-applied,
// because the two curves are held together by an ordering constraint —
// playsAtAll takes the max of the identity and playsSixty, so P(appears) can
// never fall below P(60+) — and a fit that moves one curve while the other stays
// shipped can silently spend its whole effect on that max rather than on the
// term it was aimed at.
//
// The defaults are passed in rather than restated here so that this file cannot
// drift from appearance.go's shipped values, which is the DefaultBenchWeight
// failure this project has already shipped once: two defaults for one quantity,
// so the measured one is not the one that runs.
func appearanceFit(sixtySlope, sixtyMid, condIntercept, condSlope float64) (
	float64, float64, float64, float64) {

	raw := strings.Split(os.Getenv("FPL_APPEARANCE_FIT"), ",")
	if len(raw) != 4 {
		return sixtySlope, sixtyMid, condIntercept, condSlope
	}
	var v [4]float64
	for i, s := range raw {
		f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil {
			return sixtySlope, sixtyMid, condIntercept, condSlope
		}
		v[i] = f
	}
	return v[0], v[1], v[2], v[3]
}

// SetAppearanceFit overrides the four appearance constants for a diagnostic
// sweep, and ShippedAppearanceFit returns the values to restore afterwards.
//
// Both exist for the prediction benchmark's refit arms, which need to switch fits
// between arms in one process rather than across two runs. Not safe to call
// concurrently with scoring — sweeps run their variants sequentially.
func SetAppearanceFit(sixtySlopeV, sixtyMidV, condInterceptV, condSlopeV float64) {
	sixtySlope, sixtyMidpoint = sixtySlopeV, sixtyMidV
	condMinutesIntercept, condMinutesSlope = condInterceptV, condSlopeV
}

// ShippedAppearanceFit is the fit as it ships, in SetAppearanceFit's argument
// order. It reads the constants rather than the live variables, so it restores
// the shipped curves even if called while an override is in force.
func ShippedAppearanceFit() (float64, float64, float64, float64) {
	return shippedSixtySlope, shippedSixtyMidpoint,
		shippedCondMinutesIntercept, shippedCondMinutesSlope
}

// AppearanceFit reports the live values, so a diagnostic can print the fit it
// actually ran under rather than the one it believes it set.
func AppearanceFit() (float64, float64, float64, float64) {
	return sixtySlope, sixtyMidpoint, condMinutesIntercept, condMinutesSlope
}

// PlaysSixty and PlaysAtAll expose the two shipped curves so a diagnostic can
// measure what the model actually credits.
//
// They exist because the alternative had already gone wrong. TestDiagSixtyMinutes
// scored a column labelled "model now" against realised sixty-minute rates, and
// that column was `(minutes/90)^MinutesWeight` — the minutes-reliability proxy
// playsSixty *replaced*. Its error was then read into the accuracy snapshot as a
// property of the shipped model, and a reader had no way to tell. That is this
// package's signature failure — one quantity with two implementations, and the
// measured one is not the one that runs — arriving through a diagnostic instead of
// through the model.
//
// So: no diagnostic should carry its own copy of a curve it is checking. A
// deliberately superseded estimator is still worth printing, and it must be
// labelled as superseded rather than as "now".
func PlaysSixty(meanMinutes float64) float64 { return playsSixty(meanMinutes) }

// PlaysAtAll is P(records at least one minute), the sibling of PlaysSixty. See
// there for why both are exported.
func PlaysAtAll(meanMinutes float64) float64 { return playsAtAll(meanMinutes) }

// AttackMultiplier is the shipped attacking fixture ladder at one FPL difficulty,
// exported for the same reason the two appearance curves are: a diagnostic asking
// whether the ladder already explains an effect must use the ladder the model runs,
// not a copy of its numbers. Copies of curves have gone stale twice in this
// package's diagnostics and once fed a published figure.
func AttackMultiplier(difficulty int) float64 { return attackMultiplier(difficulty) }

// SetUnifiedAppearance routes blankRate through the single appearance estimator
// (true, shipped) or through the second start-share fit it replaced (false). See
// appearance.go for why there were two and which one is better. Not safe to call
// concurrently with scoring — sweeps run their variants sequentially.
func SetUnifiedAppearance(v bool) { unifiedAppearance = v }

// SetDerivedBenchSlots prices bench slots from the eleven's own blank
// probabilities (true, shipped) or from the fixed tuple (false). It exists so a
// sweep can pin the tuple and separate a change in the blank rate from the change
// it causes in the bench weights, which is otherwise two effects at once. Not safe
// to call concurrently with scoring.
// It keeps FPL_FIXED_BENCH_SLOTS in step for the reason given on SetBenchSlots:
// the constants fingerprint reads the environment, and a setter that reached the
// same state without it stamped three non-default arms with the shipped digest.
func SetDerivedBenchSlots(v bool) {
	derivedBenchSlots = v
	if v {
		os.Unsetenv("FPL_FIXED_BENCH_SLOTS")
		return
	}
	os.Setenv("FPL_FIXED_BENCH_SLOTS", "1")
}

// blankRunAdjust discounts a player in the first few gameweeks of an absence,
// where the exponential minutes average has not yet caught up. The penalty and
// the window are both measured — see blankRunFactor. Set FPL_NO_BLANK_RUN=1 to
// restore the old behaviour.
var (
	blankRunAdjust  = os.Getenv("FPL_NO_BLANK_RUN") == ""
	blankRunPenalty = envDefaultAbove("FPL_BLANK_RUN_PENALTY", 0.75)
	blankRunMax     = int(envDefaultAbove("FPL_BLANK_RUN_MAX", 3))
)

// savesFixtureAdjust scales a keeper's save points by the same opponent
// multiplier that scales his expected goals conceded. Set FPL_NO_SAVES_FIXTURE=1
// to restore the old behaviour, where saves were carried across unadjusted.
var savesFixtureAdjust = os.Getenv("FPL_NO_SAVES_FIXTURE") == ""
